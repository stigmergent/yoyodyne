package runstate

import (
	"testing"
	"time"
)

// A pass note is a note about what the session is doing inside its poll, not a
// change of the session's state, so the log's fold reads past it: a session
// idle over an empty queue that begins a pass is still read as idle, from the
// moment it went idle.
func TestAPassNoteIsReadPastByTheSessionsLatestWord(t *testing.T) {
	t.Parallel()
	moment := time.Date(2026, 9, 30, 4, 52, 8, 0, time.UTC)
	store, err := NewWatchStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	idle := WatchTransition{
		SchemaVersion: WatchSchemaVersion, ProductID: "yoyodyne", SessionID: "watch-0123456789abcdef0123456789abcdef",
		State: WatchIdle, At: moment, Reason: "nothing is ready",
	}
	note := idle
	note.State, note.At = WatchWatching, moment.Add(1)
	note.RecurringPass = &WatchPass{Task: "architect-pass", Trigger: PassTriggerSchedule, At: note.At}
	note.Reason = note.RecurringPass.Says()
	for _, transition := range []WatchTransition{idle, note} {
		if err := store.Record(transition); err != nil {
			t.Fatalf("Record(%s) error = %v", transition.State, err)
		}
	}
	latest, found, err := store.Latest()
	if err != nil || !found || latest.State != WatchIdle {
		t.Fatalf("Latest() = %+v, %v, %v; want the idle line the note was written after", latest, found, err)
	}
}
