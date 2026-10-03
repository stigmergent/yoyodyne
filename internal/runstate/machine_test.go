package runstate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestATornMachineObservationDoesNotHideTheNextOne(t *testing.T) {
	t.Parallel()
	store, err := NewSupervisionStore(t.TempDir(), "example")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 2, 16, 15, 0, 0, time.UTC)
	first := MachineObservation{At: at, Watching: true, Power: []PowerEvent{{At: at.Add(-time.Hour), Source: "pmset: Sleep"}, {At: at, Awake: true, Source: "pmset: Wake"}}}
	if err := store.RecordMachine(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(store.Root(), "machine.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"at":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordMachine(context.Background(), MachineObservation{At: at.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	history, err := store.MachineHistory()
	if err == nil || len(history) != 2 || !history[1].At.Equal(at.Add(time.Minute)) || len(history[0].Power) != 2 {
		t.Fatalf("history lost the record after the torn write: %+v, %v", history, err)
	}
}
