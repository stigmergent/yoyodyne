package runstate

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCapacityProbeReservationSurvivesRestartAndSerializesTheScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Now().UTC()
	interval := 30 * time.Minute
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store, err := NewCapacityProbeStore(root, "yoyodyne")
			if err != nil {
				t.Error(err)
				return
			}
			next, took, err := store.Claim(context.Background(), "provider\x00one\x00*", now, interval)
			if err != nil {
				t.Error(err)
				return
			}
			if !next.Equal(now.Add(interval)) {
				t.Errorf("next = %s", next)
			}
			if took {
				claimed.Add(1)
			}
		}()
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("claimed = %d, want one invocation", claimed.Load())
	}
	restarted, err := NewCapacityProbeStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	if _, took, err := restarted.Claim(context.Background(), "provider\x00one\x00*", now.Add(time.Minute), interval); err != nil || took {
		t.Fatalf("restart claim = %t, %v", took, err)
	}
	if _, took, err := restarted.Claim(context.Background(), "provider\x00two\x00*", now.Add(time.Minute), interval); err != nil || !took {
		t.Fatalf("unaffected account claim = %t, %v", took, err)
	}
	if _, took, err := restarted.Claim(context.Background(), "provider\x00one\x00*", now.Add(interval), interval); err != nil || !took {
		t.Fatalf("due claim = %t, %v", took, err)
	}
}

func TestUnreadableProbePacingIsNeitherSpentThroughNorOverwritten(t *testing.T) {
	t.Parallel()
	store, err := NewCapacityProbeStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, _, err := store.Claim(context.Background(), "provider\x00one\x00*", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	record["future_field"] = json.RawMessage(`true`)
	encoded, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(store.Path(), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Next(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Next = %v; want unknown pacing fields refused", err)
	}
	if _, took, err := store.Claim(context.Background(), "provider\x00one\x00*", now.Add(time.Minute), time.Minute); err == nil || took {
		t.Fatalf("Claim = %t, %v; want no probe on unreadable pacing", took, err)
	}
	preserved, err := os.ReadFile(store.Path())
	if err != nil || string(preserved) != string(encoded) {
		t.Fatalf("probe pacing was overwritten: %s, %v", preserved, err)
	}
}
