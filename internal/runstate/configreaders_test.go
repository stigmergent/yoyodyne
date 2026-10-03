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
