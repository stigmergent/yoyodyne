package orchestrator

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/goal"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/publish"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// These drive yoyodyne-m5p: a queued merge whose checks fail with its head
// level with the target, on a file the change does not touch, is the target's
// failure. It is filed as the target's, as a red landing is, and the merge waits
// on the item filed for it with the harness as the one to move, rather than
// being handed to a person.

// redTargetGoal is the goal the fixture's item served, which the filed item is
// attributed to.
const redTargetGoal = "Run development nearly autonomously."

// redTargetReading is pull request 863's shape on 2026-09-28: the head level
// with main, a failing check on a file under .github the change does not touch.
func redTargetReading() publish.CheckReading {
	return publish.CheckReading{
		Files:   []string{"feature.txt"},
		Failing: []publish.FailedCheck{{Name: "adoption", Paths: []string{".github/workflows/adoption.yml"}, ID: 4215, Conclusion: "failure"}},
		Passing: 3,
	}
}

// jobLogs is the forge's log of a job, as the harness reads its tail.
type jobLogs struct {
	tail  string
	asked []int64
}

func (l *jobLogs) JobLogTail(_ context.Context, checkRun int64, _ int) (string, error) {
	l.asked = append(l.asked, checkRun)
	return l.tail, nil
}

// redTargetSweep is a run whose merge is queued on a protected main, whose item
// names the goal it served, swept by a reconciler that can file work and read
// a job's log.
func redTargetSweep(t *testing.T, filer *recordingFiler) (queuedFixture, *checkedForge, *orchestratortest.Tracker, Reconciler, *jobLogs) {
	t.Helper()
	fixture := newQueuedFixture(t)
	tracker := fixture.tracker.(*orchestratortest.Tracker)
	tracker.Item.Notes = goal.Note(redTargetGoal)
	fixture.forge.SetTargetProtection(publish.BranchProtection{Protected: true, By: "ruleset"})
	outcome := fixture.run(t)
	if outcome.Integration == nil || !outcome.Integration.ThroughPullRequest {
		t.Fatalf("integration = %#v, want a landing through the pull request", outcome.Integration)
	}
	forge := &checkedForge{queuedForge: fixture.forge}
	forge.reading = redTargetReading()
	fixture.docket = &memoryDocket{}
	reconciler := fixture.sweep(t, forge, true)
	logs := &jobLogs{tail: "--- FAIL: TestAdoption (0.01s)\n    adoption_test.go:12: bd is not installed"}
	reconciler.Filer = filer
	reconciler.JobLogs = logs
	return fixture, forge, tracker, reconciler, logs
}

// A head level with main failing a check on a file its change does not touch
// files one p0 bug for main's check, under the goal the item served and carrying
// the job's log; the queued merge is withdrawn and waits on that bug; and nothing
// is handed to a person — no blocker, no dropped merge, and the docket names the
// harness as the one to move.
func TestAQueuedHeadLevelWithItsTargetFailingAnUntouchedFileWaitsOnTheTargetsFiledItem(t *testing.T) {
	t.Parallel()

	filer := &recordingFiler{}
	fixture, forge, tracker, reconciler, logs := redTargetSweep(t, filer)

	results, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionWaitingOnTarget || results[0].Failure != "" {
		t.Fatalf("reconciliation = %#v, want the merge waiting on the target's red check", results)
	}
	if len(forge.withdrawn) != 1 || forge.HoldsQueuedMerge() {
		t.Fatalf("withdrawn = %v, queued = %t; want the queued merge withdrawn", forge.withdrawn, forge.HoldsQueuedMerge())
	}

	// Filed once, as a red landing is: a p0 bug naming the branch, the commit,
	// the check, and the request that met it, under the item's goal.
	if len(filer.filed) != 1 {
		t.Fatalf("filed = %#v, want one item for main's red check", filer.filed)
	}
	filed := filer.filed[0]
	if filed.Type != "bug" || filed.Priority == nil || *filed.Priority != 0 {
		t.Errorf("filed type %q priority %v, want a p0 bug", filed.Type, filed.Priority)
	}
	recorded := loadRun(t, fixture.store, pipelineRunID)
	for _, want := range []string{"adoption", "main", "pull request " + strconv.Itoa(recorded.PullRequest.Number), shortCommit(recorded.PullRequest.HeadCommit), pipelineRunID} {
		if !strings.Contains(filed.Title+filed.Description, want) {
			t.Errorf("the filed item does not name %q:\n%s\n%s", want, filed.Title, filed.Description)
		}
	}
	for _, want := range []string{redTargetMarker("main", "adoption"), goal.Note(redTargetGoal), "How the forge ended adoption: failure", "> --- FAIL: TestAdoption", "> " + "    adoption_test.go:12: bd is not installed"} {
		if !strings.Contains(filed.Notes, want) {
			t.Errorf("the filed item's notes do not carry %q:\n%s", want, filed.Notes)
		}
	}
	if len(logs.asked) != 1 || logs.asked[0] != 4215 {
		t.Errorf("job logs asked = %v, want the failing job's log read", logs.asked)
	}

	// The merge waits on the filed item, and nothing is handed to a person.
	record := tracker.Record()
	if record.Blocked || record.Closed {
		t.Fatalf("blocked = %t, closed = %t; the target's failure hands nothing to a person", record.Blocked, record.Closed)
	}
	if len(tracker.Blockers) != 1 || tracker.Blockers[0] != "yoyodyne-red-1" {
		t.Errorf("the item waits on %v, want the filed item", tracker.Blockers)
	}
	if !strings.Contains(record.Notes, "fail on the target branch itself") || !strings.Contains(record.Notes, "yoyodyne-red-1") {
		t.Errorf("the item was not told it waits on the target's filed item:\n%s", record.Notes)
	}
	if recorded.MergeDrop != nil || recorded.Blocker != "" || recorded.PullRequest.MergeQueued {
		t.Fatalf("record = drop %#v, blocker %q, queued %t; want no drop and no blocker, and the merge withdrawn", recorded.MergeDrop, recorded.Blocker, recorded.PullRequest.MergeQueued)
	}
	if !recorded.WaitingOnRedTarget() || recorded.PullRequest.TargetRed.WaitingOn()[0] != "yoyodyne-red-1" {
		t.Fatalf("target red = %#v, want the publication waiting on the filed item", recorded.PullRequest.TargetRed)
	}

	// The docket carries it as the harness's, and puts no stoppage to anybody.
	docketer := docketerOverStore(fixture.docket, fixture.store, docketConfig())
	if _, err := docketer.Build(); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	var publication triage.Entry
	for _, entry := range fixture.docket.entries {
		switch entry.Class {
		case triage.ClassPublication:
			publication = entry
		case triage.ClassStoppedRun:
			t.Errorf("docket = %#v; a merge waiting on the target is not a stoppage anybody decides", entry)
		}
	}
	if publication.Publication == nil {
		t.Fatalf("docket = %#v, want the publication docketed", fixture.docket.entries)
	}
	rendered := publication.Render()
	for _, want := range []string{"Waiting on the target", "adoption (filed as yoyodyne-red-1)", "Next mover: the harness"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("docket entry does not say %q:\n%s", want, rendered)
		}
	}

	// A second sweep while the item is open waits and files nothing.
	tracker.AlsoHolds = map[string]beads.WorkItem{"yoyodyne-red-1": {ID: "yoyodyne-red-1", Status: "open"}}
	waiting, err := reconciler.ResumeRedTargets(context.Background())
	if err != nil {
		t.Fatalf("ResumeRedTargets() error = %v", err)
	}
	if len(waiting) != 1 || waiting[0].Action != ActionWaitingOnTarget || !strings.Contains(waiting[0].Detail, "still waits on yoyodyne-red-1") {
		t.Fatalf("resumptions = %#v, want the publication still waiting on its open item", waiting)
	}
	if len(filer.filed) != 1 {
		t.Errorf("filed = %d items, want nothing filed again while the first is open", len(filer.filed))
	}
}

// A later request meeting the same red check finds the item already open for it
// and is noted on it, rather than filing a second beside it.
func TestALaterRequestMeetingTheSameRedCheckIsNotedOnTheOpenItem(t *testing.T) {
	t.Parallel()

	filer := &recordingFiler{open: []beads.WorkItem{{
		ID:     "yoyodyne-red-earlier",
		Status: "open",
		Notes:  "Filed by the harness for adoption red on main.\n" + redTargetMarker("main", "adoption"),
	}}}
	fixture, forge, tracker, reconciler, _ := redTargetSweep(t, filer)

	results, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionWaitingOnTarget {
		t.Fatalf("reconciliation = %#v, want the merge waiting on the target's red check", results)
	}
	if len(filer.filed) != 0 {
		t.Fatalf("filed = %#v, want the open item noted rather than a second filed", filer.filed)
	}
	if len(forge.withdrawn) != 1 {
		t.Errorf("withdrawn = %v, want the queued merge withdrawn", forge.withdrawn)
	}
	if !strings.Contains(tracker.Record().Notes, "Red again on main") {
		t.Errorf("the open item was not told about this request:\n%s", tracker.Record().Notes)
	}
	if len(tracker.Blockers) != 1 || tracker.Blockers[0] != "yoyodyne-red-earlier" {
		t.Errorf("the item waits on %v, want the item already open for the check", tracker.Blockers)
	}
	recorded := loadRun(t, fixture.store, pipelineRunID)
	check := recorded.PullRequest.TargetRed.Checks[0]
	if check.WorkItem != "yoyodyne-red-earlier" || !check.FiledEarlier {
		t.Errorf("target red check = %#v, want the earlier item named as filed earlier", check)
	}
}

// A filing the tracker refuses leaves the merge queued and writes nothing, so
// the next sweep files it: the merge is never withdrawn to wait on nothing.
func TestAFilingThatFailsLeavesTheMergeQueuedForTheNextSweep(t *testing.T) {
	t.Parallel()

	filer := &recordingFiler{refuse: errors.New("bd create timed out")}
	fixture, forge, tracker, reconciler, _ := redTargetSweep(t, filer)

	results, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionQueued || !strings.Contains(results[0].Detail, "could not be filed") {
		t.Fatalf("reconciliation = %#v, want the merge left queued saying the filing failed", results)
	}
	if len(forge.withdrawn) != 0 || !forge.HoldsQueuedMerge() || tracker.Record().Blocked {
		t.Fatalf("withdrawn = %v, blocked = %t; a filing that failed withdraws nothing and hands nothing back", forge.withdrawn, tracker.Record().Blocked)
	}
	if recorded := loadRun(t, fixture.store, pipelineRunID); !recorded.PullRequest.MergeQueued || recorded.PullRequest.TargetRed != nil {
		t.Errorf("record = queued %t, target red %#v; want the merge still recorded as queued", recorded.PullRequest.MergeQueued, recorded.PullRequest.TargetRed)
	}
}

// Once the filed item closes, the fix having landed on main leaves the head
// behind it: the sweep brings it up to date from the kept branch, checks and
// reviews it again, and queues its merge again, as a queued head behind its
// target is — rather than re-arming a head the target has moved past.
func TestAWaitOnTheTargetWhoseItemClosedIsBroughtUpToDateWhereTheFixLeftTheHeadBehind(t *testing.T) {
	t.Parallel()

	filer := &recordingFiler{}
	fixture, forge, tracker, reconciler, _ := redTargetSweep(t, filer)
	if _, err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// The fix lands on main and its item closes, which the tracker also says on
	// the edge the item was made to wait on.
	driftRemoteTarget(t, fixture.remote, "main")
	tracker.AlsoHolds = map[string]beads.WorkItem{"yoyodyne-red-1": {ID: "yoyodyne-red-1", Status: "closed"}}
	for index := range tracker.Item.Dependencies {
		tracker.Item.Dependencies[index].Status = "closed"
	}
	forge.reading = publish.CheckReading{Files: []string{"feature.txt"}, Passing: 4, BehindBy: 1}

	resumed, err := reconciler.ResumeRedTargets(context.Background())
	if err != nil {
		t.Fatalf("ResumeRedTargets() error = %v", err)
	}
	if len(resumed) != 1 || resumed[0].Action != ActionUpdating || resumed[0].Failure != "" {
		t.Fatalf("resumptions = %#v, want the head put back at its promotion", resumed)
	}
	live := loadRun(t, fixture.store, pipelineRunID)
	if !updatingQueuedHead(live) || live.PullRequest.TargetRed != nil || live.PublishFailure != "" {
		t.Fatalf("record = status %q, target red %#v, publish failure %q; want a live run at its promotion with the wait ended",
			live.Status, live.PullRequest.TargetRed, live.PublishFailure)
	}
	if !strings.Contains(tracker.Record().Notes, "every item the merge of pull request") {
		t.Errorf("the item was not told why it is being brought up to date:\n%s", tracker.Record().Notes)
	}

	updates, err := reconciler.ContinueUpdates(context.Background())
	if err != nil {
		t.Fatalf("ContinueUpdates() error = %v", err)
	}
	if len(updates) != 1 || !updates[0].Continued || updates[0].Outcome == nil || updates[0].Outcome.PullRequest == nil || !updates[0].Outcome.PullRequest.MergeQueued {
		t.Fatalf("updates = %#v, want the replayed change's merge queued again", updates)
	}
	if tracker.Record().Blocked {
		t.Error("the item was handed to a person on the way")
	}
}

// redTargetRearmForge is the forge a re-arm reads: nothing unmet on the request,
// and the head's checks as the fixture sets them.
type redTargetRearmForge struct {
	*checkedForge
}

func (redTargetRearmForge) MergeState(context.Context, int) (string, error) {
	return "CLEAN", nil
}

// Once the filed item closes on a head still level with main whose checks now
// pass, the watch's re-arm carry-out arms the merge with nobody deciding, and
// spends no re-arm. While the item is open the Rearmer refuses and says what it
// waits on.
func TestTheWatchRearmsAMergeWaitingOnTheTargetOnceItsItemClosesWithNoDecision(t *testing.T) {
	t.Parallel()

	filer := &recordingFiler{}
	fixture, forge, tracker, reconciler, _ := redTargetSweep(t, filer)
	if _, err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	docketer := docketerOverStore(fixture.docket, fixture.store, docketConfig())
	if _, err := docketer.Build(); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	rearmer := Rearmer{
		Docket:    fixture.docket,
		Runs:      fixture.store,
		Forge:     redTargetRearmForge{forge},
		Checks:    forge,
		Worktrees: newSweepManager(t, fixture.repository, fixture.worktreeRoot),
		Decisions: fixture.store.Triage(),
		Items:     tracker,
	}
	carry := CarryOut{
		Docket:    fixture.docket,
		Decisions: fixture.store.Triage(),
		Reruns:    fixture.store.Reruns(),
		Runs:      fixture.store,
		Rearmer:   rearmer,
		Items:     tracker,
	}

	// The item is open: nothing is attempted, and the verb refuses naming it.
	tracker.AlsoHolds = map[string]beads.WorkItem{"yoyodyne-red-1": {ID: "yoyodyne-red-1", Status: "open"}}
	if carried, err := carry.CarryRearms(context.Background(), false); err != nil || len(carried) != 0 {
		t.Fatalf("CarryRearms() = %#v, %v; want nothing attempted while the item is open", carried, err)
	}
	if _, err := rearmer.Rearm(context.Background(), RearmRequest{Run: pipelineRunID, Reason: "asked"}); err == nil || !strings.Contains(err.Error(), "waits on yoyodyne-red-1") {
		t.Fatalf("Rearm() error = %v, want it refused naming the open item", err)
	}

	// The item closes and the head, still level, passes.
	tracker.AlsoHolds["yoyodyne-red-1"] = beads.WorkItem{ID: "yoyodyne-red-1", Status: "closed"}
	forge.reading = publish.CheckReading{HeadCommit: forge.reading.HeadCommit, Files: []string{"feature.txt"}, Passing: 4}
	merges := len(forge.MergeRequests())
	carried, err := carry.CarryRearms(context.Background(), false)
	if err != nil {
		t.Fatalf("CarryRearms() error = %v", err)
	}
	if len(carried) != 1 || !carried[0].Carried {
		t.Fatalf("carried = %#v, want the merge armed again by the harness", carried)
	}
	if len(forge.MergeRequests()) != merges+1 {
		t.Fatalf("merge requests = %d, want one arming", len(forge.MergeRequests())-merges)
	}
	armed := loadRun(t, fixture.store, pipelineRunID)
	if !armed.PullRequest.MergeQueued || armed.PullRequest.TargetRed != nil || armed.PublishFailure != "" || armed.PullRequest.MergeRearms != 0 {
		t.Fatalf("record = queued %t, target red %#v, publish failure %q, re-arms %d; want the merge queued again, the wait ended, and no re-arm spent",
			armed.PullRequest.MergeQueued, armed.PullRequest.TargetRed, armed.PublishFailure, armed.PullRequest.MergeRearms)
	}
	if tracker.Record().Blocked {
		t.Error("the item was handed to a person on the way")
	}
}
