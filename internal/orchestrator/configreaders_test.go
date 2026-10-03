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

// unmovedCheckout stands in for a primary checkout the landing has not moved —
// a target the forge protects, where nothing is moved locally until the forge
// merges: when the landing asks, the working file does not carry what landed.
type unmovedCheckout struct {
	store   *runstate.ConfigReaderStore
	path    string
	content []byte
}

func (u unmovedCheckout) MismatchesIn(read func(string) ([]byte, error)) ([]runstate.ConfigMismatch, error) {
	if err := os.WriteFile(u.path, u.content, 0o600); err != nil {
		return nil, err
	}
	return u.store.MismatchesIn(read)
}

func (u unmovedCheckout) TemplateMismatches(path string, added []string) ([]runstate.ConfigTemplateMismatch, error) {
	return u.store.TemplateMismatches(path, added)
}

// The landed change is what adds the key, and the primary checkout's file does
// not carry it when the landing asks. The landing reads the file as the
// integrated commit holds it, so the dashboard on a build from before the key
// is still named.
func TestALandingThatAddsAKeyNamesThePartsFromTheLandedCommit(t *testing.T) {
	t.Parallel()

	repository := pipelineRepository(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		directory := filepath.Join(request.WorkingDirectory, "deploy")
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(directory, "config.yaml"), []byte("agents:\n  developer:\n    role: developer\n    effort: medium\n"), 0o644)
	}, approveVerdict)
	pipeline, _ := newAutomaticPipeline(t, repository, tracker, provider, []string{"true"})

	configPath := filepath.Join(repository, "deploy", "config.yaml")
	store, err := runstate.NewConfigReaderStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	store = store.WithProcessCheck(func(pid int) (bool, error) { return pid == 4242, nil })
	older := slices.DeleteFunc(config.SchemaKeys(), func(key string) bool { return key == "agents.*.effort" })
	if err := store.Record(runstate.ConfigReader{
		Service: "dashboard", PID: 4242, Build: "0364141b2c3d4e5f", ConfigPath: configPath,
		StartedAt: time.Date(2026, 9, 26, 23, 36, 0, 0, time.UTC), Keys: older,
	}); err != nil {
		t.Fatal(err)
	}
	pipeline.ConfigReaders = unmovedCheckout{store: store, path: configPath, content: []byte("agents:\n  developer:\n    role: developer\n")}

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.Status != runstate.StatusSucceeded || outcome.Integration == nil {
		t.Fatalf("outcome = %#v, want the run landed", outcome)
	}
	if len(outcome.ConfigMismatches) != 1 || outcome.ConfigMismatches[0].Service != "dashboard" ||
		!slices.Equal(outcome.ConfigMismatches[0].Keys, []string{"agents.developer.effort"}) {
		t.Fatalf("ConfigMismatches = %+v, want the dashboard named for the landed key", outcome.ConfigMismatches)
	}
	if notes := strings.Join(tracker.NoteRecords, "\n"); !strings.Contains(notes, "the dashboard service") || !strings.Contains(notes, "agents.developer.effort") {
		t.Fatalf("item notes do not name the dashboard and the key:\n%s", notes)
	}
}

func TestALandingAddingOnlyTemplateKeysNamesIncompatibleRunningBuilds(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, after string
		wantMismatch        bool
	}{
		{
			name:         "new merged template key",
			before:       "agents: {developer: {role: developer, model: opus}}\n",
			after:        "agents: {developer: {<<: {role: developer, model: opus, effort: medium}}}\n",
			wantMismatch: true,
		},
		{
			name:   "existing key with a new value",
			before: "agents: {developer: {role: developer, effort: medium}}\n",
			after:  "agents: {developer: {role: developer, effort: high}}\n",
		},
		{
			name:         "new template",
			after:        "agents: {developer: {role: developer, effort: medium}}\n",
			wantMismatch: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository := pipelineRepository(t)
			template := "internal/config/builtin/v1/bundle.yaml"
			templatePath := filepath.Join(repository, filepath.FromSlash(template))
			if test.before != "" {
				if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(templatePath, []byte(test.before), 0o644); err != nil {
					t.Fatal(err)
				}
				runPipelineGit(t, repository, "add", template)
				runPipelineGit(t, repository, "commit", "-m", "initial shipped template")
			}
			tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
			provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
				path := filepath.Join(request.WorkingDirectory, filepath.FromSlash(template))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				return os.WriteFile(path, []byte(test.after), 0o644)
			}, approveVerdict)
			pipeline, _ := newAutomaticPipeline(t, repository, tracker, provider, []string{"true"})
			stateRoot := t.TempDir()
			configPath := filepath.Join(stateRoot, "config.yaml")
			if err := os.WriteFile(configPath, []byte("agents: {developer: {role: developer}}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := runstate.NewConfigReaderStore(stateRoot, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			store = store.WithProcessCheck(func(int) (bool, error) { return true, nil })
			older := slices.DeleteFunc(config.SchemaKeys(), func(key string) bool { return key == "agents.*.effort" })
			services := []string{"dashboard", "scheduler", "slack", runstate.ConfigReaderSupervisor}
			for index, service := range services {
				if err := store.Record(runstate.ConfigReader{
					Service: service, PID: 4200 + index, Build: "0364141b2c3d4e5f",
					ConfigPath: configPath, StartedAt: time.Now(), Keys: older,
				}); err != nil {
					t.Fatal(err)
				}
			}
			// Move the checkout's template back before the landing comparison. Both
			// the introduced keys and their previous version must come from Git.
			pipeline.ConfigReaders = unmovedCheckout{store: store, path: templatePath, content: []byte(test.before)}
			outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
			if err != nil || outcome.Status != runstate.StatusSucceeded {
				t.Fatalf("Run() = %s, %v", outcome.Status, err)
			}
			if len(outcome.ConfigMismatches) != 0 {
				t.Fatalf("healthy active files named as broken: %+v", outcome.ConfigMismatches)
			}
			notes := strings.Join(tracker.NoteRecords, "\n")
			if !test.wantMismatch {
				if len(outcome.TemplateConfigMismatches) != 0 || strings.Contains(notes, "new keys in shipped templates") {
					t.Fatalf("an existing key was reported newly introduced: %+v; %s", outcome.TemplateConfigMismatches, notes)
				}
				return
			}
			if len(outcome.TemplateConfigMismatches) != len(services) {
				t.Fatalf("template mismatches = %+v, want all four running services", outcome.TemplateConfigMismatches)
			}
			for _, service := range services {
				if !strings.Contains(notes, "the "+service+" service") {
					t.Errorf("template-only landing does not name %s: %s", service, notes)
				}
			}
			for _, want := range []string{"new keys in shipped templates", "build 0364141b2c3d", "agents.*.effort", template, "adopting those keys", "would make"} {
				if !strings.Contains(notes, want) {
					t.Errorf("prospective landing note lacks %q: %s", want, notes)
				}
			}
			if strings.Contains(notes, "Running parts that cannot read the configuration this landing left") {
				t.Fatalf("a prospective warning claims the active file is unreadable: %s", notes)
			}
		})
	}
}
