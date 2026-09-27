package orchestrator

// A stop asked of a run no process is left to stop. The run's own process reads
// a stop at its next provider-call boundary, and a later process re-entering the
// run reads it before anything is resumed; a run paused on a dependency has
// neither, because its process returned when it recorded the pause and `yoyo
// run` turns back at the dependency before it adopts the run. So the sweep,
// holding the lease no live process holds, carries the stop out itself.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The shape of run-3b94404c on 2026-09-27: a run paused on a dependency link,
// its process gone, and a stop the development manager decided written beside
// it. While a live process holds the run the sweep leaves the stop to it; once
// nothing does, the sweep ends the run cancelled with its change preserved,
// frees its developer slot, tells the item who stopped it and why, and dockets
// the stoppage as already decided. Before this the sweep reported the run
// resumable on every pass and the slot stayed taken.
func TestASweepHonorsAStopAskedOfAPausedRunNoProcessHolds(t *testing.T) {
	t.Parallel()

	const decidedBy = "the development manager in conversation chat-0123456789abcdef"
	const reason = "superseded: the draining watch does this work (superseded by yoyodyne-ifd.398)"
	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: docketedItem, Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		if request.Role != domain.RoleDeveloper {
			return nil
		}
		tracker.Item.Dependencies = blockedBy("yoyodyne-ifd.398")
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)

	paused, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !paused.Paused || paused.PausedByDependency == nil {
		t.Fatalf("outcome = %#v, want a run paused for what its item waits on", paused)
	}

	// The stop is decided and requested after the run's process has returned,
	// which is the whole of the case: there is no boundary left to read it at.
	if _, err := store.Triage().RecordDecision(context.Background(), docketedItem, runstate.TriageDecision{
		Decision:     runstate.TriageDecisionStop,
		RunID:        paused.RunID,
		Reason:       "superseded: the draining watch does this work",
		SupersededBy: "yoyodyne-ifd.398",
		DecidedBy:    "development manager",
		Conversation: "chat-0123456789abcdef",
		Turn:         7,
	}, baseTime); err != nil {
		t.Fatalf("RecordDecision() error = %v", err)
	}
	if err := store.RecordStop(runstate.StopRequest{
		SchemaVersion: runstate.StopSchemaVersion,
		ProductID:     "yoyodyne",
		RunID:         paused.RunID,
		WorkItemID:    docketedItem,
		RequestedAt:   baseTime,
		Reason:        reason,
		RequestedBy:   decidedBy,
		Decision:      runstate.TriageDecisionStop,
	}); err != nil {
		t.Fatalf("RecordStop() error = %v", err)
	}

	docket, err := runstate.NewDocketStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewDocketStore() error = %v", err)
	}
	reconciler := Reconciler{
		Tracker:   tracker,
		Worktrees: newObserver(t, repository, worktreeRoot),
		Store:     store,
		Docket:    docketerOverStore(docket, store, pipeline.Config),
	}

	// A process holding the run is the one entitled to honor the stop, so the
	// sweep leaves the run to it and changes nothing.
	_, held, err := store.AdoptRun(context.Background(), paused.RunID)
	if err != nil {
		t.Fatalf("AdoptRun() error = %v", err)
	}
	results, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionHeld {
		t.Fatalf("reconciliations = %#v, want the run a live process holds left to it", results)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if untouched, err := store.Load(paused.RunID); err != nil || untouched.Status.Terminal() || untouched.DependencyPause == nil {
		t.Fatalf("held run = %#v (err=%v), want it exactly as its process left it", untouched, err)
	}

	// Nothing holds it now, so nothing else will ever honor the stop.
	results, err = reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("reconciliations = %d, want the one stopped run", len(results))
	}
	result := results[0]
	if result.Action != ActionStopped || result.Status != runstate.StatusCancelled || result.Failure != "" || result.DocketProblem != "" {
		t.Fatalf("reconciliation = %#v, want the run stopped as asked, cancelled, and docketed", result)
	}
	if !strings.Contains(result.Detail, decidedBy+" stopped this run") {
		t.Fatalf("detail = %q, want it to say who stopped the run", result.Detail)
	}

	// The freed slot: nothing is left in flight to hold a developer slot.
	incomplete, err := store.Incomplete()
	if err != nil {
		t.Fatalf("Incomplete() error = %v", err)
	}
	if len(incomplete) != 0 {
		t.Fatalf("runs in flight = %d, want the stopped run's slot free", len(incomplete))
	}

	// The record: cancelled, no pause left promising a continuation, and who
	// stopped it and why in the words the run's own stop uses.
	stopped, err := store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if stopped.Status != runstate.StatusCancelled || stopped.CompletedAt == nil || stopped.DependencyPause != nil {
		t.Fatalf("stopped state = %#v, want a cancelled run carrying no pause", stopped)
	}
	if !strings.Contains(stopped.Failure, decidedBy+" stopped this run") || !strings.Contains(stopped.Failure, "yoyodyne-ifd.398") {
		t.Fatalf("run failure = %q, want who stopped it and the superseding item", stopped.Failure)
	}

	// The preserved change: nothing about it was judged or removed.
	if stopped.WorktreeRemoved || stopped.BranchRemoved {
		t.Fatalf("stopped state = %#v, want the artifacts preserved", stopped)
	}
	if _, err := os.Stat(filepath.Join(stopped.WorktreePath, "feature.txt")); err != nil {
		t.Fatalf("the stopped run's change did not survive in its worktree: %v", err)
	}
	if listed := gitOutput(t, repository, "branch", "--list", stopped.Branch); !strings.Contains(listed, stopped.Branch) {
		t.Fatalf("branch %s is gone, want it preserved", stopped.Branch)
	}

	// The item is told who stopped its run and why.
	if !strings.Contains(tracker.Notes, decidedBy+" stopped this run") || !strings.Contains(tracker.Notes, "superseded by yoyodyne-ifd.398") {
		t.Fatalf("the work item was not told who stopped it and why: %q", tracker.Notes)
	}

	// The docket: one entry for the stopped run, closed by her decision.
	entries, err := docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].RunID != paused.RunID {
		t.Fatalf("docket = %#v, want the stopped run docketed once", entries)
	}
	if closure := entries[0].Closed; closure == nil || closure.Decision != runstate.TriageDecisionStop || closure.DecidedBy != decidedBy {
		t.Fatalf("closure = %#v, want the entry settled by her stop", closure)
	}
	if stop := entries[0].StopRequested; stop == nil || !stop.Landed || stop.By != decidedBy {
		t.Fatalf("stop on the entry = %#v, want the stop that landed", stop)
	}

	// Settled once: the next sweep finds nothing left to decide about the run.
	again, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second sweep = %#v, want nothing outstanding", again)
	}
}
