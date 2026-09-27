//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// runHolderRootEnv and runHolderRunEnv tell TestRunLeaseHolderProcess which run
// to hold, and their absence tells it it is an ordinary test with nothing to do.
const (
	runHolderRootEnv = "YOYODYNE_TEST_RUN_HOLDER_ROOT"
	runHolderRunEnv  = "YOYODYNE_TEST_RUN_HOLDER_RUN"
)

// TestRunLeaseHolderProcess is not a test. It is the process the test below
// kills: it takes one run's lease the way a process working on the run does,
// says so, and waits to be killed. The wait carries its own bound, so a parent
// that dies before killing it does not leave it running.
func TestRunLeaseHolderProcess(t *testing.T) {
	root, runID := os.Getenv(runHolderRootEnv), os.Getenv(runHolderRunEnv)
	if root == "" || runID == "" {
		return
	}
	store, err := runstate.NewStore(root, "yoyodyne")
	if err != nil {
		fmt.Println("store:", err)
		os.Exit(2)
	}
	if _, _, err := store.AdoptRun(context.Background(), runID); err != nil {
		fmt.Println("adopt:", err)
		os.Exit(2)
	}
	fmt.Println("run lease held")
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

func awaitHeldRunLease(output io.Reader) error {
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "run lease held") {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.New("the holder exited without taking the run's lease")
}

// A paused run whose process is killed, and a stop written for it afterwards,
// is the shape run-3b94404c was found in on 2026-09-27: parked on a dependency,
// its process gone since the evening before, a stop the development manager
// decided standing unread because only a live process reads one — and developer
// slot 1 held for twenty hours. The sweep honours the stop in the dead process's
// place, at once and without waiting out the grace: the run ends cancelled with
// its change preserved, the item is told, the decided stoppage is docketed as
// settled, and the slot is free.
func TestTheSweepHonoursAStopOnAPausedRunWhoseProcessWasKilled(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		if request.Role != domain.RoleDeveloper {
			return nil
		}
		tracker.Item.Dependencies = blockedBy("yoyodyne-blocker")
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)
	paused, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil || !paused.Paused || paused.PausedByDependency == nil {
		t.Fatalf("Run() = %#v, %v; want a run paused on the work its item waits on", paused, err)
	}
	tracker.Item.Status = "in_progress"

	// A process takes the paused run up, as a `yoyo run` continuing it would.
	stateRoot := filepath.Dir(filepath.Dir(filepath.Dir(store.Root())))
	child := exec.Command(os.Args[0], "-test.run=^TestRunLeaseHolderProcess$")
	child.Env = append(os.Environ(), runHolderRootEnv+"="+stateRoot, runHolderRunEnv+"="+paused.RunID)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}
	if err := child.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	if err := awaitHeldRunLease(output); err != nil {
		t.Fatalf("the holder never took the run's lease: %v", err)
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

	// While it lives, the run is its holder's: the sweep leaves it, and a reading
	// finds the process behind it.
	held, err := reconciler.Reconcile(context.Background())
	if err != nil || len(held) != 1 || held[0].Action != ActionHeld {
		t.Fatalf("Reconcile() with a live holder = %#v, %v; want the run left to it", held, err)
	}
	recorded, err := store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if presence, err := store.Presence(recorded, readmodel.DefaultDeadClaimThreshold, time.Now()); err != nil || !presence.Found {
		t.Fatalf("Presence() with a live holder = %#v, %v; want the process found", presence, err)
	}

	// Killed outright: nothing it would have written on the way out is written.
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	_ = child.Wait()
	presence, err := store.Presence(recorded, readmodel.DefaultDeadClaimThreshold, time.Now())
	if err != nil || presence.Found || !strings.Contains(presence.Says, "has exited") {
		t.Fatalf("Presence() after the kill = %#v, %v; want no process found at once", presence, err)
	}

	// The development manager decides the run is superseded, and the harness
	// writes the stop on her behalf.
	request := runstate.StopRequest{
		SchemaVersion: runstate.StopSchemaVersion,
		ProductID:     "yoyodyne",
		RunID:         paused.RunID,
		WorkItemID:    tracker.Item.ID,
		RequestedAt:   time.Now().UTC(),
		Reason:        "superseded by yoyodyne-other",
		RequestedBy:   "the development manager in conversation chat-test",
		Decision:      runstate.TriageDecisionStop,
	}
	if err := store.RecordStop(request); err != nil {
		t.Fatalf("RecordStop() error = %v", err)
	}

	// No grace is waited out: the clock is the real one, seconds after the park.
	results, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionCancelled || results[0].Failure != "" || results[0].DocketProblem != "" {
		t.Fatalf("reconciliation = %#v, want the stop honoured as a cancellation", results)
	}
	for _, want := range []string{"the development manager in conversation chat-test stopped this run", "superseded by yoyodyne-other", "the harness's sweep ended it in that process's place"} {
		if !strings.Contains(results[0].Detail, want) {
			t.Fatalf("detail %q does not say %q", results[0].Detail, want)
		}
	}
	settled, err := store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settled.Status != runstate.StatusCancelled || settled.CompletedAt == nil || settled.DependencyPause != nil {
		t.Fatalf("settled run = %#v, want it cancelled with its park cleared", settled)
	}
	if settled.SettledQuietSince == nil || !settled.SettledQuietSince.Equal(recorded.UpdatedAt) {
		t.Fatalf("settled run quiet since %v, want %s, when its record last moved", settled.SettledQuietSince, recorded.UpdatedAt)
	}
	// The change is where the developer left it.
	if settled.WorktreeRemoved || settled.BranchRemoved {
		t.Fatalf("settled run = %#v, want its branch and worktree kept", settled)
	}
	if _, err := os.Stat(filepath.Join(settled.WorktreePath, "feature.txt")); err != nil {
		t.Fatalf("the paused run's work is not where it was left: %v", err)
	}
	if !strings.Contains(tracker.Notes, "the harness's sweep ended it in that process's place") {
		t.Fatalf("item notes = %q, want the stop recorded on the item", tracker.Notes)
	}
	// The slot is free: the slot and the in-flight guard read this listing.
	incomplete, err := store.Incomplete()
	if err != nil || len(incomplete) != 0 {
		t.Fatalf("Incomplete() = %#v, %v; want the stopped run holding no slot", incomplete, err)
	}
	// Her decision closes the stoppage it made, as it would have had the run
	// stopped itself.
	entries, err := docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].RunID != paused.RunID || entries[0].Class != triage.ClassStoppedRun ||
		entries[0].StopRequested == nil || entries[0].Closed == nil {
		t.Fatalf("docket = %#v, want the decided stop docketed and closed by her decision", entries)
	}

	// Settled once: a second sweep has nothing to do.
	again, err := reconciler.Reconcile(context.Background())
	if err != nil || len(again) != 0 {
		t.Fatalf("second Reconcile() = %#v, %v; want nothing outstanding", again, err)
	}
}

// A paused run nothing continues is settled once its record has sat still for
// the grace, whatever it was paused on — not only a provider the harness stopped.
// run-3b94404c was parked on a dependency and was reported resumable by every
// sweep for twenty hours, because the settlement read provider stops alone.
func TestTheSweepSettlesADependencyPausedRunNothingContinued(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		if request.Role != domain.RoleDeveloper {
			return nil
		}
		tracker.Item.Dependencies = blockedBy("yoyodyne-blocker")
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)
	paused, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil || !paused.Paused || paused.PausedByDependency == nil {
		t.Fatalf("Run() = %#v, %v; want a run paused on the work its item waits on", paused, err)
	}
	tracker.Item.Status = "in_progress"
	recorded, err := store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	docket, err := runstate.NewDocketStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewDocketStore() error = %v", err)
	}
	sweepAt := func(at time.Time) Reconciler {
		return Reconciler{
			Tracker:   tracker,
			Worktrees: newObserver(t, repository, worktreeRoot),
			Store:     store,
			Docket:    docketerOverStore(docket, store, pipeline.Config),
			Clock:     &pausingClock{now: at},
		}
	}

	// Inside the grace it is the wait it is, and the reading says when that ends.
	inside, err := sweepAt(recorded.UpdatedAt.Add(DefaultVanishedGrace - time.Minute)).Reconcile(context.Background())
	if err != nil || len(inside) != 1 || inside[0].Action != ActionResumable || !strings.Contains(inside[0].Detail, "settles it as a stopped run") {
		t.Fatalf("Reconcile() inside the grace = %#v, %v; want it resumable with the grace said", inside, err)
	}

	results, err := sweepAt(recorded.UpdatedAt.Add(DefaultVanishedGrace)).Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(results) != 1 || results[0].Action != ActionBlocked || results[0].Failure != "" {
		t.Fatalf("reconciliation = %#v, want the abandoned park settled as blocked", results)
	}
	for _, want := range []string{"no live process behind it", "waits on unfinished work: yoyodyne-blocker", "the harness settled it as an environmental stop"} {
		if !strings.Contains(results[0].Detail, want) {
			t.Fatalf("detail %q does not say %q", results[0].Detail, want)
		}
	}
	settled, err := store.Load(paused.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !settled.Status.Terminal() || settled.DependencyPause != nil || settled.Blocker == "" ||
		settled.Environmental == nil || settled.Environmental.Cause != runstate.CauseProcessVanished {
		t.Fatalf("settled run = %#v, want it terminal, blocked, and recorded as a vanished process", settled)
	}
	if _, err := os.Stat(filepath.Join(settled.WorktreePath, "feature.txt")); err != nil {
		t.Fatalf("the paused run's work is not where it was left: %v", err)
	}
	incomplete, err := store.Incomplete()
	if err != nil || len(incomplete) != 0 {
		t.Fatalf("Incomplete() = %#v, %v; want the settled run holding no slot", incomplete, err)
	}
	entries, err := docket.List()
	if err != nil || len(entries) != 1 || entries[0].RunID != paused.RunID || entries[0].Closed != nil {
		t.Fatalf("docket = %#v, %v; want the stoppage docketed for the development manager", entries, err)
	}
}
