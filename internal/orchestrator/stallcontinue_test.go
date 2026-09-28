package orchestrator

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The two stalls of 2026-09-27 and 09-28, with the half hour turned into a
// clock: a first attempt whose provider stream went silent is stopped, left in
// flight, and settled by the sweep, and the docket entry names the harness as
// the one to move. The next pull continues it with nobody deciding anything —
// not while the operator's pause or the intake hold stands, and then in the
// same session and worktree once they are lifted — charging no attempt, round,
// grant, or re-run. The continued attempt stalls again, and the second stall is
// settled and docketed as the development manager's, saying the harness's
// continuation is spent; the harness offers nothing more.
func TestAFirstStallIsContinuedByTheHarnessAndASecondIsDocketed(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	// Both developer attempts go silent and are stopped on time.
	stalling := providerStopBackend(2, execution.ProcessStalled, approveVerdict)
	commands := []string{"exit 0"}
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, stalling, commands), stalling)
	docket := &memoryDocket{}
	docketer := docketerOverStore(docket, store, pipeline.Config)

	paused, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil || !paused.Paused || paused.ProviderStop != runstate.ProviderStopStalled {
		t.Fatalf("Run() error = %v, outcome = %#v, want the first attempt stopped for a silent stream", err, paused)
	}
	tracker.Item.Status = "in_progress"
	settle := func() runstate.State {
		t.Helper()
		stopped, err := store.Load(paused.RunID)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		reconciler := Reconciler{
			Tracker:   tracker,
			Worktrees: newObserver(t, repository, worktreeRoot),
			Store:     store,
			Docket:    docketer,
			Clock:     &pausingClock{now: stopped.UpdatedAt.Add(DefaultVanishedGrace)},
		}
		results, err := reconciler.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		if len(results) != 1 || results[0].Action != ActionBlocked {
			t.Fatalf("reconciliation = %#v, want the stalled run settled after the grace", results)
		}
		settled, err := store.Load(paused.RunID)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		return settled
	}

	// The first stall, settled: the harness continues it, and the entry says so.
	first := settle()
	if !first.SettledSilentStreamStall() || !first.HarnessContinuesStall() {
		t.Fatalf("settled run = %#v, want a first silent-stream stall the harness continues", first)
	}
	if len(docket.entries) != 1 || !docket.entries[0].HarnessContinuesStall {
		t.Fatalf("docket = %#v, want the stall docketed as one the harness continues", docket.entries)
	}
	for _, want := range []string{"produced no output for longer than the harness allows", "the harness continues it itself", "Next mover: the harness", "continuation 1 of 1"} {
		if rendered := docket.entries[0].Render(); !strings.Contains(rendered, want) {
			t.Fatalf("docket entry does not say %q:\n%s", want, rendered)
		}
	}
	countersBefore, err := store.Triage().Counters(tracker.Item.ID)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}

	worktrees, err := gitworktree.New(gitworktree.Options{Runner: execution.OSProcessRunner{}, RepositoryRoot: repository, WorktreeRoot: worktreeRoot, Timeout: testGitBudget})
	if err != nil {
		t.Fatalf("gitworktree.New() error = %v", err)
	}
	intake := newIntakeHoldStore(t)
	continuer := StallContinuer{
		Docket: docket, Redocket: docketer, Runs: store, Intake: intake, Items: tracker, Worktrees: worktrees,
		Capacity: pipeline.Config.Execution.MaxConcurrentDevelopers,
		Start: func(ctx context.Context, workItemID, runID string) (Outcome, error) {
			return pipeline.Continue(ctx, workItemID, runID)
		},
	}
	holds := &pausedHolds{held: true, hold: runstate.OperatorHold{HeldAt: time.Now()}}
	carrying := func() CarryOut {
		return CarryOut{Docket: docket, Decisions: store.Triage(), Reruns: store.Reruns(), Runs: store, Stalls: continuer, Holds: holds}
	}
	tasks, err := carrying().Outstanding()
	if err != nil || len(tasks) != 1 || tasks[0].Decision != DecisionContinueStall || tasks[0].RunID != paused.RunID {
		t.Fatalf("Outstanding() = %#v, %v; want the harness's continuation of the stall", tasks, err)
	}
	unchanged := func(gate string) {
		t.Helper()
		if again, _ := store.Load(paused.RunID); again.Status != first.Status || len(again.RepairContinuations) != 0 {
			t.Fatalf("a continuation stopped by %s changed the run: %#v", gate, again)
		}
	}
	// The operator's pause stops it, as it stops a recorded decision's carry-out.
	if carried, _, err := carrying().Carry(context.Background(), tasks[0]); err != nil || carried.Carried || !carried.Waiting || carried.Gate != runstate.TriageGateSpendingPause {
		t.Fatalf("Carry() under the pause = %#v, %v; want it waiting on the pause", carried, err)
	}
	unchanged("the pause")
	// And so does the intake hold.
	holds.held = false
	if _, err := intake.Hold(runstate.IntakeHolderOperator, "the queue is heading somewhere odd", time.Now()); err != nil {
		t.Fatalf("Hold() error = %v", err)
	}
	if carried, _, err := carrying().Carry(context.Background(), tasks[0]); err != nil || carried.Carried || !carried.Waiting || carried.Gate != runstate.TriageGateIntakeHold {
		t.Fatalf("Carry() under the intake hold = %#v, %v; want it waiting on the hold", carried, err)
	}
	unchanged("the intake hold")
	if _, _, err := intake.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	// The next pull continues it, with no decision recorded, in its own session
	// and worktree — and the continued attempt stalls again.
	carried, continued, err := carrying().Carry(context.Background(), tasks[0])
	if err != nil {
		t.Fatalf("Carry() error = %v", err)
	}
	if !carried.Carried || !continued.Paused || continued.ProviderStop != runstate.ProviderStopStalled || continued.RunID != paused.RunID {
		t.Fatalf("carried = %#v, outcome = %#v; want the same run continued and stopped for a silent stream again", carried, continued)
	}
	developer := stalling.RequestsForRole(domain.RoleDeveloper)
	if len(developer) != 2 || developer[1].SessionID != first.ProviderSessionID || developer[1].WorkingDirectory != first.WorktreePath {
		t.Fatalf("developer attempts = %#v, want the second in the stalled run's own session and worktree", developer)
	}
	if !strings.Contains(tracker.Notes, "Continued after a stall") {
		t.Fatalf("item notes do not record the continuation:\n%s", tracker.Notes)
	}
	if closure, closed := docket.closed[docket.entries[0].Key]; !closed || closure.Decision != continuedStallDocketDecision {
		t.Fatalf("docket closure = %#v, %t; want the entry closed as continued by the harness", closure, closed)
	}
	countersAfter, err := store.Triage().Counters(tracker.Item.ID)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	countersAfter.UpdatedAt = countersBefore.UpdatedAt
	if !reflect.DeepEqual(countersAfter, countersBefore) {
		t.Fatalf("the item's triage record moved across the harness's continuation:\nbefore %#v\nafter  %#v", countersBefore, countersAfter)
	}
	if claimed, _ := store.Reruns().Claimed(tracker.Item.ID); len(claimed) != 0 {
		t.Fatalf("re-runs claimed = %#v, want none", claimed)
	}

	// The second stall, settled: docketed for the development manager, saying the
	// harness's continuation is spent, and the harness offers nothing more.
	second := settle()
	if !second.SettledSilentStreamStall() || second.HarnessContinuesStall() || second.HarnessStallContinuations() != 1 || second.RepairAttempts != 0 {
		t.Fatalf("second settled run = %#v, want a stall whose one harness continuation is spent, with no attempt charged", second)
	}
	latest := docket.entries[len(docket.entries)-1]
	if latest.HarnessContinuesStall {
		t.Fatalf("second entry = %#v, want it the development manager's", latest)
	}
	rendered := latest.Render()
	for _, want := range []string{"its continuation is spent", "development manager's decision", "Next mover: you", "yoyo triage repair " + paused.RunID} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("second entry does not say %q:\n%s", want, rendered)
		}
	}
	if tasks, err := carrying().Outstanding(); err != nil || len(tasks) != 0 {
		t.Fatalf("Outstanding() after the second stall = %#v, %v; want nothing the harness continues", tasks, err)
	}
}

// A stall in a run a session re-adopted after a redeploy stop says so on the
// entry, because the stall began in the session that re-adoption resumed.
func TestAStallAfterARedeployReadoptionSaysSo(t *testing.T) {
	t.Parallel()

	stoppedAt := time.Date(2026, 9, 28, 18, 40, 0, 0, time.UTC)
	state := runstate.State{
		RunID: "run-" + strings.Repeat("b", 32), WorkItemID: "yoyodyne-task", Status: runstate.StatusFailed, Phase: runstate.PhaseDeveloping,
		WorktreePath: "/tmp/w", Branch: "b", BaseCommit: "c", TargetBranch: "main", ProviderSessionID: "session",
		Environmental: &runstate.EnvironmentalRefusal{Cause: runstate.CauseProcessVanished, ProviderStop: runstate.ProviderStopStalled, RecordedAt: stoppedAt, Settled: true},
		Readopted:     &runstate.RedeployStop{At: stoppedAt, Phase: runstate.PhaseDeveloping, BoundSeconds: 900},
	}
	if !state.HarnessContinuesStall() {
		t.Fatal("a first settled stall is not continued by the harness")
	}
	for _, want := range []string{"stopped for a redeploy at its developing phase at 2026-09-28T18:40:00Z", "re-adopted", "this stall began in the session that re-adoption resumed"} {
		if says := state.StallStopSays(); !strings.Contains(says, want) {
			t.Fatalf("StallStopSays() = %q, want it to say %q", says, want)
		}
	}
	// A stall inside the repair loop is not the harness's: a failure was
	// returned there, and what it is owed is a repair.
	state.RepairAttempts = 1
	if state.SettledSilentStreamStall() || state.HarnessContinuesStall() {
		t.Fatal("a stall inside the repair loop is read as one the harness continues")
	}
}
