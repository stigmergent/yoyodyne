package orchestrator

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The class a run is recorded as stopping at is decided from what stopped it and
// never from the evidence it happens to carry. The case this exists for is a run
// the provider killed while a check failure and a refused path were still on its
// record from an earlier round: every one of those looks like a discriminator,
// and reading the first of them as the reason files a provider death as a change
// that failed its checks.
func TestTheStopClassNamesWhatStoppedTheRunRatherThanWhatItCarries(t *testing.T) {
	t.Parallel()

	carrying := runstate.State{
		CheckFailure: &runstate.CheckFailure{Command: "make test", ExitCode: 1},
		PathRefusal:  &runstate.PathRefusal{Paths: []string{"docs/decisions/x.md"}},
	}
	died := stoppedBy(runstate.StopProvider, errors.New("the provider ended this run without judging the work"))

	for _, test := range []struct {
		name   string
		state  runstate.State
		cause  error
		status runstate.Status
		want   runstate.StopClass
	}{
		{name: "a provider death over leftover evidence", state: carrying, cause: died, status: runstate.StatusFailed, want: runstate.StopProvider},
		{name: "a class survives being wrapped again", state: carrying, cause: fmt.Errorf("run ended: %w", died), status: runstate.StatusFailed, want: runstate.StopProvider},
		{name: "a stop nothing classified", state: carrying, cause: errors.New("save running state: disk full"), status: runstate.StatusFailed, want: runstate.StopHarness},
		{name: "a cancellation over the gate it reached", state: carrying, cause: died, status: runstate.StatusCancelled, want: runstate.StopCancelled},
		{
			name:   "the environment over the gate it reached",
			state:  runstate.State{Environmental: &runstate.EnvironmentalRefusal{Cause: runstate.CauseSandboxSpawnFailure}},
			cause:  stoppedBy(runstate.StopChecks, errors.New("verification infrastructure failed")),
			status: runstate.StatusFailed,
			want:   runstate.CauseSandboxSpawnFailure.StopClass(),
		},
		{
			name:   "a recovery window names the bound while environmental accounting stands",
			state:  runstate.State{Environmental: &runstate.EnvironmentalRefusal{Cause: runstate.CauseTransportFailure}},
			cause:  stoppedBy(runstate.StopIntegration, runstate.StopError{Class: runstate.StopRecoveryWindow, Cause: errors.New("recovery waits spent")}),
			status: runstate.StatusFailed,
			want:   runstate.StopRecoveryWindow,
		},
		{
			name:   "a settled environmental record belongs to an earlier round",
			state:  runstate.State{Environmental: &runstate.EnvironmentalRefusal{Cause: runstate.CauseSandboxSpawnFailure, Settled: true}},
			cause:  stoppedBy(runstate.StopReview, errors.New("independent review requires repair")),
			status: runstate.StatusFailed,
			want:   runstate.StopReview,
		},
	} {
		run := &activeRun{state: test.state}
		if got := run.classifyStop(test.cause, test.status); got != test.want {
			t.Errorf("%s: classifyStop() = %q, want %q", test.name, got, test.want)
		}
	}
	if stoppedBy(runstate.StopChecks, nil) != nil {
		t.Error("stoppedBy() made an error out of nothing")
	}
}

// Exercise the real terminal write for every recognized cause. Reading old
// evidence cannot change the class chosen by the ending, even across save/load.
func TestEveryStopClassIsSavedWhenARunEnds(t *testing.T) {
	t.Parallel()
	for _, class := range runstate.StopClasses() {
		if class == runstate.StopUnknown {
			continue // Unknown is a reading of an old record, not a new ending.
		}
		t.Run(class.Name(), func(t *testing.T) {
			store, err := runstate.NewStore(t.TempDir(), "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			state := runstate.State{SchemaVersion: runstate.StateSchemaVersion,
				RunID: pipelineRunID, ProductID: "yoyodyne", RepositoryID: "yoyodyne",
				WorkItemID: "yoyodyne-task", Backend: "claude-code",
				Status: runstate.StatusRunning, StartedAt: now, UpdatedAt: now}
			if err := store.Create(state); err != nil {
				t.Fatal(err)
			}
			run := &activeRun{pipeline: Pipeline{Store: store}, state: state}
			cause := stoppedBy(class, errors.New("the bound refused this run"))
			if environmental := runstate.EnvironmentalCause(class); environmental.Valid() {
				run.state.Environmental = &runstate.EnvironmentalRefusal{Cause: environmental, RecordedAt: now}
				cause = stoppedBy(runstate.StopChecks, errors.New("a gate met the environment's refusal"))
			}
			outcome, _ := run.fail(cause, runstate.StatusFailed)
			stored, err := store.Load(state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !stored.Status.Terminal() || stored.StopClass != class || outcome.StopClass != class {
				t.Fatalf("ending = %q, saved cause = %q, outcome cause = %q; want %q", stored.Status, stored.StopClass, outcome.StopClass, class)
			}
		})
	}
}

func TestTheInnermostBoundSurvivesGateAndRecordingErrors(t *testing.T) {
	t.Parallel()
	run := &activeRun{}
	bound := runstate.StopError{Class: runstate.StopEventBound, Cause: errors.New("event exceeded its bound")}
	cause := stoppedBy(runstate.StopProvider, withFailedRecord(bound, errors.New("could not record it")))
	if got := run.classifyStop(cause, runstate.StatusFailed); got != runstate.StopEventBound {
		t.Fatalf("class = %q, want the event bound", got)
	}
}

func TestReconciliationKeepsTheProviderClockAndDoesNotBorrowAnOlderCause(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		environment *runstate.EnvironmentalRefusal
		want        runstate.StopClass
	}{
		{name: "a vanished process", want: runstate.CauseProcessVanished.StopClass()},
		{name: "an earlier checkout refusal", environment: &runstate.EnvironmentalRefusal{Cause: runstate.CauseWorktreeCheckoutKilled, Settled: true}, want: runstate.CauseProcessVanished.StopClass()},
		{name: "provider silence", environment: &runstate.EnvironmentalRefusal{Cause: runstate.CauseProcessVanished, ProviderStop: runstate.ProviderStopStalled, Settled: true}, want: runstate.StopProviderIdle},
		{name: "provider budget", environment: &runstate.EnvironmentalRefusal{Cause: runstate.CauseProcessVanished, ProviderStop: runstate.ProviderStopBudgetExhausted, Settled: true}, want: runstate.StopProviderBudget},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := runstate.NewStore(t.TempDir(), "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if test.environment != nil {
				test.environment.RecordedAt = now
			}
			state := runstate.State{SchemaVersion: runstate.StateSchemaVersion,
				RunID: pipelineRunID, ProductID: "yoyodyne", RepositoryID: "yoyodyne",
				WorkItemID: "yoyodyne-task", Backend: "claude-code",
				Status: runstate.StatusRunning, StartedAt: now, UpdatedAt: now,
				Environmental: test.environment}
			if err := store.Create(state); err != nil {
				t.Fatal(err)
			}
			if _, err := (Reconciler{Store: store}).saveTerminalFailure(state, "no process held this run"); err != nil {
				t.Fatal(err)
			}
			stored, err := store.Load(state.RunID)
			if err != nil || stored.StopClass != test.want || !stored.Status.Terminal() {
				t.Fatalf("saved ending = %+v, error %v; want %s", stored, err, test.want)
			}
		})
	}
}
