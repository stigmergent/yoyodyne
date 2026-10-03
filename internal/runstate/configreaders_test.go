package runstate

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/repowrite/writertest"
)

func TestConfigReaderMismatchesNameTheRunningPartsThatCannotReadTheFile(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("agents:\n  developer:\n    role: developer\n    effort: medium\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewConfigReaderStore(root, "example")
	if err != nil {
		t.Fatal(err)
	}
	// The dashboard and the Slack sink both run a build from before the key;
	// the sink's process has gone, so it says nothing. The scheduler runs this
	// build and reads everything.
	alive := map[int]bool{101: true, 102: false, 103: true}
	store = store.WithProcessCheck(func(pid int) (bool, error) { return alive[pid], nil })
	older := slices.DeleteFunc(config.SchemaKeys(), func(key string) bool { return key == "agents.*.effort" })
	started := time.Date(2026, 9, 26, 23, 36, 0, 0, time.UTC)
	for _, reader := range []ConfigReader{
		{Service: "dashboard", PID: 101, Build: "0123456789abcdef", ConfigPath: configPath, StartedAt: started, Keys: older},
		{Service: "slack", PID: 102, Build: "0123456789abcdef", ConfigPath: configPath, StartedAt: started, Keys: older},
		{Service: "scheduler", PID: 103, Build: "fedcba9876543210", ConfigPath: configPath, StartedAt: started, Keys: config.SchemaKeys()},
	} {
		if err := store.Record(reader); err != nil {
			t.Fatalf("Record(%s) error = %v", reader.Service, err)
		}
	}

	mismatches, err := store.Mismatches()
	if err != nil {
		t.Fatalf("Mismatches() error = %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("Mismatches() = %+v, want the dashboard alone", mismatches)
	}
	got := mismatches[0]
	if got.Service != "dashboard" || got.Build != "0123456789abcdef" || got.PID != 101 || !reflect.DeepEqual(got.Keys, []string{"agents.developer.effort"}) {
		t.Fatalf("Mismatches()[0] = %+v", got)
	}
	says := got.Says()
	for _, want := range []string{"the dashboard service", "build 0123456789ab", "pid 101", "agents.developer.effort", configPath} {
		if !strings.Contains(says, want) {
			t.Errorf("Says() = %q, want it to name %q", says, want)
		}
	}

	// A template-only key still names the old build, even when the active file
	// has no such key. Current builds and exited parts remain absent.
	if err := os.WriteFile(configPath, []byte("agents: {developer: {role: developer}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if active, err := store.Mismatches(); err != nil || len(active) != 0 {
		t.Fatalf("healthy active file = %+v, %v", active, err)
	}
	prospective, err := store.TemplateMismatches("internal/config/builtin/v1/bundle.yaml", []string{"agents.*.effort"})
	if err != nil || len(prospective) != 1 || prospective[0].Service != "dashboard" {
		t.Fatalf("TemplateMismatches() = %+v, %v, want the dashboard alone", prospective, err)
	}
	for _, want := range []string{"build 0123456789ab", "agents.*.effort", "adopting those keys", "would make"} {
		if !strings.Contains(prospective[0].Says(), want) {
			t.Errorf("prospective finding %q lacks %q", prospective[0].Says(), want)
		}
	}
	if strings.Contains(prospective[0].Says(), "every read it makes of the configuration fails") {
		t.Fatal("prospective finding claims the healthy active file is already broken")
	}
}

func TestConfigReaderRefusesAPartTheProductDoesNotHave(t *testing.T) {
	store, err := NewConfigReaderStore(t.TempDir(), "example")
	if err != nil {
		t.Fatal(err)
	}
	err = store.Record(ConfigReader{Service: "printer", PID: 1, ConfigPath: "/x/config.yaml", StartedAt: time.Now(), Keys: []string{"version"}})
	if err == nil || !strings.Contains(err.Error(), "printer") {
		t.Fatalf("Record() error = %v, want a refusal naming the part", err)
	}
}

func TestConfigReaderWritesStayInsideTheStateRoot(t *testing.T) {
	writertest.Run(t, writertest.Writer{
		Name: "configuration reader", Directory: "products/example/config-readers", File: "dashboard.json",
		Write: func(t *testing.T, root string) error {
			store, err := NewConfigReaderStore(root, "example")
			if err != nil {
				return err
			}
			return store.Record(configReaderRecordFixture())
		},
	})
}

func TestConfigReaderRefusesAReplacedWriteRoot(t *testing.T) {
	for _, replacement := range []string{"symlink", "directory"} {
		t.Run(replacement, func(t *testing.T) {
			base := t.TempDir()
			statePath := filepath.Join(base, "state")
			outside := filepath.Join(base, "outside")
			for _, path := range []string{statePath, outside} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			store, err := NewConfigReaderStore(statePath, "example")
			if err != nil {
				t.Fatal(err)
			}
			pinned, err := store.pinWriteRoot()
			if err != nil {
				t.Fatal(err)
			}
			defer pinned.Close()
			moved := filepath.Join(base, "moved")
			if err := os.Rename(statePath, moved); err != nil {
				t.Fatal(err)
			}
			if replacement == "symlink" {
				if err := os.Symlink(outside, statePath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(statePath, 0o700); err != nil {
				t.Fatal(err)
			}
			// This is the second half of Record, after its directory was pinned
			// and before writing. A check-to-use replacement must be refused.
			if err := store.recordIn(pinned, configReaderRecordFixture()); err == nil {
				t.Fatal("a replaced state root was accepted")
			}
			for _, path := range []string{outside, moved, statePath} {
				if entries, err := os.ReadDir(path); err != nil || len(entries) != 0 {
					t.Fatalf("records appeared after a refused replacement in %s: %v, %v", path, entries, err)
				}
			}
			if replacement == "symlink" {
				if err := store.Record(configReaderRecordFixture()); err == nil {
					t.Fatal("Record followed a replacement symlink at its declared root")
				}
				if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
					t.Fatalf("Record escaped into %s: %v, %v", outside, entries, err)
				}
			}
		})
	}
}

func TestConfigReaderCreatesAMissingStateRootThroughTheConfinedWriter(t *testing.T) {
	store, err := NewConfigReaderStore(filepath.Join(t.TempDir(), "new", "state"), "example")
	if err != nil {
		t.Fatal(err)
	}
	store = store.WithProcessCheck(func(int) (bool, error) { return true, nil })
	if err := store.Record(configReaderRecordFixture()); err != nil {
		t.Fatal(err)
	}
	if readers, err := store.Running(); err != nil || len(readers) != 1 {
		t.Fatalf("newly created record = %+v, %v", readers, err)
	}
	info, err := os.Stat(filepath.Join(store.root, "dashboard.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("record permissions: %v, %v", info, err)
	}
}

func configReaderRecordFixture() ConfigReader {
	return ConfigReader{
		SchemaVersion: ConfigReaderSchemaVersion, ProductID: "example",
		Service: "dashboard", PID: 4242, ConfigPath: "/example/config.yaml",
		StartedAt: time.Now(), Keys: []string{"version"},
	}
}
