package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/checks"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

func TestEachCheckScalesWithLoadAndStaysInsideTheStage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                           string
		load                           float64
		configured, stage, takes, want time.Duration
		passed                         bool
	}{
		{"idle", 8, 30 * time.Minute, 2 * time.Hour, 45 * time.Minute, 30 * time.Minute, false},
		{"high load completes a long check", 48, 30 * time.Minute, 2 * time.Hour, 45 * time.Minute, 90 * time.Minute, true},
		{"tenfold ceiling", 1600, 30 * time.Minute, 2 * time.Hour, 45 * time.Minute, 5 * time.Hour, true},
		{"stage caps the check", 48, 2 * time.Hour, 30 * time.Minute, 2 * time.Hour, 90 * time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repository := pipelineRepository(t)
			tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
			provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
				return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
			}, approveVerdict)
			pipeline, store := newAutomaticPipeline(t, repository, tracker, provider, []string{"make race"})
			clock := &steppingClock{now: time.Now().UTC().Add(time.Hour)}
			pipeline.Config.Execution.CheckStageTimeout = config.Duration(tc.stage)
			pipeline.Checks = checks.Runner{Process: &timedChecks{clock: clock, takes: map[string]time.Duration{"make race": tc.takes}}, Clock: clock, Timeout: tc.configured}
			pipeline.Load = func() (float64, int, bool) { return tc.load, 16, true }
			outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
			if (err == nil) != tc.passed {
				t.Fatalf("Run() = %v, want passed %v", err, tc.passed)
			}
			if len(outcome.Checks) != 1 || outcome.Checks[0].Timeout != tc.want || outcome.Checks[0].ConfiguredTimeout != tc.configured {
				t.Fatalf("checks = %#v, want configured %s scaled to %s", outcome.Checks, tc.configured, tc.want)
			}
			state, loadErr := store.Load(outcome.RunID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			stage := state.CheckStage
			if stage == nil || stage.Content == "" || stage.CheckBoundSeconds > stage.BoundSeconds || stage.Load != tc.load || stage.CheckConfiguredSeconds != int64(tc.configured/time.Second) || state.CheckTimeAllowance == nil {
				t.Fatalf("stage = %#v, allowance = %#v", stage, state.CheckTimeAllowance)
			}
			if !tc.passed {
				if state.ChecksPassed != nil || state.CheckFailure != nil || state.RepairAttempts != 0 || !state.HarnessContinuesCheckStage() || state.WorktreeRemoved || state.BranchRemoved || state.ProviderSessionID == "" {
					t.Fatalf("timeout did not preserve the unfinished change: %#v", state)
				}
				for _, want := range []string{"configured", "scaled", "make race", stage.Content, "allowance reserved", "cause remains unresolved"} {
					if !strings.Contains(state.Failure, want) {
						t.Fatalf("failure %q lacks %q", state.Failure, want)
					}
				}
				active := activeRun{pipeline: pipeline, state: state, worktree: worktreeOf(state)}
				// Even otherwise valid evidence from an older stage cannot make
				// an unfinished stage satisfy the integration gate.
				active.state.ChecksPassed = &runstate.ChecksPassed{Content: stage.Content, Attempt: state.RepairAttempts, Commit: state.HarnessCommit}
				active.state.ReviewDecision = runstate.ReviewApprove
				if err := active.integrationEarned(context.Background()); !errors.Is(err, ErrIntegrationUnearned) {
					t.Fatalf("unfinished gate = %v", err)
				}
			}
		})
	}
}

func TestRemainingCheckTimeAllowanceCapsAStageBelowItsConfiguredLimit(t *testing.T) {
	t.Parallel()
	stage := &runstate.CheckStage{StartedAt: time.Now(), ConfiguredSeconds: 1800, BoundSeconds: 5400, LoadScaledSeconds: 5400, Load: 48, Cores: 16}
	active := activeRun{state: runstate.State{CheckTimeAllowance: &runstate.CheckTimeAllowance{LimitSeconds: 36000, ReservedSeconds: 35400}}}
	active.limitCheckStage(stage)
	if stage.BoundSeconds != 600 || !stage.AllowanceLimited || stage.LoadScaledSeconds != 5400 {
		t.Fatalf("remaining allowance did not cap the stage: %#v", stage)
	}
	if err := stage.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stageBoundLoad(*stage), "load-scaled 1h30m0s") || !strings.Contains(stage.BoundSays(), "capped by the cumulative") {
		t.Fatal("the load-scaled limit was lost behind the remaining allowance")
	}
}

// Rebuild the pipeline between every timeout, alternate check and stage limits,
// and use both automatic and decided continuations. None buys a fresh allowance.
func TestCheckAndStageTimeoutsShareTheirAllowanceAcrossRestarts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		load float64
	}{{"time allowance", 160}, {"continuation count", 48}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reserved := int64(tc.load / 16 * 1800)
			ctx := context.Background()
			repository, worktreeRoot, store := restartableFixture(t)
			tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
			provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
				return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
			}, approveVerdict)
			clock := &steppingClock{now: time.Now().UTC().Add(time.Hour)}
			docket := &memoryDocket{}
			build := func(check time.Duration) Pipeline {
				p := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"make race"}), provider)
				p.Config.Execution.CheckStageTimeout = config.Duration(30 * time.Minute)
				p.Checks = checks.Runner{Process: &timedChecks{clock: clock, takes: map[string]time.Duration{"make race": 24 * time.Hour}}, Clock: clock, Timeout: check}
				p.Load = func() (float64, int, bool) { return tc.load, 16, true }
				p.Docket = docketerOverStore(docket, store, p.Config)
				return p
			}
			first := build(10 * time.Minute)
			outcome, err := first.Run(ctx, tracker.Item.ID)
			if err == nil {
				t.Fatal("hung check passed")
			}
			stopped, err := store.Load(outcome.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !stopped.CheckStage.StoppedAtCheckBound || stopped.CheckTimeAllowance.ReservedSeconds != reserved {
				t.Fatalf("first timeout = %#v", stopped.CheckStage)
			}
			initial := stopped
			// A reconstructed pipeline is the automatic continuation's starter.
			second := build(time.Hour)
			continuer := CheckStageContinuer{Docket: docket, Runs: store, Intake: newIntakeHoldStore(t), Items: tracker, Worktrees: second.Worktrees.(*gitworktree.Manager), Capacity: 2,
				Load:  func() (float64, int, bool) { return 8, 16, true },
				Start: func(ctx context.Context, item, run string) (Outcome, error) { return second.Continue(ctx, item, run) },
			}
			result, err := continuer.Continue(ctx, CheckStageContinueRequest{Run: stopped.RunID})
			if err == nil || !result.Continued {
				t.Fatalf("continuation = %#v, %v", result, err)
			}
			stopped, err = store.Load(stopped.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !stopped.CheckStage.StoppedAtBound || stopped.CheckContinuationCount() != 1 || stopped.CheckTimeAllowance.ReservedSeconds != 2*reserved {
				t.Fatalf("second timeout did not exhaust the shared allowance: %#v, %#v", stopped.CheckStage, stopped.CheckTimeAllowance)
			}
			if tc.load == 48 {
				if !stopped.HarnessContinuesCheckStage() {
					t.Fatal("second continuation was refused too early")
				}
				if _, err := store.Triage().GrantRepair(ctx, tracker.Item.ID, triageDecided(runstate.TriageDecisionRepair, stopped.RunID), 2, docketedNow, handbackCaps); err != nil {
					t.Fatal(err)
				}
				third := build(10 * time.Minute)
				decided := repairContinuerOver(t, third, store, docket, tracker)
				decided.Remains = third.Worktrees.(*gitworktree.Manager)
				result, err := decided.Continue(ctx, RepairContinueRequest{Run: stopped.RunID})
				if err == nil || !result.Continued {
					t.Fatalf("decided continuation = %#v, %v", result, err)
				}
				stopped, err = store.Load(stopped.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if !stopped.CheckStage.StoppedAtCheckBound || stopped.CheckContinuationCount() != 2 || stopped.CheckAllowanceExhausted() || stopped.CheckTimeAllowance.ReservedSeconds != 3*reserved {
					t.Fatal("decided continuation lost the shared allowance")
				}
			}
			if stopped.HarnessContinuesCheckStage() {
				t.Fatal("exhausted continuation is still automatic")
			}
			if stopped.WorktreePath != initial.WorktreePath || stopped.Branch != initial.Branch || stopped.ProviderSessionID != initial.ProviderSessionID || stopped.WorktreeRemoved || stopped.BranchRemoved || stopped.ChecksPassed != nil || stopped.RepairAttempts != 0 {
				t.Fatal("timeout lost preserved work or earned gate evidence")
			}
			if len(provider.RequestsForRole(domain.RoleDeveloper)) != 1 {
				t.Fatal("continuation invoked the developer")
			}
			if !strings.Contains(stopped.CheckStageStopSays(), "cause remains unresolved") || !strings.Contains(stopped.CheckStageStopSays(), "allowance is exhausted") {
				t.Fatal(stopped.CheckStageStopSays())
			}
			if stopped.CheckStageContinuations[0].Stage.Content != initial.CheckStage.Content || stopped.CheckStageContinuations[0].ReservedSeconds != reserved {
				t.Fatal("continuation lost the prior check record")
			}
			if due, err := continuer.Due(stopped.RunID); err != nil || due {
				t.Fatalf("exhausted continuation due = %v, %v", due, err)
			}
			// A development-manager grant is not another path around the same limit.
			found := triage.Found{WorktreeThere: true, BranchThere: true}
			if err := continuableRepair(stopped, found); err == nil {
				t.Fatal("decided continuation reset the allowance")
			}
		})
	}
}
