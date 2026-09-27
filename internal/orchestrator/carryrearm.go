package orchestrator

// Arming a publication nothing ever asked the forge to merge, on the development
// manager's decision, with nobody typing a verb.
//
// A promoted, approved run whose record holds its pull request and says nothing
// asked the forge to merge it is docketed at once, and she decides it: a re-arm,
// or a re-run. A re-run is fired by the ordinary carry-out, because it is a run
// and takes a developer slot. A re-arm is not a run. It is one merge request,
// made by the Rearmer under the target branch's promotion lease, and it holds
// no slot and leaves no run to wait out — so it is fired here, on its own path,
// once per pull, rather than dressed up as a started run the pass would then
// account for as one.
//
// Until yoyodyne-ifd.429.31 nothing fired it, and the only exit from that state
// was a person merging the request on the forge. The gates are the Rearmer's
// own — the forge's merge state, a head behind its target, a failing check, the
// pre-merge check on the remote target, the decision standing and not yet
// carried out — and a refusal is written onto the item's triage record exactly
// as a refused re-run's is, where the development manager reads it, and paced
// the same way. The operator's pause and the intake hold stop it as they stop
// every other carry-out: while either stands nothing is attempted, and the
// switch is on the status line in its own right.
//
// A re-arm of a merge the forge dropped is deliberately not fired here. That is
// a repeat of a request the forge's machinery already refused once, and it stays
// `yoyo triage rearm`'s; widening this to it is its own decision.

import (
	"context"
	"errors"
	"fmt"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// CarryRearms fires every re-arm decision standing about a publication nothing
// ever asked the forge to merge that the harness has not acted on, and reports
// what became of each attempt. intakeHeld is the pull's own reading of the
// intake hold, which a carry-out honours as every other one does.
func (c CarryOut) CarryRearms(ctx context.Context, intakeHeld bool) ([]CarriedOut, error) {
	if c.Rearmer == nil {
		return nil, nil
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	if intakeHeld {
		return nil, nil
	}
	if _, held, err := c.paused(); err != nil || held {
		return nil, err
	}
	tasks, err := c.outstandingRearms()
	carried := make([]CarriedOut, 0, len(tasks))
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		carried = append(carried, c.carryRearm(ctx, task))
	}
	return carried, err
}

// outstandingRearms is every re-arm decision the sweep above would fire: the
// run's publication still nothing asked the forge to merge, the decision
// standing about that run a re-arm, the publication's budget carrying one the
// harness has not made, no run of the item in flight, and no earlier attempt
// whose refusal is still cooling.
func (c CarryOut) outstandingRearms() ([]CarryOutTask, error) {
	entries, err := c.Docket.List()
	if err != nil {
		return nil, fmt.Errorf("read the triage docket: %w", err)
	}
	candidates := make(map[string]bool)
	for _, entry := range entries {
		if entry.Class == triage.ClassPublication && entry.RunID != "" && entry.WorkItemID != "" {
			candidates[entry.RunID] = true
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	inFlight, err := c.itemsInFlight()
	if err != nil {
		return nil, err
	}
	recorded, err := c.Runs.Recorded()
	if err != nil {
		return nil, fmt.Errorf("read the recorded runs to find the publications nothing asked the forge to merge: %w", err)
	}
	now := c.now()
	var tasks []CarryOutTask
	var problems []error
	for _, state := range recorded {
		if !candidates[state.RunID] || !state.PublicationUnarmed() {
			continue
		}
		delete(candidates, state.RunID)
		if _, busy := inFlight[state.WorkItemID]; busy {
			continue
		}
		counters, err := c.Decisions.Counters(state.WorkItemID)
		if err != nil {
			problems = append(problems, fmt.Errorf("read what triage has decided about %s: %w", state.WorkItemID, err))
			continue
		}
		decision, found := counters.DecisionOf(state.RunID)
		if !found || decision.Decision != runstate.TriageDecisionRearm {
			continue
		}
		key := triage.PublicationKey(state.RunID, state.PullRequest.Number)
		if counters.RearmsOf(key) <= state.PullRequest.MergeRearms {
			continue
		}
		if stopped, refused := counters.CarryOutOf(state.RunID); refused && stopped.Cooling(now) {
			continue
		}
		tasks = append(tasks, CarryOutTask{
			WorkItemID: state.WorkItemID,
			RunID:      state.RunID,
			DocketKey:  key,
			Decision:   decision.Decision,
			Reason:     decision.Reason,
			DecidedAt:  decision.DecidedAt,
		})
	}
	return tasks, errors.Join(problems...)
}

// carryRearm makes the one merge request a decision authorizes, and writes a
// refusal onto the item where the development manager reads it.
func (c CarryOut) carryRearm(ctx context.Context, task CarryOutTask) CarriedOut {
	carried := CarriedOut{
		WorkItemID: task.WorkItemID,
		RunID:      task.RunID,
		DocketKey:  task.DocketKey,
		Decision:   task.Decision,
	}
	result, err := c.Rearmer.Rearm(ctx, RearmRequest{Run: task.RunID, Reason: task.Reason})
	if err != nil || !result.Rearmed {
		return c.stopped(ctx, task, carried, runstate.TriageGateHarness, false, refusalText(err),
			"what the refusal names: a head brought level with its target, checks that pass, or a forge that can be read again — nothing was spent where the refusal says so, and the decision is attempted again once the refusal has cooled; a re-run is the other decision, and hands the change back for a fresh run")
	}
	carried.Carried = true
	carried.Reason = result.Reason
	carried.RecordProblem = result.RecordProblem
	return carried
}
