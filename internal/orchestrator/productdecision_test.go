package orchestrator

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// inFlightState is a run still in flight, which is what a product decision is
// made about.
func inFlightState() runstate.State {
	state := stoppedState()
	state.Status = runstate.StatusRunning
	state.Phase = runstate.PhaseDeveloping
	state.CompletedAt = nil
	state.Blocker = ""
	state.CheckFailure = nil
	state.ReviewSummary = ""
	state.ReviewFindings = 0
	state.ReviewFindingDetails = nil
	return state
}

func supersession() triage.ProductDecision {
	return triage.ProductDecision{
		Decision:     triage.ProductSuperseded,
		SupersededBy: "yoyodyne-ifd.398",
		Reason:       "398 rebuilds this whole",
		DecidedBy:    "the Lead Product Manager in conversation chat-0123456789abcdef",
	}
}

// A decision about a run in flight is docketed once, carries where the run
// stood, and is read again against the run's own record every time the docket
// is built for her.
func TestAProductDecisionIsDocketedOnceWithWhereTheRunStands(t *testing.T) {
	t.Parallel()

	docket := &memoryDocket{}
	recorder := ProductDecisionDocketer{Docket: docket, Clock: docketClock{}}
	running := inFlightState()
	created, err := recorder.RecordProductDecision(running, supersession())
	if err != nil || !created {
		t.Fatalf("RecordProductDecision() = %t, %v; want the entry created", created, err)
	}
	again, err := recorder.RecordProductDecision(running, supersession())
	if err != nil || again {
		t.Fatalf("RecordProductDecision() again = %t, %v; want the standing entry left as it is", again, err)
	}

	reviewing := running
	reviewing.Phase = runstate.PhaseReviewing
	built, err := docketerOver([]runstate.State{reviewing}, docket).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(built.Entries) != 1 || built.Entries[0].Class != triage.ClassProductDecision {
		t.Fatalf("entries = %#v, want the one product decision", built.Entries)
	}
	decided := built.Entries[0].ProductDecision
	if !decided.RunInFlight || decided.RunPhase != string(runstate.PhaseReviewing) {
		t.Fatalf("decision = %#v, want the run's standing read again at the build", decided)
	}
	rendered := built.Entries[0].Render()
	if !strings.Contains(rendered, "reviewing phase") || !strings.Contains(rendered, "Next mover: you — decide whether run "+docketedRunID+" stops or finishes") {
		t.Fatalf("the entry does not say where the run stands and what she decides:\n%s", rendered)
	}

	ended := running
	ended.Status = runstate.StatusSucceeded
	if _, err := recorder.RecordProductDecision(ended, supersession()); err == nil || !strings.Contains(err.Error(), "not in flight") {
		t.Fatalf("RecordProductDecision(ended) error = %v, want a run that has ended refused", err)
	}
}

// A run that ends before she decides leaves nothing to stop. One that finished
// settles the entry at the next build; one that stopped holding its change is
// docketed as that stoppage, with the product decision folded beneath it for
// her decision about the stoppage to settle.
func TestAProductDecisionWhoseRunEndedIsSettledOrFoldedBeneathItsStoppage(t *testing.T) {
	t.Parallel()

	finished := &memoryDocket{}
	if _, err := (ProductDecisionDocketer{Docket: finished, Clock: docketClock{}}).RecordProductDecision(inFlightState(), supersession()); err != nil {
		t.Fatalf("RecordProductDecision() error = %v", err)
	}
	succeeded := inFlightState()
	succeeded.Status = runstate.StatusSucceeded
	succeeded.Phase = runstate.PhaseComplete
	completed := docketedNow
	succeeded.CompletedAt = &completed
	built, err := docketerOver([]runstate.State{succeeded}, finished).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(built.Entries) != 0 {
		t.Fatalf("entries = %#v, want the decision about a finished run settled", built.Entries)
	}
	closure, closed := finished.closed[triage.ProductDecisionKey(docketedRunID, triage.ProductSuperseded)]
	if !closed || closure.Decision != settledProductDecision || !strings.Contains(closure.Reason, "no run left to stop") {
		t.Fatalf("closure = %#v (closed=%t), want the harness's settlement", closure, closed)
	}

	stopped := &memoryDocket{}
	if _, err := (ProductDecisionDocketer{Docket: stopped, Clock: docketClock{}}).RecordProductDecision(inFlightState(), supersession()); err != nil {
		t.Fatalf("RecordProductDecision() error = %v", err)
	}
	built, err = docketerOver([]runstate.State{stoppedState()}, stopped).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(built.Entries) != 1 || built.Entries[0].Class != triage.ClassStoppedRun {
		t.Fatalf("entries = %#v, want the stoppage as the one live entry", built.Entries)
	}
	if earlier := built.Entries[0].Earlier; len(earlier) != 1 || earlier[0].Class != triage.ClassProductDecision || earlier[0].ProductDecision.RunInFlight {
		t.Fatalf("earlier = %#v, want the product decision beneath the stoppage, its run no longer in flight", earlier)
	}
	if len(stopped.closed) != 0 {
		t.Fatalf("closed = %#v, want nothing settled while the stoppage is hers to decide", stopped.closed)
	}
}
