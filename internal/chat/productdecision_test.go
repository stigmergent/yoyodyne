package chat

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

const proceedAction = `{"action":"triage","id":"yoyodyne-ifd.428.34","run":"` + stoppedRun + `","decision":"proceed","reason":"it is in review already, and the header it builds is the part 398 keeps"}`

// Letting the run finish is the other answer to the Lead Product Manager's
// decision: nothing is asked of the run, the decision lands on the item's triage
// record and its notes, and the docket entry that put the question is closed.
// It decides no stoppage, so a stoppage the run reaches afterwards is still hers
// to decide.
func TestTheDevelopmentManagerLetsARunInFlightFinish(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}
	docket := &fakeDocket{closes: 1}
	tracker := supersededItem()
	options := stopOptions(t, tracker, budgets, stops, proceedAction)
	options.Docket = docket
	reply := triageSend(t, options)

	if len(reply.Actions) != 1 || !reply.Actions[0].Applied {
		t.Fatalf("actions = %#v", reply.Actions)
	}
	if len(stops.stops) != 0 {
		t.Fatalf("stops = %#v, want nothing asked of the run", stops.stops)
	}
	counters := budgets.counters(t, "yoyodyne-ifd.428.34")
	decided, found := counters.DecisionOf(stoppedRun)
	if !found || decided.Decision != runstate.TriageDecisionProceed || decided.Spends() {
		t.Fatalf("decision = %#v (found=%t), want a proceed that spends nothing", decided, found)
	}
	if counters.Standing(stoppedRun).Decided {
		t.Fatal("letting a run finish reads as deciding a stoppage it never reached")
	}
	if len(docket.closed) != 1 || docket.closed[0].Decision != "proceed" || docket.closed[0].Classes[0] != triage.ClassProductDecision {
		t.Fatalf("closed = %#v, want the product decision closed by it", docket.closed)
	}
	if len(tracker.updates) != 1 || !strings.Contains(tracker.updates[0].change.AppendNotes, "Triaged: left to finish in flight") {
		t.Fatalf("updates = %#v, want the decision noted on the item", tracker.updates)
	}
}

// Letting a run that has already ended finish is refused before anything is
// recorded, as a stop of it is.
func TestLettingARunThatHasEndedFinishIsRefused(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	docket := &fakeDocket{closes: 1}
	tracker := supersededItem()
	options := stopOptions(t, tracker, budgets, &fakeDecidedStops{}, proceedAction)
	options.Docket = docket
	reply := triageSend(t, options)

	if len(reply.Actions) != 1 || reply.Actions[0].Applied || !strings.Contains(reply.Actions[0].Failure, "not in flight") {
		t.Fatalf("actions = %#v, want the proceed refused for a run that has ended", reply.Actions)
	}
	if _, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun); found || len(docket.closed) != 0 || len(tracker.updates) != 0 {
		t.Fatalf("a refused proceed did something: closed %#v, updates %#v", docket.closed, tracker.updates)
	}
}

// A stop she decides closes the product decision that asked for it.
func TestAStopClosesTheProductDecisionItAnswers(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	docket := &fakeDocket{closes: 1}
	options := stopOptions(t, supersededItem(), budgets, &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}, stopAction)
	options.Docket = docket
	reply := triageSend(t, options)

	if len(reply.Actions) != 1 || !reply.Actions[0].Applied {
		t.Fatalf("actions = %#v", reply.Actions)
	}
	if len(docket.closed) != 1 || docket.closed[0].Decision != "stop" || docket.closed[0].Classes[0] != triage.ClassProductDecision {
		t.Fatalf("closed = %#v, want the product decision closed by the stop", docket.closed)
	}
}

// What the action carries is checked before anything is looked up: a decision
// from the vocabulary, and the superseding item named on a supersession and
// never as the item itself.
func TestAProductDecisionIsCheckedBeforeItIsCarriedOut(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, action, want string
	}{
		{"no decision", `{"action":"inflight","id":"yoyodyne-ifd.428.34","reason":"r"}`, `inflight requires "decision"`},
		{"unknown decision", `{"action":"inflight","id":"yoyodyne-ifd.428.34","decision":"stop","reason":"r"}`, `inflight decision "stop" is not a decision`},
		{"supersession naming nothing", `{"action":"inflight","id":"yoyodyne-ifd.428.34","decision":"superseded","reason":"r"}`, `names the item doing the work instead`},
		{"superseded by itself", `{"action":"inflight","id":"yoyodyne-ifd.428.34","decision":"superseded","superseded_by":"yoyodyne-ifd.428.34","reason":"r"}`, `not superseded by itself`},
		{"no reason", `{"action":"inflight","id":"yoyodyne-ifd.428.34","decision":"retired"}`, `reason`},
		{"a run named", `{"action":"inflight","id":"yoyodyne-ifd.428.34","run":"` + stoppedRun + `","decision":"retired","reason":"r"}`, `inflight does not take "run"`},
	} {
		_, _, _, err := extractTrackerActions("```yoyodyne-tracker\n{\"actions\":[" + tc.action + "]}\n```")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want it to mention %q", tc.name, err, tc.want)
		}
	}
}
