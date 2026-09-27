package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// fakeDecidedStops is the harness's hand for a stop she decided: which runs are
// in flight, and every stop it was asked to put to one.
type fakeDecidedStops struct {
	inFlight map[string]bool
	stops    []DecidedStop
	err      error
}

func (f *fakeDecidedStops) Stoppable(_ context.Context, runID string) error {
	if !f.inFlight[runID] {
		return fmt.Errorf("run %s is not in flight — it ended failed — so there is nothing to stop", runID)
	}
	return nil
}

func (f *fakeDecidedStops) Stop(_ context.Context, stop DecidedStop) error {
	if f.err != nil {
		return f.err
	}
	f.stops = append(f.stops, stop)
	return nil
}

// stopOptions is a development manager's conversation wired the way the command
// line wires hers: the durable triage record, the run records, and the hand that
// asks a run to stop.
func stopOptions(t *testing.T, tracker Tracker, budgets TriageBudgets, stops DecidedStops, action string) Options {
	t.Helper()

	options := triageOptions(t, tracker, budgets, trackerReply("Stopping it.", action))
	options.Reports = &fakeReports{}
	options.Stoppages = fakeStoppedRuns{items: map[string]string{stoppedRun: "yoyodyne-ifd.428.34"}}
	if stops != nil {
		options.Stops = stops
	}
	return options
}

func supersededItem() *fakeTracker {
	return &fakeTracker{items: map[string]beads.WorkItem{
		"yoyodyne-ifd.428.34": {ID: "yoyodyne-ifd.428.34", Title: "the item whose run is superseded", Status: "in_progress"},
	}}
}

const stopAction = `{"action":"triage","id":"yoyodyne-ifd.428.34","run":"` + stoppedRun + `","decision":"stop","superseded_by":"yoyodyne-ifd.398","reason":"yoyodyne-ifd.398 delivers this work, so this run would spend a slot and its review rounds on a change that will not land"}`

// She decides a run in flight is superseded, and recording the decision is what
// stops it: the decision lands on the item's durable triage record with the
// superseding item, the run is asked to stop in her name, and the item's notes
// say who stopped it and why.
func TestTheDevelopmentManagerStopsARunInFlightByDecidingIt(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}
	tracker := supersededItem()
	reply := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))

	if len(reply.Actions) != 1 || !reply.Actions[0].Applied {
		t.Fatalf("actions = %#v", reply.Actions)
	}
	decided, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun)
	if !found || decided.Decision != runstate.TriageDecisionStop || decided.SupersededBy != "yoyodyne-ifd.398" {
		t.Fatalf("decision = %#v (found=%t), want the stop and what supersedes it on the triage record", decided, found)
	}
	if decided.Spends() {
		t.Fatal("a stop spends a budget, and it buys no attempt at anything")
	}
	if len(stops.stops) != 1 {
		t.Fatalf("stops = %#v, want the run asked to stop once", stops.stops)
	}
	stop := stops.stops[0]
	if stop.RunID != stoppedRun || stop.WorkItemID != "yoyodyne-ifd.428.34" {
		t.Fatalf("stop = %#v, want the run the decision names", stop)
	}
	if !strings.HasPrefix(stop.RequestedBy, "the development manager in conversation ") {
		t.Fatalf("stop requested by %q, want her named as the one who asked", stop.RequestedBy)
	}
	if !strings.Contains(stop.Reason, "will not land") || !strings.Contains(stop.Reason, "superseded by yoyodyne-ifd.398") {
		t.Fatalf("stop reason = %q, want her reasoning and the superseding item", stop.Reason)
	}
	if len(tracker.updates) != 1 {
		t.Fatalf("updates = %#v, want the stop noted on the item", tracker.updates)
	}
	notes := tracker.updates[0].change.AppendNotes
	for _, want := range []string{"Triaged: stopped in flight, with its change preserved", "run " + stoppedRun, "by the development manager in conversation", "superseded by yoyodyne-ifd.398"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("the item's note is missing %q:\n%s", want, notes)
		}
	}
	rendered := renderTrackerOutcomes(domain.RoleDevelopmentManager, reply.Actions)
	if !strings.Contains(rendered, "asked run "+stoppedRun) || !strings.Contains(rendered, "already decided") {
		t.Fatalf("she was not told what the stop does:\n%s", rendered)
	}
}

// A run that has already ended has a stoppage to decide about, not a run to
// stop. The stop is refused before anything is recorded, so the docket does not
// come to read a decided stoppage the run never had.
func TestAStopIsRefusedForARunThatHasEnded(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	stops := &fakeDecidedStops{}
	tracker := supersededItem()
	reply := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))

	if len(reply.Actions) != 1 || reply.Actions[0].Applied {
		t.Fatalf("actions = %#v, want the stop refused", reply.Actions)
	}
	if !strings.Contains(reply.Actions[0].Failure, "not in flight") {
		t.Fatalf("refusal = %q, want it to say the run is not in flight", reply.Actions[0].Failure)
	}
	if _, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun); found {
		t.Fatal("a refused stop was recorded on the triage record")
	}
	if len(stops.stops) != 0 || len(tracker.updates) != 0 {
		t.Fatalf("a refused stop did something: stops %#v, updates %#v", stops.stops, tracker.updates)
	}
}

// A conversation with no hand to carry a stop out refuses one rather than
// recording a decision nothing will act on.
func TestAStopIsRefusedWhereNothingCanCarryItOut(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	tracker := supersededItem()
	reply := triageSend(t, stopOptions(t, tracker, budgets, nil, stopAction))

	if len(reply.Actions) != 1 || reply.Actions[0].Applied {
		t.Fatalf("actions = %#v, want the stop refused", reply.Actions)
	}
	if _, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun); found {
		t.Fatal("a stop nothing could carry out was recorded as decided")
	}
	if len(tracker.updates) != 0 {
		t.Fatalf("updates = %#v", tracker.updates)
	}
}

// A request the harness could not write leaves the run going and nothing on the
// item's triage record, so no decision stands against a run that goes on, and
// recording the same decision again is what asks again. Both halves are read
// from the real triage record: a retry that the record refused would be a stop
// she has no way to re-issue.
func TestAStopThatCouldNotBeAskedRecordsNothingAndCanBeAskedAgain(t *testing.T) {
	t.Parallel()

	budgets := newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)
	stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}, err: errors.New("the state root is read-only")}
	tracker := supersededItem()
	reply := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))

	if len(reply.Actions) != 1 || reply.Actions[0].Applied {
		t.Fatalf("actions = %#v, want the stop reported as failed", reply.Actions)
	}
	if failure := reply.Actions[0].Failure; !strings.Contains(failure, "could not be asked to stop, so nothing was recorded and the run goes on") ||
		!strings.Contains(failure, "recording the same decision again asks it again") {
		t.Fatalf("failure = %q", failure)
	}
	if _, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun); found {
		t.Fatal("a stop that was never asked for stands on the triage record against a run that goes on")
	}
	if len(tracker.updates) != 0 {
		t.Fatalf("an unmade stop was noted on the item: %#v", tracker.updates)
	}

	// The same decision recorded again, once the request can be written, asks
	// the run and records the stop.
	stops.err = nil
	retried := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))
	if len(retried.Actions) != 1 || !retried.Actions[0].Applied {
		t.Fatalf("retried actions = %#v, want the stop asked again", retried.Actions)
	}
	if len(stops.stops) != 1 || stops.stops[0].RunID != stoppedRun {
		t.Fatalf("stops = %#v, want the retry to have asked the run", stops.stops)
	}
	if decided, found := budgets.counters(t, "yoyodyne-ifd.428.34").DecisionOf(stoppedRun); !found || decided.Decision != runstate.TriageDecisionStop {
		t.Fatalf("decision = %#v (found=%t), want the retried stop recorded", decided, found)
	}

	// And recording it a second time about a run still in flight is not
	// refused: it re-asks, and the later decision supersedes the earlier one.
	again := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))
	if len(again.Actions) != 1 || !again.Actions[0].Applied || len(stops.stops) != 2 {
		t.Fatalf("a repeated stop = %#v (stops %d), want it asked again", again.Actions, len(stops.stops))
	}
}

// refusingDecisions is a triage record that will not take a decision, which is
// the one failure left after the request is written.
type refusingDecisions struct{ *triageBudgetGate }

func (refusingDecisions) RecordDecision(context.Context, string, runstate.TriageDecision) (runstate.TriageCounters, error) {
	return runstate.TriageCounters{}, errors.New("the triage record is locked")
}

// A request written and a decision the record then refused is said as exactly
// that: the run is asked to stop, and what is missing is the record.
func TestAStopTheRecordRefusedSaysTheRunIsStillAskedToStop(t *testing.T) {
	t.Parallel()

	budgets := refusingDecisions{newTriageBudgetGate(t, runstate.TriageCaps{ReviewRounds: 4, RepairGrants: 1, Reruns: 1, MergeRearms: 1}, 2)}
	stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}
	tracker := supersededItem()
	reply := triageSend(t, stopOptions(t, tracker, budgets, stops, stopAction))

	if len(reply.Actions) != 1 || reply.Actions[0].Applied {
		t.Fatalf("actions = %#v, want the failure reported", reply.Actions)
	}
	outcome := reply.Actions[0]
	if !strings.Contains(outcome.Failure, "was not recorded on yoyodyne-ifd.428.34's triage record, though run "+stoppedRun+" is asked to stop") {
		t.Fatalf("failure = %q", outcome.Failure)
	}
	if len(stops.stops) != 1 {
		t.Fatalf("stops = %#v, want the request written", stops.stops)
	}
	if len(outcome.Landed) == 0 || !strings.Contains(outcome.Landed[0], "is asked to stop") {
		t.Fatalf("landed = %#v, want the request named as having landed", outcome.Landed)
	}
}

// The verb is hers. Every other role — the developer and the reviewer among them
// — is refused it whole, and the hand that stops a run is never reached, even
// where one is wired.
func TestOnlyTheDevelopmentManagerMayStopARun(t *testing.T) {
	t.Parallel()

	for _, role := range ConversationalRoles() {
		if role == domain.RoleDevelopmentManager {
			continue
		}
		stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}
		tracker := supersededItem()
		options := stopOptions(t, tracker, nil, stops, stopAction)
		options.Role = role
		options.Agent = string(role)
		_, err := openTestSession(t, options).Send(context.Background(), "Stop the superseded run.")
		authority := &AuthorityError{}
		if !errors.As(err, &authority) {
			t.Fatalf("%s was not refused the stop: %v", role, err)
		}
		if len(stops.stops) != 0 || len(tracker.updates) != 0 {
			t.Fatalf("%s stopped a run: stops %#v, updates %#v", role, stops.stops, tracker.updates)
		}
	}
}

// What supersedes a run is named by a stop and by nothing else, and never as the
// item itself.
func TestOnlyAStopNamesWhatSupersedesIt(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		action string
		want   string
	}{
		{
			name:   "on another decision",
			action: `{"action":"triage","id":"yoyodyne-ifd.428.34","run":"` + stoppedRun + `","decision":"rescope","superseded_by":"yoyodyne-ifd.398","reason":"why"}`,
			want:   `only the "stop" decision names "superseded_by"`,
		},
		{
			name:   "as the item itself",
			action: `{"action":"triage","id":"yoyodyne-ifd.428.34","run":"` + stoppedRun + `","decision":"stop","superseded_by":"yoyodyne-ifd.428.34","reason":"why"}`,
			want:   "a run is not superseded by its own item",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stops := &fakeDecidedStops{inFlight: map[string]bool{stoppedRun: true}}
			tracker := supersededItem()
			reply, err := openTestSession(t, stopOptions(t, tracker, nil, stops, testCase.action)).Send(context.Background(), "Work the docket.")
			if refusal := refusalOf(reply, err); !strings.Contains(refusal, testCase.want) {
				t.Fatalf("refusal = %q, want it to contain %q", refusal, testCase.want)
			}
			if len(stops.stops) != 0 || len(tracker.updates) != 0 {
				t.Fatalf("a refused block did something: stops %#v, updates %#v", stops.stops, tracker.updates)
			}
		})
	}
}
