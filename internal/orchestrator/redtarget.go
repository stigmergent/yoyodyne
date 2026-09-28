package orchestrator

// A queued merge whose checks fail on the target rather than on the change.
//
// A head level with its target has nothing but the change between it and the
// target, so a failing check that names no file the change touches failed on
// something the target already carries. Until yoyodyne-m5p that was handed to a
// person as a dropped merge, whatever the check named: on 2026-09-28 at 04:15Z
// the adoption check was red on main itself, pull request 863 was withdrawn and
// its item handed to a person, and every merge queued behind it would have met
// the same check and been handed over the same way. A required check red on the
// target, or flaky there, became a hand step per queued merge.
//
// The harness already knows the other shape of this failure. A red landing is
// news about the target branch, not a verdict on the run that landed it: it
// files one p0 bug per target branch and check, under the goal the landed item
// served, and blocks nothing. This is the same filing for the same fact met from
// the other side. The queued merge is withdrawn, one p0 item is filed per target
// branch and failing check — or, where one is already open for that check on
// that branch, this request is noted on it — and the publication is recorded as
// waiting on those items with the harness as the one to move, rather than handed
// to anybody. The work item is made to wait on them in the tracker too.
//
// What ends the wait is the harness's as well. Once every item it waits on is
// closed, a head the fix left behind the target is brought up to date from the
// kept branch by the reconciling sweep, checked, reviewed, and queued again, as
// a queued head behind its target is (ResumeRedTargets); and a head still level
// with a target whose checks now pass is re-armed by the watch's re-arm
// carry-out, with nobody deciding anything (CarryRearms).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/goal"
	"github.com/mason-bryant/yoyodyne/internal/oneline"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// ActionWaitingOnTarget reports a queued merge the sweep withdrew because its
// checks failed on the target rather than on the change: the failure is filed as
// the target's, and the publication waits on the item filed for it.
const ActionWaitingOnTarget ReconcileAction = "waiting-on-target"

// ReconcileJobLogs reads the tail of the forge's log of the job behind a check
// run. It is satisfied by publish.GitHub.
type ReconcileJobLogs interface {
	JobLogTail(ctx context.Context, checkRun int64, lines int) (string, error)
}

// redTargetLogLines is how much of a failing job's log the filed item carries.
const redTargetLogLines = 60

// redTargetMarker is the line an item filed for a check red on the target
// carries in its notes, naming the check and the branch, which is what a later
// request meeting the same check finds it by. It is the red landing's marker in
// shape, and a different line, because a landing check is a command the harness
// ran and this is a check the forge ran.
func redTargetMarker(target, check string) string {
	return "Red forge check: " + check + " on " + target
}

// waitOnRedTarget settles a queued merge whose checks fail on a head level with
// its target, on no file its change touches, as the target's failure. dropped is
// a merge the forge already stopped holding, which has nothing to withdraw.
//
// Nothing is written until everything the wait names exists. The work item is
// read for the goal it served, and every failing check is filed or found, before
// the merge is withdrawn; a failure at any of those leaves the merge where it was
// and the next sweep asks again, finding by its marker whatever this one filed.
// A sweep wired with nothing to file through hands the merge back as it always
// did.
func (r Reconciler) waitOnRedTarget(ctx context.Context, state runstate.State, checks runstate.PullRequestChecks, dropped bool) (Reconciliation, error) {
	published := *state.PullRequest
	target := state.Integration.TargetBranch
	describe := checks.Describe(target)
	if r.Filer == nil {
		return r.handBackRedMerge(ctx, state, fmt.Sprintf(
			"the forge's checks on pull request %d fail with its head level with %s, so nothing but this change differs from the target and bringing it up to date would change nothing: %s. Nothing is wired to this harness to file the target's failure as its own item, so the harness withdrew the queued merge rather than leave a red change queued, and the pull request needs a person",
			published.Number, target, describe))
	}
	left := func(why string) (Reconciliation, error) {
		result := reconciliationOf(state, ActionQueued)
		stays := "the merge is left queued"
		if dropped {
			stays = "the record is left as it stands"
		}
		result.Detail = fmt.Sprintf("the forge's checks on pull request %d fail on %s itself rather than on this change (%s), and %s, so %s and the next sweep asks again",
			published.Number, target, describe, why, stays)
		return result, nil
	}
	item, err := r.Tracker.Show(ctx, state.WorkItemID)
	if err != nil {
		return left(fmt.Sprintf("the goal %s served could not be read to file the target's failure under (%v)", state.WorkItemID, err))
	}
	statement, _ := goal.NamedIn(item.Notes)
	waiting := runstate.TargetRed{At: r.clock().Now(), TargetBranch: target, HeadCommit: checks.HeadCommit}
	for _, failing := range checks.Failing {
		filed, err := r.fileRedTargetCheck(ctx, state, failing, target, statement)
		if err != nil {
			return left(fmt.Sprintf("the failure of %s could not be filed (%v)", failing.Name, err))
		}
		waiting.Checks = append(waiting.Checks, filed)
	}
	if !dropped {
		if err := r.Checks.DisableAutoMerge(ctx, published.Number); err != nil {
			return reconciliationOf(state, ActionUnsettled), fmt.Errorf("withdraw the queued merge of pull request %d, whose checks fail on %s itself, for run %s: %w", published.Number, target, state.RunID, err)
		}
	}
	reason := fmt.Sprintf("the forge's checks on pull request %d fail with its head level with %s on no file this change touches, so the failure is %s's rather than this change's: %s. The harness filed it as the target's and the publication %s; it moves by itself once those close — re-armed on a level head whose checks pass, or brought up to date from the kept branch where the fix left the head behind — and nothing here needs a person",
		published.Number, target, target, describe, waiting.Describe())
	// The work item is told, and made to wait on the filed items, before the
	// record changes: a sweep that stops in between leaves the merge recorded as
	// queued, and the next one finds it withdrawn, files nothing new, and writes
	// the record then.
	if _, err := r.Tracker.RecordOutcome(ctx, state.WorkItemID, renderRedTargetNotes(state, reason)); err != nil {
		return reconciliationOf(state, ActionUnsettled), fmt.Errorf("record the wait of run %s on %s's red check: %w", state.RunID, target, err)
	}
	var problems []string
	for _, blocker := range waiting.WaitingOn() {
		if err := r.Tracker.AddBlocker(ctx, state.WorkItemID, blocker); err != nil {
			problems = append(problems, fmt.Sprintf("%s could not be made to wait on %s in the tracker (%v); the publication's own record still holds it", state.WorkItemID, blocker, err))
		}
	}
	published.MergeQueued = false
	published.Checks = &checks
	published.TargetRed = &waiting
	state.PullRequest = &published
	state.PublishFailure = oneline.Bound(reason, runstate.MaxSelectionReasonBytes)
	state.UpdatedAt = r.clock().Now()
	if err := r.Store.Save(state); err != nil {
		return reconciliationOf(state, ActionUnsettled), fmt.Errorf("record the wait of run %s on %s's red check: %w", state.RunID, target, err)
	}
	result := reconciliationOf(state, ActionWaitingOnTarget)
	result.Detail = reason
	if len(problems) > 0 {
		result.Detail += "; " + strings.Join(problems, "; ")
	}
	return result, nil
}

// fileRedTargetCheck files the item one check red on the target is, or finds the
// one already open for it and notes this request on it. It is fileRedLanding's
// filing for a check the forge ran: priority 0, a bug, under the goal the run's
// item served, with the harness's own words in the fields the protected-path
// gate reads and what the forge's log said only in the notes.
func (r Reconciler) fileRedTargetCheck(ctx context.Context, state runstate.State, failing runstate.FailingCheck, target, statement string) (runstate.TargetRedCheck, error) {
	published := state.PullRequest
	marker := redTargetMarker(target, failing.Name)
	head := shortCommit(published.HeadCommit)
	existing, err := openItemMarked(ctx, r.Filer, marker)
	if err != nil {
		return runstate.TargetRedCheck{}, fmt.Errorf("read whether an item is already open for it: %w", err)
	}
	if existing != "" {
		note := fmt.Sprintf("Red again on %s: pull request %d of %s (%s), level with %s at %s, failed %s on no file its change touches. Its merge waits on this item.",
			target, published.Number, state.WorkItemID, state.RunID, target, head, failing.Name)
		if _, err := r.Tracker.RecordOutcome(ctx, existing, note); err != nil {
			return runstate.TargetRedCheck{}, fmt.Errorf("tell the open item %s about pull request %d: %w", existing, published.Number, err)
		}
		return runstate.TargetRedCheck{Name: failing.Name, WorkItem: existing, FiledEarlier: true}, nil
	}
	ended := nonEmpty(failing.Conclusion, "no conclusion reported")
	description := fmt.Sprintf("The forge's check %s is red on %s itself: pull request %d of %s (%s), queued to merge with its head at %s level with %s, failed it on no file its change touches, so nothing but that change differed from %s and the failure is the target's.\n\n"+
		"The harness withdrew that merge and it waits on this item, as every request queued behind it that meets the same check does. The forge ended the check as: %s. What the forge's log of the job said is in this item's notes; the run is %s on the forge.",
		failing.Name, target, published.Number, state.WorkItemID, state.RunID, head, target, target, ended, published.URL)
	notes := fmt.Sprintf("Filed by the harness for %s red on %s, met by pull request %d of %s (%s) at %s, as a red landing files its own item.\n%s",
		failing.Name, target, published.Number, state.WorkItemID, state.RunID, head, marker)
	if statement != "" {
		notes += "\n\n" + goal.Note(statement)
	}
	notes += "\n\n" + r.jobLogAccount(ctx, failing)
	priority := 0
	created, err := r.Filer.Create(ctx, beads.NewWorkItem{
		Title:       fmt.Sprintf("Red forge check on %s: %s fails with the head level with %s, met by pull request %d", target, failing.Name, target, published.Number),
		Description: description,
		Type:        "bug",
		Notes:       notes,
		Priority:    &priority,
	})
	if err != nil {
		return runstate.TargetRedCheck{}, err
	}
	return runstate.TargetRedCheck{Name: failing.Name, WorkItem: created.ID}, nil
}

// jobLogAccount is what the filed item's notes say of the job's log: its
// conclusion, and the tail of the log the harness read through its own forge
// access, quoted so no line of it is read as one of the harness's. A log that
// could not be read says why, which is where a person would go next.
func (r Reconciler) jobLogAccount(ctx context.Context, failing runstate.FailingCheck) string {
	account := fmt.Sprintf("How the forge ended %s: %s.", failing.Name, nonEmpty(failing.Conclusion, "no conclusion reported"))
	if len(failing.Paths) > 0 {
		account += " Its annotations named: " + strings.Join(failing.Paths, ", ") + "."
	}
	switch {
	case r.JobLogs == nil:
		return account + " Nothing is wired to this harness to read the job's log."
	case failing.CheckRun <= 0:
		return account + " The forge named no job for it, so there is no log to read."
	}
	tail, err := r.JobLogs.JobLogTail(ctx, failing.CheckRun, redTargetLogLines)
	if err != nil {
		return account + fmt.Sprintf(" Its log could not be read: %s.", oneline.Bound(err.Error(), 400))
	}
	if strings.TrimSpace(tail) == "" {
		return account + " Its log was empty."
	}
	return account + fmt.Sprintf("\n\nThe last %d lines of the job's log (check run %d):\n\n%s", redTargetLogLines, failing.CheckRun, quotedOutput(tail))
}

// openItemMarked is the identifier of an unfinished item whose notes carry the
// marker line, or nothing. It reads the queue's three unfinished statuses, as a
// red landing's lookup does, because an item somebody has claimed or that is
// blocked is still the item that answers the check being red.
func openItemMarked(ctx context.Context, filer WorkFiler, marker string) (string, error) {
	for _, status := range []string{"open", "in_progress", "blocked"} {
		items, err := filer.List(ctx, status)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			for _, line := range strings.Split(item.Notes, "\n") {
				if strings.TrimSpace(line) == marker {
					return item.ID, nil
				}
			}
		}
	}
	return "", nil
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// renderRedTargetNotes tells the work item its queued merge waits on the
// target's red check rather than on anything about the change.
func renderRedTargetNotes(state runstate.State, reason string) string {
	return strings.Join([]string{
		"Yoyodyne withdrew the merge this run left queued with the forge, because its checks fail on the target branch itself rather than on this change.",
		"Outcome: " + reason,
		"Run: " + state.RunID,
		fmt.Sprintf("Pull request: #%d %s", state.PullRequest.Number, state.PullRequest.URL),
		"Nothing about this item needs a decision: the change stays reviewed on its kept branch, and the harness takes the merge up again once the items it waits on close.",
	}, "\n")
}

// RedTargetResumption is what the sweep did about one publication waiting on
// its target's red check.
type RedTargetResumption struct {
	RunID      string `json:"run_id"`
	WorkItemID string `json:"work_item_id"`
	// Action is what came of it: still waiting, brought up to date, filed
	// again, or left for the watch to re-arm.
	Action  ReconcileAction `json:"action"`
	Detail  string          `json:"detail,omitempty"`
	Failure string          `json:"failure,omitempty"`
}

// ResumeRedTargets takes up every publication waiting on its target's red check
// whose items have all closed. The fix having landed leaves the head behind the
// target, and that is brought up to date from the kept branch — checked,
// reviewed, and queued again, as a queued head behind its target is, and hosted
// by ContinueUpdates. A head still level whose checks still fail on no file the
// change touches is filed again as the target's, since the items that answered
// it closed with the check still red. A head level and passing is left for the
// watch's re-arm carry-out, which arms it with nobody deciding.
//
// A publication with any item still open waits, and says so; nothing is asked of
// the forge for it.
func (r Reconciler) ResumeRedTargets(ctx context.Context) ([]RedTargetResumption, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if r.Checks == nil {
		return nil, nil
	}
	recorded, err := r.Store.Recorded()
	if err != nil {
		return nil, fmt.Errorf("read the recorded runs to find the publications waiting on a red target: %w", err)
	}
	var results []RedTargetResumption
	var problems []error
	for _, candidate := range recorded {
		if !candidate.WaitingOnRedTarget() {
			continue
		}
		result := RedTargetResumption{RunID: candidate.RunID, WorkItemID: candidate.WorkItemID}
		open, err := r.openRedTargetItems(ctx, *candidate.PullRequest.TargetRed)
		if err != nil {
			result.Action = ActionUnsettled
			result.Failure = err.Error()
			problems = append(problems, err)
			results = append(results, result)
			continue
		}
		if len(open) > 0 {
			result.Action = ActionWaitingOnTarget
			result.Detail = fmt.Sprintf("pull request %d still waits on %s", candidate.PullRequest.Number, strings.Join(open, ", "))
			results = append(results, result)
			continue
		}
		reconciliation, err := r.resumeRedTarget(ctx, candidate.RunID)
		result.Action, result.Detail = reconciliation.Action, reconciliation.Detail
		if err != nil {
			result.Failure = err.Error()
			problems = append(problems, err)
		}
		results = append(results, result)
	}
	return results, errors.Join(problems...)
}

// resumeRedTarget reads the checks of one publication whose items have closed,
// under the run's own lease, and decides on them.
func (r Reconciler) resumeRedTarget(ctx context.Context, runID string) (Reconciliation, error) {
	state, lease, err := r.Store.AdoptRun(ctx, runID)
	if err != nil {
		return Reconciliation{RunID: runID, Action: ActionUnsettled}, fmt.Errorf("take run %s to take up its wait on a red target: %w", runID, err)
	}
	defer func() { _ = lease.Release() }()
	if !state.WaitingOnRedTarget() {
		result := reconciliationOf(state, ActionQueued)
		result.Detail = "the publication stopped waiting on its target while this sweep was reading it, so it is left as it stands"
		return result, nil
	}
	published := *state.PullRequest
	target := state.Integration.TargetBranch
	reading, err := r.Checks.Checks(ctx, published.Number, target)
	if err != nil {
		result := reconciliationOf(state, ActionWaitingOnTarget)
		result.Detail = fmt.Sprintf("every item pull request %d waited on is closed, and its checks could not be read (%v), so it is left waiting and the next sweep asks again", published.Number, err)
		return result, nil
	}
	checks := recordedChecks(reading, r.clock().Now())
	switch {
	case checks.ChangeFails():
		return r.handBackRedMerge(ctx, state, fmt.Sprintf(
			"the items pull request %d waited on for %s's red check are closed, and its checks now fail on this change: %s. The pull request needs its change repaired",
			published.Number, target, checks.Describe(target)))
	case checks.BehindBy > 0:
		if refusal := unreplayable(state); refusal != "" {
			return r.handBackRedMerge(ctx, state, fmt.Sprintf(
				"the items pull request %d waited on for %s's red check are closed and its head is behind %s, and the harness cannot bring it up to date: %s: %s. The pull request needs a person",
				published.Number, target, target, refusal, checks.Describe(target)))
		}
		published.Checks = &checks
		state.PullRequest = &published
		result := reconciliationOf(state, ActionWaitingOnTarget)
		result.Detail = fmt.Sprintf("every item pull request %d waited on for %s's red check is closed; %s", published.Number, target, checks.Describe(target))
		return r.updateQueuedHead(ctx, state, result, true)
	case checks.Red() && !checks.FailedInTheJob():
		return r.waitOnRedTarget(ctx, state, checks, true)
	case checks.Red():
		return r.rerunEndedJobsAfterRedTarget(ctx, state, checks)
	default:
		result := reconciliationOf(state, ActionWaitingOnTarget)
		result.Detail = fmt.Sprintf("every item pull request %d waited on for %s's red check is closed and its head is level with %s (%s), so the watch's re-arm carry-out arms its merge at its next pull",
			published.Number, target, target, checks.Describe(target))
		return result, nil
	}
}

// rerunEndedJobsAfterRedTarget decides a closed wait whose level head is red
// only on jobs the forge ended itself — cancelled, timed out, or never started.
// Nothing in the tree decided those, so they are neither the target's failure to
// file again nor a head the re-arm can arm: they are run again on the same head
// within runstate.MaxCheckReruns, as a merge still queued has its ended jobs
// run again, and the wait stands meanwhile, so the next sweep reads the re-run.
// Ended again past the bound, or refused a re-run, the merge is handed back
// saying the forge ended it, as a queued one is.
func (r Reconciler) rerunEndedJobsAfterRedTarget(ctx context.Context, state runstate.State, checks runstate.PullRequestChecks) (Reconciliation, error) {
	published := *state.PullRequest
	target := state.Integration.TargetBranch
	if prior := published.Checks; prior != nil && prior.HeadCommit == checks.HeadCommit {
		checks.Reruns = prior.Reruns
		checks.RerunChecks = append([]int64(nil), prior.RerunChecks...)
	}
	record := func(detail string) (Reconciliation, error) {
		published.Checks = &checks
		state.PullRequest = &published
		state.UpdatedAt = r.clock().Now()
		if err := r.Store.Save(state); err != nil {
			return reconciliationOf(state, ActionUnsettled), fmt.Errorf("record the checks of pull request %d on run %s: %w", published.Number, state.RunID, err)
		}
		result := reconciliationOf(state, ActionWaitingOnTarget)
		result.Detail = detail
		return result, nil
	}
	answered := fmt.Sprintf("every item pull request %d waited on for %s's red check is closed; %s", published.Number, target, checks.Describe(target))
	if checks.AwaitingRerun() {
		return record(answered + "; the forge has not yet started the re-run the harness asked for, so it goes on waiting and the next sweep reads it")
	}
	refused := ""
	if checks.Reruns < runstate.MaxCheckReruns {
		if refused = r.rerunFailedJobs(ctx, checks); refused == "" {
			checks.RecordRerun()
			return record(fmt.Sprintf("%s; the forge ended the failed jobs before any step failed, so the harness asked it to run them again (%d of %d on this head) and the next sweep reads the re-run",
				answered, checks.Reruns, runstate.MaxCheckReruns))
		}
	}
	why := fmt.Sprintf("were ended by the forge before any step failed, and ended that way again on each of %d re-run(s) of this head", checks.Reruns)
	if refused != "" {
		why = fmt.Sprintf("were ended by the forge before any step failed, and the forge would not run them again (%s)", refused)
	}
	return r.handBackRedMerge(ctx, state, fmt.Sprintf(
		"the items pull request %d waited on for %s's red check are closed, and its checks %s: %s. The forge's log of those runs says why it ended them, and the pull request needs a person",
		published.Number, target, why, checks.Describe(target)))
}

// openRedTargetItems is the items a wait on a red target still waits on.
func (r Reconciler) openRedTargetItems(ctx context.Context, waiting runstate.TargetRed) ([]string, error) {
	var open []string
	for _, id := range waiting.WaitingOn() {
		item, err := r.Tracker.Show(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("read whether %s, filed for a red check on %s, is closed: %w", id, waiting.TargetBranch, err)
		}
		if !strings.EqualFold(item.Status, "closed") {
			open = append(open, id)
		}
	}
	return open, nil
}
