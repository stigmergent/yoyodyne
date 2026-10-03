package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/checks"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/review"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// Expected classes belong only in the assertion. The scenarios using this
// helper ask the pipeline or sweep to produce and persist the ending itself.
func assertSavedStopClass(t *testing.T, store interface {
	Load(string) (runstate.State, error)
}, runID string, want runstate.StopClass) {
	t.Helper()
	state, err := store.Load(runID)
	if err != nil {
		t.Fatalf("Load(%s) error = %v", runID, err)
	}
	if !state.Status.Terminal() || state.CompletedAt == nil || state.StopClass != want {
		t.Fatalf("saved ending = %q, completed at %v, stop_class = %q; want a terminal run with %q", state.Status, state.CompletedAt, state.StopClass, want)
	}
}

func TestRequiredWorkItemContextRefusalSavesItsStopCause(t *testing.T) {
	t.Parallel()

	tracker := newOutcomeTracker()
	tracker.OnClaim = func() error {
		tracker.Item.Description = strings.Repeat("required context ", 1<<16)
		return nil
	}
	provider := orchestratortest.RoleBackend(writeFeature, approveVerdict)
	pipeline, store := newAutomaticPipeline(t, pipelineRepository(t), tracker, provider, []string{"exit 0"})

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil || !strings.Contains(err.Error(), "assemble claimed work item context: work item context is") {
		t.Fatalf("Run() error = %v, want the required context refused", err)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.StopContextBound)
	if len(provider.Requests) != 0 {
		t.Fatal("context refusal invoked a provider")
	}
}

func TestRequiredReviewContextRefusalSavesItsStopCause(t *testing.T) {
	t.Parallel()

	tracker := newOutcomeTracker()
	provider := orchestratortest.RoleBackend(writeFeature, approveVerdict)
	pipeline, store := newAutomaticPipeline(t, pipelineRepository(t), tracker, provider, []string{"exit 0"})
	// The configured persona is required input too; it is not subject to the
	// optional diff and document excerpt limits.
	pipeline.Reviewer = review.Reviewer{Backend: provider, Model: testReviewerModel, Persona: strings.Repeat("review guidance ", review.MaxReviewInputBytes/8)}

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil || !strings.Contains(err.Error(), "review input") {
		t.Fatalf("Run() error = %v, want the complete review input refused", err)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.StopContextBound)
	if len(provider.RequestsForRole(domain.RoleDeveloper)) != 1 || len(provider.RequestsForRole(domain.RoleReviewer)) != 0 {
		t.Fatal("the oversized review invoked a reviewer or repeated the developer")
	}
}

// Only the candidate save is oversized. The real store rejects it without
// replacing the last valid state, and the pipeline can then save its ending.
type oversizedCandidateStore struct {
	*runstate.Store
	refused bool
}

func (s *oversizedCandidateStore) Save(state runstate.State) error {
	if !s.refused && state.Phase == runstate.PhaseDeveloping {
		s.refused = true
		state.WorkItemLabels = []string{strings.Repeat("x", 1<<20)}
	}
	return s.Store.Save(state)
}

func TestEncodedStateRefusalSavesItsStopCause(t *testing.T) {
	t.Parallel()

	tracker := newOutcomeTracker()
	provider := orchestratortest.RoleBackend(writeFeature, approveVerdict)
	pipeline, store := newAutomaticPipeline(t, pipelineRepository(t), tracker, provider, []string{"exit 0"})
	oversized := &oversizedCandidateStore{Store: store}
	pipeline.Store = oversized

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil || !strings.Contains(err.Error(), "encoded run state is") || !oversized.refused {
		t.Fatalf("Run() error = %v, candidate save refused = %t", err, oversized.refused)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.StopStateBound)
	state, err := store.Load(outcome.RunID)
	if err != nil || len(state.WorkItemLabels) != 0 {
		t.Fatalf("refused candidate replaced the saved state: labels = %v, error = %v", state.WorkItemLabels, err)
	}
}

func TestEncodedEventRefusalSavesItsStopCause(t *testing.T) {
	t.Parallel()

	tracker := newOutcomeTracker()
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		event, err := execution.NewEvent(request.RunID, request.LastSequence+1, time.Now(), execution.EventProcessOutput, "provider", map[string]any{"output": strings.Repeat("x", 1<<20)})
		if err != nil {
			return err
		}
		return request.EventSink(event)
	}, approveVerdict)
	pipeline, store := newAutomaticPipeline(t, pipelineRepository(t), tracker, provider, []string{"exit 0"})

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil || !strings.Contains(err.Error(), "encoded event is") {
		t.Fatalf("Run() error = %v, want the oversized provider event refused", err)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.StopEventBound)
}

func TestDeadClaimAuditSavesItsStopCause(t *testing.T) {
	t.Parallel()

	store, err := runstate.NewStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	state := deadRun(pipelineRunID, "yoyodyne-task")
	state.SchemaVersion = runstate.StateSchemaVersion
	state.ProductID, state.RepositoryID, state.Backend = "yoyodyne", "yoyodyne", "claude-code"
	if err := store.Create(state); err != nil {
		t.Fatal(err)
	}
	harness := newClaimHarness()
	auditor := harness.auditor()
	auditor.Runs = store
	sweep, err := auditor.Audit(context.Background(), []beads.WorkItem{claimedItem(state.WorkItemID, "Recover a dead claim")})
	if err != nil || len(sweep.Problems) != 0 || len(sweep.Released) != 1 {
		t.Fatalf("Audit() = %+v, error = %v; want the claim released", sweep, err)
	}
	assertSavedStopClass(t, store, state.RunID, runstate.StopDeadClaim)
}

type failedIntegrationWorktrees struct {
	WorktreeManager
	failure error
}

func (w failedIntegrationWorktrees) Integrate(context.Context, gitworktree.Worktree, string) (gitworktree.Integration, error) {
	return gitworktree.Integration{}, w.failure
}

func TestIntegrationOperationRefusalsSaveTheirStopCauses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		failure error
		want    runstate.StopClass
	}{
		{"replay killed", gitworktree.ErrReplayKilled, runstate.CauseReplayKilled.StopClass()},
		{"credential refused", gitworktree.ErrRemoteAuthRefused, runstate.CauseRemoteAuthRefused.StopClass()},
		{"transport failed", errors.New("connection reset by peer"), runstate.CauseTransportFailure.StopClass()},
		{"promotion refused", errors.New("promotion refused the candidate"), runstate.StopIntegration},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, tracker, _, pipeline, store := automaticFixture(t)
			pipeline.Worktrees = failedIntegrationWorktrees{WorktreeManager: pipeline.Worktrees, failure: test.failure}
			outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
			if err == nil || outcome.Integration != nil {
				t.Fatalf("Run() error = %v, integration = %+v; want the promotion refused", err, outcome.Integration)
			}
			assertSavedStopClass(t, store, outcome.RunID, test.want)
		})
	}
}

func TestVanishedParkedRunSavesItsStopCause(t *testing.T) {
	t.Parallel()

	repository, tracker, _, pipeline, store := automaticFixture(t)
	holds := newOperatorHoldStore(t)
	pipeline.Holds = holds
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		if _, err := holds.Hold(time.Now()); err != nil {
			return err
		}
		return writeFeature(request)
	}, approveVerdict)
	pipeline.Backend = provider
	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil || !outcome.Paused {
		t.Fatalf("Run() = %+v, error = %v; want the spending hold to park the run", outcome, err)
	}
	if _, _, err := holds.Release(); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(outcome.RunID)
	if err != nil {
		t.Fatal(err)
	}
	reconciler := Reconciler{
		Tracker: tracker, Store: store, Holds: holds,
		Worktrees: newObserver(t, repository, filepath.Dir(state.WorktreePath)),
		Clock:     fixedClock{at: state.UpdatedAt.Add(DefaultVanishedGrace + time.Hour)},
	}
	results, err := reconciler.Reconcile(context.Background())
	if err != nil || len(results) != 1 || results[0].Action != ActionBlocked {
		t.Fatalf("Reconcile() = %+v, error = %v; want the uncontinued park settled", results, err)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.CauseProcessVanished.StopClass())
}

type unavailableCheckProcess struct{}

func (unavailableCheckProcess) Run(context.Context, execution.Command, execution.OutputObserver) (execution.ProcessResult, error) {
	return execution.ProcessResult{Status: execution.ProcessFailed}, errors.New("check worker unavailable")
}

func TestOrdinaryGateFailuresSaveTheirStopCauses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		setup func(*Pipeline, *runstate.Store, *orchestratortest.Backend)
		want  runstate.StopClass
	}{
		{
			name: "check infrastructure failed", want: runstate.StopChecks,
			setup: func(p *Pipeline, _ *runstate.Store, _ *orchestratortest.Backend) {
				p.Checks = checks.Runner{Process: unavailableCheckProcess{}}
			},
		},
		{
			name: "state store temporarily unavailable", want: runstate.StopHarness,
			setup: func(p *Pipeline, store *runstate.Store, _ *orchestratortest.Backend) {
				p.Store = &interruptingStore{StateStore: store, failPhase: runstate.PhaseDeveloping}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, tracker, provider, pipeline, store := automaticFixture(t)
			test.setup(&pipeline, store, provider)
			outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
			if err == nil {
				t.Fatal("Run() succeeded across a failed boundary")
			}
			assertSavedStopClass(t, store, outcome.RunID, test.want)
		})
	}
}

func TestRunContextCancellationSavesItsStopCause(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, tracker, provider, pipeline, store := automaticFixture(t)
	provider.Respond = func(backend.RunRequest) (backend.RunResult, error) {
		cancel()
		return backend.RunResult{Process: execution.ProcessResult{Status: execution.ProcessCancelled}}, context.Canceled
	}
	outcome, err := pipeline.Run(ctx, tracker.Item.ID)
	if err == nil || outcome.Status != runstate.StatusCancelled {
		t.Fatalf("Run() = %+v, error = %v; want the cancelled run to end", outcome, err)
	}
	assertSavedStopClass(t, store, outcome.RunID, runstate.StopCancelled)
}
