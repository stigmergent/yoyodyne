package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A landing that leaves the configuration carrying a key a running part's
// build cannot read names that part — its build, its process, and the key — on
// the item and in the outcome, the way the effort lines should have been named
// against the dashboard on 2026-09-28. A part that reads the key, and a part
// whose process has gone, are not named.
func TestALandingNamesTheRunningPartsThatCannotReadTheConfiguration(t *testing.T) {
	t.Parallel()

	repository := pipelineRepository(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline, _ := newAutomaticPipeline(t, repository, tracker, provider, []string{"true"})

	stateRoot := t.TempDir()
	configPath := filepath.Join(stateRoot, "config.yaml")
	if err := os.WriteFile(configPath, []byte("agents:\n  developer:\n    role: developer\n    effort: medium\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := runstate.NewConfigReaderStore(stateRoot, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	alive := map[int]bool{4242: true, 4343: true, 4444: false}
	store = store.WithProcessCheck(func(pid int) (bool, error) { return alive[pid], nil })
	older := slices.DeleteFunc(config.SchemaKeys(), func(key string) bool { return key == "agents.*.effort" })
	started := time.Date(2026, 9, 26, 23, 36, 0, 0, time.UTC)
	for _, reader := range []runstate.ConfigReader{
		{Service: "dashboard", PID: 4242, Build: "0364141b2c3d4e5f", ConfigPath: configPath, StartedAt: started, Keys: older},
		{Service: "scheduler", PID: 4343, Build: "9870df6a1b2c3d4e", ConfigPath: configPath, StartedAt: started, Keys: config.SchemaKeys()},
		{Service: "slack", PID: 4444, Build: "0364141b2c3d4e5f", ConfigPath: configPath, StartedAt: started, Keys: older},
	} {
		if err := store.Record(reader); err != nil {
			t.Fatalf("Record(%s) error = %v", reader.Service, err)
		}
	}
	pipeline.ConfigReaders = store

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Status != runstate.StatusSucceeded || outcome.Integration == nil {
		t.Fatalf("outcome = %#v, want the run landed", outcome)
	}
	if len(outcome.ConfigMismatches) != 1 || outcome.ConfigMismatches[0].Service != "dashboard" {
		t.Fatalf("ConfigMismatches = %+v, want the dashboard alone", outcome.ConfigMismatches)
	}
	notes := strings.Join(tracker.NoteRecords, "\n")
	for _, want := range []string{"Running parts that cannot read the configuration this landing left", "the dashboard service", "build 0364141b2c3d", "pid 4242", "agents.developer.effort", "`yoyo dashboard`"} {
		if !strings.Contains(notes, want) {
			t.Errorf("item notes do not name %q:\n%s", want, notes)
		}
	}
	for _, unwanted := range []string{"the scheduler service", "the slack service"} {
		if strings.Contains(notes, unwanted) {
			t.Errorf("item notes name %q, which reads the key or is not running:\n%s", unwanted, notes)
		}
	}
}

// A landing where every running part reads the configuration says nothing.
func TestALandingEveryRunningPartReadsSaysNothingOfThem(t *testing.T) {
	t.Parallel()

	repository := pipelineRepository(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline, _ := newAutomaticPipeline(t, repository, tracker, provider, []string{"true"})
	store, err := runstate.NewConfigReaderStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	pipeline.ConfigReaders = store

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(outcome.ConfigMismatches) != 0 || strings.Contains(strings.Join(tracker.NoteRecords, "\n"), "cannot read the configuration") {
		t.Fatalf("outcome names %+v and notes %v, want nothing said", outcome.ConfigMismatches, tracker.NoteRecords)
	}
}
