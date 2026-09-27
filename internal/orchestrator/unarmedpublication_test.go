package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/publish"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// checksStub is the forge's reading of a request's checks, which gates arming a
// request nothing ever asked the forge to merge.
type checksStub struct {
	reading publish.CheckReading
	err     error
	asked   int
}

func (c *checksStub) Checks(context.Context, int, string) (publish.CheckReading, error) {
	c.asked++
	return c.reading, c.err
}

// unarmedPublicationRun is a finished, approved run whose promotion holds its pull
// request and whose record says nothing ever asked the forge to merge it: no
// merge queued, none dropped, no method recorded, and no account of anything
// having gone wrong. It ended a minute before the docket is built, well inside
// the stuck-merge age, because what puts it on the docket is not its age.
func unarmedPublicationRun() runstate.State {
	state := droppedPublication()
	completed := docketedNow.Add(-time.Minute)
	state.Status = runstate.StatusSucceeded
	state.UpdatedAt = completed
	state.CompletedAt = &completed
	state.PublishFailure = ""
	state.Blocker = ""
	state.MergeDrop = nil
	state.PullRequest.MergeMethod = ""
	return state
}

// newUnarmedHarness records one unarmed publication and builds the docket over
// it the way every sweep does, rather than writing the entry by hand: the docket
// finding it is half of what is under test.
func newUnarmedHarness(t *testing.T) (*rearmHarness, *checksStub, DocketBuild) {
	t.Helper()
	root := t.TempDir()
	runs, err := runstate.NewStore(root, "yoyodyne")
	if err != nil {
		t.Fatalf("runstate.NewStore() error = %v", err)
	}
	state := unarmedPublicationRun()
	if err := runs.Create(state); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := runs.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !state.PublicationUnarmed() {
		t.Fatalf("the fixture is not a publication nothing asked the forge to merge: %+v", state)
	}
	docket := &memoryDocket{}
	build, err := docketerOverStore(docket, runs, rearmConfig()).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	harness := &rearmHarness{
		docket: docket,
		runs:   runs,
		leases: &leasedRuns{Store: runs},
		forge: &forgeStub{
			observed: publish.PullRequest{Number: state.PullRequest.Number, URL: state.PullRequest.URL, State: "OPEN", HeadCommit: rearmedCommit},
			status:   "CLEAN",
			result:   publish.MergeResult{Queued: true},
		},
		worktrees: &remoteTargetStub{},
		state:     state,
	}
	checks := &checksStub{reading: publish.CheckReading{HeadCommit: rearmedCommit, Passing: 4}}
	return harness, checks, build
}

func (h *rearmHarness) armer(checks RearmChecks) Rearmer {
	rearmer := h.rearmer()
	rearmer.Checks = checks
	return rearmer
}

// A publication nothing ever asked the forge to merge is put to the development
// manager at once, and her re-arm decision is carried out by the harness as the
// merge request the run's own merge would have made — the same method, pinned to
// the promoted commit, after the same pre-merge check, under the target branch's
// promotion lease — and nothing about it is left for a hand merge on the forge.
func TestAPublicationNothingAskedTheForgeToMergeIsDocketedAndArmedByTheHarness(t *testing.T) {
	t.Parallel()

	harness, checks, build := newUnarmedHarness(t)

	if len(build.Entries) != 1 {
		t.Fatalf("docket = %+v, want the one unarmed publication on it", build.Entries)
	}
	entry := build.Entries[0]
	if entry.Class != triage.ClassPublication || entry.Key != harness.publication() || entry.RunID != harness.state.RunID {
		t.Fatalf("entry = %+v, want the publication keyed to run %s and pull request 92", entry, harness.state.RunID)
	}
	if entry.Publication == nil || !strings.Contains(entry.Publication.Message, "nothing ever asked the forge to merge pull request 92 into main") ||
		!strings.Contains(entry.Publication.Message, "a re-run hands the change back for a fresh run") {
		t.Fatalf("entry publication = %+v, want the account naming the two decisions", entry.Publication)
	}

	harness.decide(t)
	result, err := harness.armer(checks).Rearm(context.Background(), RearmRequest{Run: harness.state.RunID, Reason: rearmReasoning})
	if err != nil {
		t.Fatalf("Rearm() error = %v", err)
	}
	want := publish.MergeRequest{Number: 92, HeadCommit: rearmedCommit, Method: mergeMethod}
	if len(harness.forge.requested) != 1 || harness.forge.requested[0] != want {
		t.Fatalf("merge requests = %#v, want the run's own merge request %#v", harness.forge.requested, want)
	}
	if checks.asked != 1 || len(harness.worktrees.verified) != 1 || len(harness.leases.promoted) != 1 {
		t.Fatalf("checks read %d, remote target verified %d, leases %v; want each once before the merge", checks.asked, len(harness.worktrees.verified), harness.leases.promoted)
	}
	if !result.FirstArm || !result.Rearmed || result.Method != string(mergeMethod) {
		t.Fatalf("result = %+v, want a first arming by the run's own method", result)
	}
	for _, said := range []string{result.Reason, result.Render()} {
		if !strings.Contains(said, "nothing had") {
			t.Fatalf("account %q does not say nothing had asked the forge before", said)
		}
	}
	armed := harness.reload(t)
	if !armed.PullRequest.MergeQueued || armed.PullRequest.MergeMethod != string(mergeMethod) || armed.PullRequest.MergeRearms != 1 {
		t.Fatalf("recorded publication = %+v, want the merge queued, its method recorded, and the decision spent", armed.PullRequest)
	}
	if armed.PublicationUnarmed() {
		t.Fatal("the armed publication still reads as one nothing asked the forge to merge")
	}
}

// The arming is refused where the request's head is behind its target or its
// checks fail, naming the gate, and refused before anything is spent.
func TestArmingAnUnaskedPublicationIsRefusedAtTheLandingGates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		reading publish.CheckReading
		want    string
	}{
		{name: "head behind target", reading: publish.CheckReading{HeadCommit: rearmedCommit, BehindBy: 3}, want: "head-behind-target gate: pull request 92's head is 3 commit(s) behind main"},
		{name: "failing check", reading: publish.CheckReading{HeadCommit: rearmedCommit, Failing: []publish.FailedCheck{{Name: "make race"}}}, want: "checks gate: pull request 92 has 1 failing check(s) on its head (make race)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			harness, checks, _ := newUnarmedHarness(t)
			checks.reading = test.reading
			harness.decide(t)
			_, err := harness.armer(checks).Rearm(context.Background(), RearmRequest{Run: harness.state.RunID, Reason: rearmReasoning})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Rearm() error = %v, want it refused naming %q", err, test.want)
			}
			if len(harness.forge.requested) != 0 {
				t.Fatalf("a refused arming asked the forge for %#v", harness.forge.requested)
			}
			if left := harness.reload(t); left.PullRequest.MergeRearms != 0 || !left.PublicationUnarmed() {
				t.Fatalf("a refused arming changed the record: %+v", left.PullRequest)
			}
		})
	}

	// A harness that cannot read the checks does not arm unchecked.
	harness, _, _ := newUnarmedHarness(t)
	harness.decide(t)
	if _, err := harness.armer(nil).Rearm(context.Background(), RearmRequest{Run: harness.state.RunID, Reason: rearmReasoning}); err == nil ||
		!strings.Contains(err.Error(), "armed only on a reading of them") {
		t.Fatalf("Rearm() without a checks reading error = %v, want it refused", err)
	}
	if len(harness.forge.requested) != 0 {
		t.Fatalf("an unchecked arming asked the forge for %#v", harness.forge.requested)
	}
}

// The other decision is a re-run, which hands the change back for a fresh run:
// the re-run finds the publication's entry and the run's record admits it, so a
// re-run decided about it is not refused for want of a stoppage.
func TestAnUnaskedPublicationCanBeHandedBackForAFreshRun(t *testing.T) {
	t.Parallel()

	harness, _, _ := newUnarmedHarness(t)
	entry, err := Rerunner{Docket: harness.docket}.entry(harness.state.RunID)
	if err != nil {
		t.Fatalf("entry() error = %v", err)
	}
	if entry.Class != triage.ClassPublication || entry.Key != harness.publication() {
		t.Fatalf("entry = %+v, want the publication's entry", entry)
	}
	if err := stoppageIsOver(harness.state); err != nil {
		t.Fatalf("stoppageIsOver() = %v, want an unarmed publication admitted to a re-run", err)
	}
	// A publication something did ask the forge about is not admitted this way.
	queued := harness.state
	published := *queued.PullRequest
	published.MergeQueued = true
	queued.PullRequest = &published
	if err := stoppageIsOver(queued); err == nil {
		t.Fatal("stoppageIsOver() admitted a queued merge to a re-run")
	}
}
