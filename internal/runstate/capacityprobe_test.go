package runstate

import (
	"context"
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
