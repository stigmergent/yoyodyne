package runstate

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/repowrite/writertest"
)

func TestCapacityProbeWritesAreConfinedToTheDeclaredStateRoot(t *testing.T) {
	t.Parallel()
	writertest.Run(t, writertest.Writer{
		Name: "capacity probe reservation", Directory: "products/yoyodyne", File: "capacity-probes.jsonl",
		Write: func(t *testing.T, root string) error {
			store, err := NewCapacityProbeStore(root, "yoyodyne")
			if err != nil {
				return err
			}
			_, _, err = store.Claim(context.Background(), "provider\x00one\x00*", time.Now().UTC(), time.Minute)
			return err
		},
	})
}

func TestCapacityProbeRefusesBothLockAndJournalSymlinks(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"", ".lock"} {
		t.Run("symlink at journal"+suffix, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			store, err := NewCapacityProbeStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(outside, "untouched")
			if err := os.WriteFile(target, []byte("outside must stay unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, store.Path()+suffix); err != nil {
				t.Fatal(err)
			}
			if _, claimed, err := store.Claim(context.Background(), "scope", time.Now().UTC(), time.Minute); err == nil || claimed {
				t.Fatalf("Claim = %t, %v; want the escaped target refused", claimed, err)
			}
			content, err := os.ReadFile(target)
			if err != nil || string(content) != "outside must stay unchanged" {
				t.Fatalf("outside file changed: %q, %v", content, err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 1 {
				t.Fatalf("outside entries = %v, %v", entries, err)
			}
		})
	}
}

// Replacing the directory after Claim has opened its lock reproduces the gap
// between a path check and a later write. The context is a deterministic signal:
// Claim asks for its Done channel while constructing the lock timeout.
func TestCapacityProbeDirectoryReplacementAfterLockOpenCannotEscape(t *testing.T) {
	t.Parallel()
	root, outside := t.TempDir(), t.TempDir()
	store, err := NewCapacityProbeStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(store.Path())
	ctx := &probeReplacementContext{Context: context.Background(), replace: func() {
		if _, err := os.Stat(store.Path() + ".lock"); err != nil {
			t.Fatalf("replacement ran before the lock was opened: %v", err)
		}
		if err := os.Rename(directory, directory+"-original"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, directory); err != nil {
			t.Fatal(err)
		}
	}}
	if _, claimed, err := store.Claim(ctx, "scope", time.Now().UTC(), time.Minute); err == nil || claimed {
		t.Fatalf("Claim = %t, %v; want replaced directory refused", claimed, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside entries = %v, %v; want no escaped writes", entries, err)
	}
}

type probeReplacementContext struct {
	context.Context
	once    sync.Once
	replace func()
}

func (c *probeReplacementContext) Done() <-chan struct{} {
	c.once.Do(c.replace)
	return c.Context.Done()
}

func TestCapacityProbeRecoversAnInterruptedAppendWithoutLosingOtherScopes(t *testing.T) {
	t.Parallel()
	store, err := NewCapacityProbeStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, _, err := store.Claim(context.Background(), "first", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(store.Path(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"schema_version":1,"scope":"unfinished`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.Claim(context.Background(), "second", now, time.Minute); err != nil || !claimed {
		t.Fatalf("recovery claim = %t, %v", claimed, err)
	}
	next, err := store.Next()
	if err != nil || len(next) != 2 || !next["first"].Equal(now.Add(time.Minute)) || !next["second"].Equal(now.Add(time.Minute)) {
		t.Fatalf("recovered reservations = %v, %v", next, err)
	}
}
