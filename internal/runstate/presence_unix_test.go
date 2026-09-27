//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package runstate

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A process holding a run's lease stamps itself beside it, so a surface that
// only reads can tell a run somebody is working on from one nobody is — and the
// run that sat in developer slot 1 for twenty hours on 2026-09-26 would have
// read as the second the whole time.
func TestPresenceFindsTheHolderAndNamesARunNothingHolds(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	state := testState(t, StatusPending)
	lease, err := store.Reserve(context.Background(), state, 1)
	if err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	// Long after the record last moved: only the stamp can say somebody holds it.
	later := state.UpdatedAt.Add(24 * time.Hour)
	held, err := store.Presence(state, 30*time.Minute, later)
	if err != nil {
		t.Fatalf("Presence() while held error = %v", err)
	}
	if !held.Found {
		t.Fatalf("Presence() while this process holds the lease = %+v, want found", held)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := os.Stat(store.root + "/" + state.RunID + ".holder"); !os.IsNotExist(err) {
		t.Fatalf("holder stamp after release: stat error = %v, want it gone", err)
	}

	// Let go of, and quiet for longer than the window: nothing is working on it.
	gone, err := store.Presence(state, 30*time.Minute, later)
	if err != nil {
		t.Fatalf("Presence() after release error = %v", err)
	}
	if gone.Found || !strings.Contains(gone.Says, "no process holds it") {
		t.Fatalf("Presence() after release = %+v, want no process found", gone)
	}
	// Let go of, but moved inside the window: a holder from a build that stamps
	// nothing is given the benefit of the doubt.
	recent, err := store.Presence(state, 30*time.Minute, state.UpdatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("Presence() inside the window error = %v", err)
	}
	if !recent.Found {
		t.Fatalf("Presence() inside the window = %+v, want found", recent)
	}
}

// A holder killed outright leaves its stamp behind, and the stamp is checked
// against the operating system rather than trusted, so the run reads as having
// no process at once rather than after the quiet window.
func TestPresenceReadsAStampWhoseProcessHasExitedAsNoProcess(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	state := testState(t, StatusRunning)
	if err := store.Create(state); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	exited := exec.Command("true")
	if err := exited.Run(); err != nil {
		t.Fatalf("run a process to exit: %v", err)
	}
	path, err := store.holderPath(state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	stamp := `{"pid":` + strconv.Itoa(exited.Process.Pid) + `,"held_at":"2026-08-14T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(stamp), 0o600); err != nil {
		t.Fatal(err)
	}
	presence, err := store.Presence(state, 30*time.Minute, state.UpdatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("Presence() error = %v", err)
	}
	if presence.Found || !strings.Contains(presence.Says, "has exited") {
		t.Fatalf("Presence() over a dead holder's stamp = %+v, want no process found", presence)
	}
}
