package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/publish"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A re-arm the development manager records about a merge the forge dropped is
// carried out by the watch's own pull, exactly as the verb would make it: the
// pull request the verdict authorized, by the method its own merge recorded,
// pinned to the promoted commit. Nobody types `yoyo triage rearm`, and the pull
// after it finds nothing left to carry out. yoyodyne-ifd.428.46.
func TestTheWatchRearmsADroppedMergeItsDecisionNames(t *testing.T) {
	t.Parallel()

	harness := newRearmHarness(t)
	harness.decide(t)
	watch := harness.carryOut(nil)

	carried, err := watch.CarryRearms(context.Background(), false)
	if err != nil {
		t.Fatalf("CarryRearms() error = %v", err)
	}
	if len(carried) != 1 || !carried[0].Carried || carried[0].RunID != harness.state.RunID || carried[0].Decision != runstate.TriageDecisionRearm {
		t.Fatalf("carried = %+v, want the re-arm of the dropped merge carried out", carried)
	}
	want := publish.MergeRequest{Number: 92, HeadCommit: rearmedCommit, Method: publish.MergeMethod(rearmedMethod)}
	if len(harness.forge.requested) != 1 || harness.forge.requested[0] != want {
		t.Fatalf("merge requests = %#v, want the dropped request repeated as %#v", harness.forge.requested, want)
	}
	if len(harness.leases.promoted) != 1 || harness.leases.promoted[0] != "main" {
		t.Fatalf("promotion leases = %#v, want the re-arm made under main's", harness.leases.promoted)
	}
	rearmed := harness.reload(t)
	if !rearmed.PullRequest.MergeQueued || rearmed.PullRequest.MergeRearms != 1 || rearmed.PublishFailure != "" {
		t.Fatalf("recorded publication = %+v (failure %q), want the merge queued again and the decision spent", rearmed.PullRequest, rearmed.PublishFailure)
	}

	again, err := watch.CarryRearms(context.Background(), false)
	if err != nil || len(again) != 0 || len(harness.forge.requested) != 1 {
		t.Fatalf("a second pull carried %+v (%v) with requests %#v; want nothing left to carry out", again, err, harness.forge.requested)
	}
}

// A re-arm of a dropped merge the forge still holds on a requirement only a
// person can meet is refused by the action's own gate, and the refusal is written
// onto the item naming it, then left to cool rather than asked every poll.
func TestTheWatchRecordsARefusedRearmOfADroppedMergeOnTheItem(t *testing.T) {
	t.Parallel()

	harness := newRearmHarness(t)
	harness.forge.status = "BLOCKED"
	harness.decide(t)
	watch := harness.carryOut(nil)

	carried, err := watch.CarryRearms(context.Background(), false)
	if err != nil {
		t.Fatalf("CarryRearms() error = %v", err)
	}
	if len(carried) != 1 || carried[0].Carried || !strings.Contains(carried[0].Problem, "only a person can satisfy") {
		t.Fatalf("carried = %+v, want the re-arm refused naming what holds the merge", carried)
	}
	if len(harness.forge.requested) != 0 {
		t.Fatalf("a refused re-arm asked the forge to merge %#v", harness.forge.requested)
	}
	counters, err := harness.runs.Triage().Counters(harness.state.WorkItemID)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	finding, found := counters.CarryOutOf(harness.state.RunID)
	if !found || finding.Decision != runstate.TriageDecisionRearm || finding.Waiting || !strings.Contains(finding.Refusal, "only a person can satisfy") {
		t.Fatalf("finding = %+v (found %v), want the refusal on the item's triage record", finding, found)
	}
	if again, _ := watch.CarryRearms(context.Background(), false); len(again) != 0 {
		t.Fatalf("the next pull attempted %+v; want the refusal left to cool", again)
	}
}

// The intake hold and the operator's pause stop a re-arm as they stop a repair or
// a re-run: nothing is asked of the forge, the item says once what the decision is
// waiting on rather than on every poll, and the first pull after the switch opens
// carries the decision out.
func TestASwitchShutForEverythingIsWrittenOnceOnARearmAndTheFirstPullAfterFiresIt(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		intakeHeld bool
		paused     bool
		gate       string
	}{
		{name: "intake hold", intakeHeld: true, gate: runstate.TriageGateIntakeHold},
		{name: "operator pause", paused: true, gate: runstate.TriageGateSpendingPause},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			harness := newRearmHarness(t)
			harness.decide(t)
			watch := harness.carryOut(nil)
			watch.Holds = pausedHolds{held: test.paused, hold: runstate.OperatorHold{HeldAt: docketedNow}}

			held, err := watch.CarryRearms(context.Background(), test.intakeHeld)
			if err != nil {
				t.Fatalf("CarryRearms() under the switch error = %v", err)
			}
			if len(held) != 1 || held[0].Carried || !held[0].Waiting || held[0].Gate != test.gate {
				t.Fatalf("held = %+v, want the re-arm said to wait on %s", held, test.gate)
			}
			if len(harness.forge.requested) != 0 {
				t.Fatalf("a re-arm under %s asked the forge for %#v", test.gate, harness.forge.requested)
			}
			counters, err := harness.runs.Triage().Counters(harness.state.WorkItemID)
			if err != nil {
				t.Fatalf("Counters() error = %v", err)
			}
			finding, found := counters.CarryOutOf(harness.state.RunID)
			if !found || !finding.Waiting || finding.Gate != test.gate || finding.Attempts != 1 {
				t.Fatalf("finding = %+v (found %v), want one waiting record naming %s", finding, found, test.gate)
			}

			if again, _ := watch.CarryRearms(context.Background(), test.intakeHeld); len(again) != 0 {
				t.Fatalf("a second pull under the same switch attempted %+v; want the record left standing", again)
			}

			watch.Holds = pausedHolds{}
			carried, err := watch.CarryRearms(context.Background(), false)
			if err != nil {
				t.Fatalf("CarryRearms() after the switch opened error = %v", err)
			}
			if len(carried) != 1 || !carried[0].Carried || len(harness.forge.requested) != 1 {
				t.Fatalf("carried = %+v with requests %#v, want the re-arm made on the first pull after the switch opened", carried, harness.forge.requested)
			}
		})
	}
}

// A re-arm held back because a run of its item is in flight is attempted by no
// pass, so it is written onto the item as unattempted once it has stood a poll
// interval, naming the run it waits on.
func TestARearmHeldBehindARunInFlightIsRecordedAsUnattempted(t *testing.T) {
	t.Parallel()

	harness := newRearmHarness(t)
	live := droppedPublication()
	live.RunID = "run-aaaabbbbccccddddeeeeffff00001111"
	live.Status = runstate.StatusRunning
	live.CompletedAt = nil
	live.Blocker = ""
	live.PublishFailure = ""
	live.Integration = nil
	live.PullRequest = nil
	if err := harness.runs.Create(live); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	harness.decide(t)
	watch := harness.carryOut(nil)

	carried, err := watch.CarryRearms(context.Background(), false)
	if err != nil || len(carried) != 0 || len(harness.forge.requested) != 0 {
		t.Fatalf("CarryRearms() = %+v, %v with requests %#v; want nothing attempted beside a run in flight", carried, err, harness.forge.requested)
	}

	watch.Clock = laterClock{after: 2 * time.Minute}
	written, err := watch.RecordUnattempted(context.Background(), time.Minute, nil)
	if err != nil {
		t.Fatalf("RecordUnattempted() error = %v", err)
	}
	if len(written) != 1 || written[0].Decision != runstate.TriageDecisionRearm || !strings.Contains(written[0].Problem, live.RunID) {
		t.Fatalf("written = %+v, want the re-arm recorded as unattempted naming run %s", written, live.RunID)
	}
	counters, err := harness.runs.Triage().Counters(harness.state.WorkItemID)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	finding, found := counters.CarryOutOf(harness.state.RunID)
	if !found || !finding.Unattempted || finding.Gate != runstate.TriageGateWorkItem {
		t.Fatalf("finding = %+v (found %v), want the unattempted re-arm on the item's triage record", finding, found)
	}
}
