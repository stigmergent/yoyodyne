package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// stateRootProject is a Git checkout carrying its own configuration, which is
// what a process records its state root against.
func stateRootProject(t *testing.T) (project, configPath string) {
	t.Helper()
	project = t.TempDir()
	git(t, project, "init", "-b", "main")
	configPath = filepath.Join(project, config.DirectoryName, config.FileName)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(validConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return project, configPath
}

// Two processes reading different roots refuse rather than diverge: the first
// command records the root it opened in the checkout's marker, and a later one
// that resolved another root — here because a shell exported a different
// YOYODYNE_STATE_HOME — refuses to start, naming both, and records nothing.
func TestAProcessOnASecondStateRootRefusesToStart(t *testing.T) {
	// Not parallel: the state root every command resolves is set for this process.
	t.Setenv("YOYODYNE_CONFIG_HOME", t.TempDir())
	first := t.TempDir()
	t.Setenv("YOYODYNE_STATE_HOME", first)
	project, configPath := stateRootProject(t)

	if _, stderr, code := runCLI(t, "reports", "--config", configPath); code != 0 {
		t.Fatalf("reports on the first root code = %d, stderr = %q", code, stderr)
	}
	marker := filepath.Join(project, ".git", "yoyodyne", "state-root")
	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the first process recorded no marker: %v", err)
	}
	if strings.TrimSpace(string(content)) != first {
		t.Fatalf("marker = %q, want %q", content, first)
	}

	second := t.TempDir()
	t.Setenv("YOYODYNE_STATE_HOME", second)
	for _, args := range [][]string{
		{"reports", "--config", configPath},
		{"status", "--config", configPath},
		{"amendment", "list", "--config", configPath},
	} {
		_, stderr, code := runCLI(t, args...)
		if code == 0 {
			t.Fatalf("%v on a second root succeeded; want a refusal", args)
		}
		for _, want := range []string{first, second, marker, runstate.RootOriginEnvironment} {
			if !strings.Contains(stderr, want) {
				t.Errorf("%v refusal %q does not name %q", args, stderr, want)
			}
		}
	}
	if entries, err := os.ReadDir(second); err != nil || len(entries) != 0 {
		t.Fatalf("the refused processes wrote under the second root: %v, %v", entries, err)
	}

	t.Setenv("YOYODYNE_STATE_HOME", first)
	if _, stderr, code := runCLI(t, "reports", "--config", configPath); code != 0 {
		t.Fatalf("reports back on the recorded root code = %d, stderr = %q", code, stderr)
	}
}

// The machine key sets the root when no variable does, and config show says so
// by the file that set it.
func TestTheMachineKeySetsTheRootAndConfigShowNamesIt(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("YOYODYNE_CONFIG_HOME", configHome)
	t.Setenv("YOYODYNE_STATE_HOME", "")
	root := t.TempDir()
	machinePath := filepath.Join(configHome, runstate.MachineFileName)
	if err := os.WriteFile(machinePath, []byte("state_root: "+root+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project, configPath := stateRootProject(t)

	stdout, stderr, code := runCLI(t, "config", "show", "--origins", "--config", configPath)
	if code != 0 {
		t.Fatalf("config show code = %d, stderr = %q", code, stderr)
	}
	for _, want := range []string{
		"# state root: " + root + " (from machine:" + machinePath + ")",
		"state_root: machine:" + machinePath,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("config show does not say %q:\n%s", want, stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".git", "yoyodyne", "state-root")); !os.IsNotExist(err) {
		t.Fatalf("config show recorded a marker (%v); reporting the root must record nothing", err)
	}

	stdout, stderr, code = runCLI(t, "config", "show", "--json", "--config", configPath)
	if code != 0 {
		t.Fatalf("config show --json code = %d, stderr = %q", code, stderr)
	}
	var payload struct {
		StateRoot map[string]string `json:"state_root"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.StateRoot["path"] != root || payload.StateRoot["origin"] != "machine:"+machinePath {
		t.Fatalf("config show --json state_root = %v", payload.StateRoot)
	}

	if _, stderr, code := runCLI(t, "reports", "--config", configPath); code != 0 {
		t.Fatalf("reports code = %d, stderr = %q", code, stderr)
	}
	if content, _ := os.ReadFile(filepath.Join(project, ".git", "yoyodyne", "state-root")); strings.TrimSpace(string(content)) != root {
		t.Fatalf("marker = %q, want the machine key's root %q", content, root)
	}
}
