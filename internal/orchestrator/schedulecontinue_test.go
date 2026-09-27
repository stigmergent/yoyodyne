package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// harnessContinuations is what a pull continues a paused run through, over the
// real harness: its tracker for the item, its store for a stop, and the notes
// each continuation writes, kept so the test can read them.
type harnessContinuations struct {
	harness *realScheduleHarness
	mu      sync.Mutex
	notes   []string
}

func (c *harnessContinuations) Show(ctx context.Context, id string) (beads.WorkItem, error) {
	return c.harness.Show(ctx, id)
}

func (c *harnessContinuations) StopRequested(runID string) (runstate.StopRequest, bool, error) {
	return c.harness.store.StopRequested(runID)
}

func (c *harnessContinuations) RecordOutcome(ctx context.Context, id, notes string) (beads.WorkItem, error) {
	c.mu.Lock()
	c.notes = append(c.notes, notes)
	c.mu.Unlock()
	return c.harness.RecordOutcome(ctx, id, notes)
}

func (c *harnessContinuations) recorded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.notes...)
}

// pausedOnADependency runs one item through a pull whose developer links it
// behind unfinished work, so the run pauses at the gate with its change in its
// worktree, its item claimed, and no process behind it. It returns the harness,
// the continuations the pulls read, the scheduler, and the paused run.
func pausedOnADependency(t *testing.T) (*realScheduleHarness, *harnessContinuations, Scheduler, runstate.State) {
	t.Helper()
	harness := newRealScheduleHarness(t, 1, "yoyodyne-task")
	developed := 0
	develop := harness.develop
	harness.develop = func(workItemID, worktree string) error {
		harness.mu.Lock()
		developed++
		first := developed == 1
		harness.mu.Unlock()
		if first {
			if err := harness.AddBlocker(context.Background(), workItemID, "yoyodyne-blocker"); err != nil {
				return err
			}
		}
		return develop(workItemID, worktree)
	}
	continuations := &harnessContinuations{harness: harness}
	scheduler := Scheduler{Open: func(ctx context.Context) (Pull, error) {
		pull, err := harness.open(ctx)
		pull.Continuations = continuations
		return pull, err
	}}

	schedule, err := scheduler.Schedule(context.Background())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(schedule.Started) != 1 || !schedule.Started[0].Outcome.Paused || schedule.Started[0].Outcome.PausedByDependency == nil {
		t.Fatalf("first pass = %s, want the one item started and paused on its dependency", schedule.Render())
	}
	paused, err := harness.store.Load(schedule.Started[0].Outcome.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !pausedForDependency(paused) {
		t.Fatalf("run after the first pass = %#v, want it paused on the dependency", paused)
	}
	// The pull after the run paused found it still waiting, and said on what.
	if !deferredSaying(schedule, "yoyodyne-task", "waits on unfinished work: yoyodyne-blocker") {
		t.Fatalf("first pass = %#v, want the paused run passed over naming what it waits on", schedule.Deferred)
	}
	return harness, continuations, scheduler, paused
}

func deferredSaying(schedule Schedule, workItemID, says string) bool {
	for _, deferred := range schedule.Deferred {
		if deferred.WorkItemID == workItemID && strings.Contains(deferred.Reason, says) {
			return true
		}
	}
	return false
}

func closeBlocker(harness *realScheduleHarness, workItemID string) {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	for index := range harness.items {
		if harness.items[index].ID != workItemID {
			continue
		}
		for dependency := range harness.items[index].Dependencies {
			harness.items[index].Dependencies[dependency].Status = "closed"
		}
	}
}

// A run paused on work its item waits on is continued by the next pull once
// that work has closed, in its own worktree and session, with nobody typing
// `yoyo run`. Its item stays claimed while it waits, so no queue ever offers
// it again, and before yoyodyne-ifd.428.51 that is where it stayed.
func TestAPullContinuesARunPausedOnADependencyOnceItCloses(t *testing.T) {
	t.Parallel()

	harness, continuations, scheduler, paused := pausedOnADependency(t)

	// While the work it waits on is open, a pull passes it over and leaves it.
	waiting, err := scheduler.Schedule(context.Background())
	if err != nil {
		t.Fatalf("Schedule() while waiting error = %v", err)
	}
	if len(waiting.Started) != 0 || !deferredSaying(waiting, "yoyodyne-task", "yoyodyne-blocker") {
		t.Fatalf("pass while waiting = %s (%#v), want nothing started and the paused run named with what it waits on", waiting.Render(), waiting.Deferred)
	}

	closeBlocker(harness, "yoyodyne-task")
	schedule, err := scheduler.Schedule(context.Background())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(schedule.Started) != 1 {
		t.Fatalf("pass after the dependency closed = %s, want the paused run continued", schedule.Render())
	}
	continued := schedule.Started[0]
	if continued.Outcome.RunID != paused.RunID {
		t.Fatalf("continued run = %q, want the paused run %q rather than a new one", continued.Outcome.RunID, paused.RunID)
	}
	if continued.Failure != "" || continued.Declined != "" || continued.Outcome.Paused || continued.Outcome.Integration == nil {
		t.Fatalf("continued run = %#v, want it finished and integrated", continued)
	}
	if !strings.Contains(continued.Reason, "continuing it in its own worktree and developer session") {
		t.Fatalf("reason = %q, want the continuation said", continued.Reason)
	}
	item, err := harness.Show(context.Background(), "yoyodyne-task")
	if err != nil || item.Status != "closed" {
		t.Fatalf("item after the continued run = %#v, %v; want it closed", item, err)
	}
	notes := continuations.recorded()
	if len(notes) != 1 || !strings.Contains(notes[0], "Continued by the harness") || !strings.Contains(notes[0], paused.RunID) {
		t.Fatalf("notes recorded = %q, want the continuation recorded on the item naming the run", notes)
	}
	finished, err := harness.store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if finished.DependencyPause != nil || finished.Status != runstate.StatusSucceeded {
		t.Fatalf("run after the continuation = %#v, want it succeeded with its pause lifted", finished)
	}
}

// A stop somebody recorded on the paused run is honoured before any
// continuation: the pull does not pick the run up again even once the work it
// waited on has closed, and leaves it for the sweep to end.
func TestAPullDoesNotContinueAPausedRunSomebodyStopped(t *testing.T) {
	t.Parallel()

	harness, continuations, scheduler, paused := pausedOnADependency(t)
	if err := harness.store.RecordStop(runstate.StopRequest{
		SchemaVersion: runstate.StopSchemaVersion,
		ProductID:     "yoyodyne",
		RunID:         paused.RunID,
		WorkItemID:    paused.WorkItemID,
		RequestedAt:   time.Now().UTC(),
		RequestedBy:   "the development manager",
		Reason:        "another item does this work",
		Decision:      runstate.TriageDecisionStop,
	}); err != nil {
		t.Fatalf("RecordStop() error = %v", err)
	}
	closeBlocker(harness, "yoyodyne-task")

	schedule, err := scheduler.Schedule(context.Background())
	if err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if len(schedule.Started) != 0 {
		t.Fatalf("pass = %s, want the stopped run not continued", schedule.Render())
	}
	if !deferredSaying(schedule, "yoyodyne-task", "the development manager asked for it to stop") {
		t.Fatalf("deferred = %#v, want the stop named", schedule.Deferred)
	}
	if notes := continuations.recorded(); len(notes) != 0 {
		t.Fatalf("notes recorded = %q, want no continuation recorded", notes)
	}
	left, err := harness.store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !pausedForDependency(left) {
		t.Fatalf("run = %#v, want it left paused for the sweep to end", left)
	}
}
