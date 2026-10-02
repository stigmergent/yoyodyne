package orchestrator

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

type countedRepair struct {
	RepairContinuer
	attempts int
}

func (c *countedRepair) Continue(ctx context.Context, request RepairContinueRequest) (RepairContinueResult, error) {
	c.attempts++
	return c.RepairContinuer.Continue(ctx, request)
}

func TestAPermanentRepairRefusalIsRecordedOnceAndNeverRetried(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		remains   gitworktree.Survival
		headError error
		cause     triage.CarryOutCause
	}{
		{name: "retired worktree", remains: gitworktree.Survival{BranchExists: true}, cause: triage.CarryOutWorktreeGone},
		{name: "deleted branch", remains: gitworktree.Survival{WorktreePresent: true}, cause: triage.CarryOutBranchGone},
		{name: "moved HEAD", remains: gitworktree.Survival{BranchExists: true, WorktreePresent: true}, headError: gitworktree.ErrOwnedHeadMoved, cause: triage.CarryOutHeadMoved},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newContinueHarness(t, continuableState())
			continuer := harness.continuer()
			continuer.Remains = &looked{survival: test.remains}
			harness.ownership.err = test.headError
			repairer := &countedRepair{RepairContinuer: continuer}
			watch := harness.carryOut()
			watch.Repairer = repairer
			watch.Notes = harness.tracker
			watch.Clock = laterClock{after: time.Minute}
			harness.docket.close(triage.Key(triage.ClassStoppedRun, docketedRunID), runstate.TriageDecisionRepair, docketedNow)

			carried, _, err := watch.Carry(context.Background(), theOneOutstanding(t, watch))
			if err != nil || carried.Carried || carried.Cause != test.cause || carried.RecordProblem != "" {
				t.Fatalf("Carry() = %+v, %v; want the permanent refusal recorded", carried, err)
			}
			for _, after := range []time.Duration{time.Hour, 24 * time.Hour, 7 * 24 * time.Hour} {
				watch.Clock = laterClock{after: after}
				if outstanding, err := watch.Outstanding(); err != nil || len(outstanding) != 0 {
					t.Fatalf("after %s Outstanding() = %+v, %v; want no second attempt", after, outstanding, err)
				}
				if written, err := watch.RecordUnattempted(context.Background(), time.Minute, nil); err != nil || len(written) != 0 {
					t.Fatalf("RecordUnattempted() = %+v, %v; want the refusal left intact", written, err)
				}
			}
			counters, err := harness.runs.Triage().Counters(docketedItem)
			if err != nil || len(counters.CarryOuts) != 1 || counters.CarryOuts[0].Attempts != 1 || repairer.attempts != 1 {
				t.Fatalf("counters = %+v, %v, attempts = %d; want one gate and one attempt", counters, err, repairer.attempts)
			}
			if harness.carried(t) != 0 || len(harness.started) != 0 || len(harness.tracker.NoteRecords) != 1 {
				t.Fatalf("started = %+v, notes = %+v; want no spend and one item note", harness.started, harness.tracker.NoteRecords)
			}
			for _, want := range []string{counters.CarryOuts[0].Refusal, "will not clear on its own", "re-run", "escalation"} {
				if !strings.Contains(harness.tracker.Notes, want) {
					t.Fatalf("item note %q lacks %q", harness.tracker.Notes, want)
				}
			}
			entries, _ := harness.docket.List()
			docket := Docketer{Decisions: harness.runs.Triage(), Reruns: harness.runs.Reruns()}
			if problems := docket.joinDecisions(entries, docketedRunsOf(entries), nil, nil); len(problems) != 0 {
				t.Fatal(problems)
			}
			if len(entries) != 1 || entries[0].CarryOut == nil || entries[0].CarryOut.Cause != test.cause || !entries[0].Critical() {
				t.Fatalf("entries = %+v; want the one permanent gate on her docket", entries)
			}
			if rendered := entries[0].Render(); !strings.Contains(rendered, "no further attempt") || strings.Contains(rendered, "keeps trying it") {
				t.Fatalf("entry promises the wrong next step: %s", rendered)
			}
		})
	}
}

type countedRearmer struct {
	Rearmer
	attempts int
}

func (c *countedRearmer) Rearm(ctx context.Context, request RearmRequest) (RearmResult, error) {
	c.attempts++
	return c.Rearmer.Rearm(ctx, request)
}

func TestAnUnmakeableRearmWaitsForANewDecision(t *testing.T) {
	t.Parallel()
	harness := newRearmHarness(t)
	harness.decide(t)
	harness.docket.close(harness.publication(), runstate.TriageDecisionRearm, docketedNow)
	harness.forge.statusErr = UnrearmablePublicationError{RunID: harness.state.RunID, Number: 92, Why: "this publication cannot be made from its recorded promotion"}
	rearmer := &countedRearmer{Rearmer: harness.rearmer()}
	watch := harness.carryOut(nil)
	watch.Rearmer = rearmer
	watch.Clock = laterClock{after: time.Minute}
	carried, err := watch.CarryRearms(context.Background(), false)
	if err != nil || len(carried) != 1 || carried[0].Cause != triage.CarryOutPublicationUnmakeable {
		t.Fatalf("CarryRearms() = %+v, %v; want a permanent forge refusal", carried, err)
	}
	// Even a changed environment cannot revive the old decision.
	harness.forge.statusErr = nil
	watch.Clock = laterClock{after: 7 * 24 * time.Hour}
	if again, err := watch.CarryRearms(context.Background(), false); err != nil || len(again) != 0 || rearmer.attempts != 1 {
		t.Fatalf("later pull = %+v, %v, attempts %d; want no second attempt", again, err, rearmer.attempts)
	}
	if entry := refusedOnTheDocket(t, harness); entry.CarryOut.Cause != triage.CarryOutPublicationUnmakeable || entry.CarryOut.Attempts != 1 {
		t.Fatalf("docket entry = %+v; want one permanent gate", entry)
	}
	caps := rearmCaps
	caps.MergeRearms = 2
	if _, err := harness.runs.Triage().RecordMergeRearm(context.Background(), harness.state.WorkItemID, harness.publication(),
		triageDecided(runstate.TriageDecisionRearm, harness.state.RunID), docketedNow.Add(8*24*time.Hour), caps); err != nil {
		t.Fatal(err)
	}
	watch.Clock = laterClock{after: 8*24*time.Hour + time.Minute}
	rearmer.Clock = watch.Clock
	if again, err := watch.CarryRearms(context.Background(), false); err != nil || len(again) != 1 || !again[0].Carried || rearmer.attempts != 2 {
		t.Fatalf("new decision = %+v, %v, attempts %d; want it attempted", again, err, rearmer.attempts)
	}
}

func TestAnUnpromotedRearmIsNotAttemptedAgainAfterItsRefusal(t *testing.T) {
	t.Parallel()
	harness := newRearmHarness(t)
	state := harness.state
	state.Integration = nil
	if err := harness.runs.Save(state); err != nil {
		t.Fatal(err)
	}
	harness.decide(t)
	watch := harness.carryOut(nil)
	watch.Clock = laterClock{after: time.Minute}
	first, err := watch.CarryRearms(context.Background(), false)
	if err != nil || len(first) != 1 || first[0].Cause != triage.CarryOutPublicationUnmakeable {
		t.Fatalf("first = %+v, %v", first, err)
	}
	watch.Clock = laterClock{after: 24 * time.Hour}
	if again, err := watch.CarryRearms(context.Background(), false); err != nil || len(again) != 0 {
		t.Fatalf("later = %+v, %v; want no retry of an unpromoted publication", again, err)
	}
}

func TestPermanentCarryOutCausesAreTypedAndUnreadableWorkKeepsItsPacing(t *testing.T) {
	t.Parallel()
	for _, cause := range triage.CarryOutCauses() {
		if got := carryOutCause(permanentCarryOut(cause, errors.New("the original refusal"))); got != cause {
			t.Fatalf("cause = %q, want %q", got, cause)
		}
	}
	for _, err := range []error{
		errors.New("worktree HEAD moved, according to untrusted text"),
		WorktreeSurgeryError{Cause: errors.New("reading the worktree timed out")},
		continuableRepair(continuableState(), triage.Found{Unknown: true}),
	} {
		if got := carryOutCause(err); got != "" {
			t.Fatalf("%v was permanently classified as %q; a failed reading may clear", err, got)
		}
	}
}

func TestPermanentCarryOutCausesAreNamedInTheRunStopInventory(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../docs/run-stops.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, cause := range triage.CarryOutCauses() {
		if !strings.Contains(string(body), "`"+string(cause)+"`") {
			t.Errorf("the run-stop inventory does not name %s", cause)
		}
	}
}

func TestAPermanentRefusalIsDeliveredAfterTheOriginalStoppageWasAlreadyShown(t *testing.T) {
	t.Parallel()
	state := reviewStoppedState(docketedRunID, docketedItem)
	judge := &standingJudge{judgment: Judgment{ConversationID: "chat-abc"}}
	escalator := escalatorOver(t, []runstate.State{state}, judge, nil)
	clock := &movingClock{now: escalationNow}
	escalator.Clock = clock
	if sweep, err := escalator.Escalate(context.Background()); err != nil || len(sweep.Escalated) != 1 {
		t.Fatalf("original delivery = %+v, %v", sweep, err)
	}
	store, err := runstate.NewTriageStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	decided := escalationNow.Add(time.Minute)
	if _, err := store.GrantRepair(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRepair, docketedRunID), 1, decided, continueCaps); err != nil {
		t.Fatal(err)
	}
	key := triage.Key(triage.ClassStoppedRun, docketedRunID)
	escalator.Docket.(*memoryDocket).close(key, runstate.TriageDecisionRepair, decided)
	refused := decided.Add(time.Minute)
	if _, err := store.RecordCarryOutRefusal(context.Background(), docketedItem, runstate.TriageCarryOut{
		RunID: docketedRunID, Decision: runstate.TriageDecisionRepair, DecidedAt: decided,
		Cause: triage.CarryOutWorktreeGone, Gate: runstate.TriageGatePreservedWork,
		Refusal: "the preserved checkout was retired", Clears: "the development manager records a re-run or an escalation",
	}, refused); err != nil {
		t.Fatal(err)
	}
	escalator.Decisions = store
	clock.now = refused.Add(time.Minute)
	sweep, err := escalator.Escalate(context.Background())
	if err != nil || len(sweep.Escalated) != 1 || !sweep.Escalated[0].Delivered || len(judge.shown) != 2 {
		t.Fatalf("permanent refusal delivery = %+v, %v; shown %d", sweep, err, len(judge.shown))
	}
	if shown := judge.shown[1]; shown.CarryOut == nil || shown.CarryOut.Refusal != "the preserved checkout was retired" || shown.CarryOut.Cause != triage.CarryOutWorktreeGone {
		t.Fatalf("shown = %+v; want the refusal's words on the docket entry", shown)
	}
	clock.now = refused.Add(7 * 24 * time.Hour)
	if again, err := escalator.Escalate(context.Background()); err != nil || len(again.Escalated) != 0 || len(judge.shown) != 2 {
		t.Fatalf("second delivery = %+v, %v; shown %d, want the gate delivered once", again, err, len(judge.shown))
	}
}

// Old budgets held the spend without recording the decision. Model that read
// while keeping the ordinary durable refusal writer and the real actions.
type legacyCarryDecisions struct{ *runstate.TriageStore }

func (s legacyCarryDecisions) Counters(id string) (runstate.TriageCounters, error) {
	counters, err := s.TriageStore.Counters(id)
	counters.Decisions = nil
	return counters, err
}

func TestAPreDecisionRecordGrantIsDocketedOnceWithoutAuthorizingARepair(t *testing.T) {
	t.Parallel()
	harness := newContinueHarness(t, continuableState())
	watch := harness.carryOut()
	legacy := legacyCarryDecisions{harness.runs.Triage()}
	watch.Decisions = legacy
	watch.Notes = harness.tracker
	repairer := &countedRepair{RepairContinuer: harness.continuer()}
	watch.Repairer = repairer
	watch.Clock = laterClock{after: time.Minute}
	first, _, err := watch.Carry(context.Background(), theOneOutstanding(t, watch))
	if err != nil || first.Carried || first.Cause != triage.CarryOutDecisionMissing || repairer.attempts != 0 {
		t.Fatalf("old grant = %+v, %v, action attempts %d; want only the missing decision refused", first, err, repairer.attempts)
	}
	watch.Clock = laterClock{after: 7 * 24 * time.Hour}
	if tasks, err := watch.Outstanding(); err != nil || len(tasks) != 0 {
		t.Fatalf("legacy grant was offered again: %+v, %v", tasks, err)
	}
	counters, err := legacy.Counters(docketedItem)
	finding, found := counters.RefusedCarryOut(docketedRunID)
	refused, _ := counters.CarryOutFindings()
	if err != nil || !found || finding.Attempts != 1 || refused != 1 || len(harness.tracker.NoteRecords) != 1 {
		t.Fatalf("legacy refusal = %+v, %v; want one recorded gate and one note", counters, err)
	}
	entries, _ := harness.docket.List()
	docket := Docketer{Decisions: legacy, Reruns: harness.runs.Reruns()}
	if problems := docket.joinDecisions(entries, docketedRunsOf(entries), nil, nil); len(problems) != 0 || entries[0].CarryOut == nil || entries[0].CarryOut.Cause != triage.CarryOutDecisionMissing || !entries[0].Critical() {
		t.Fatalf("legacy docket = %+v, %v; want the permanent missing-decision gate", entries, problems)
	}
}
