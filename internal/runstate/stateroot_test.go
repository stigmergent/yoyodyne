package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// machine is a described machine: a home directory, the variables its shell
// exports, and optionally a machine file in its configurations home.
type machine struct {
	home string
	env  map[string]string
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	return &machine{home: t.TempDir(), env: map[string]string{}}
}

func (m *machine) getenv(key string) string { return m.env[key] }

func (m *machine) homeDir() (string, error) { return m.home, nil }

func (m *machine) writeMachineFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(m.home, ".config", "yoyodyne", MachineFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (m *machine) resolve(t *testing.T) ResolvedRoot {
	t.Helper()
	resolved, err := ResolveRoot(m.getenv, m.homeDir, "linux")
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	return resolved
}

func TestTheStateRootResolvesInTheOrderTheDesignRules(t *testing.T) {
	t.Parallel()

	m := newMachine(t)
	if got := m.resolve(t); got.Path != filepath.Join(m.home, ".local", "state", "yoyodyne") || got.Origin != RootOriginPlatformDefault {
		t.Fatalf("nothing configured = %+v, want the platform default", got)
	}

	m.env["XDG_STATE_HOME"] = "/xdg"
	if got := m.resolve(t); got.Path != filepath.Join("/xdg", "yoyodyne") || got.Origin != RootOriginXDG {
		t.Fatalf("XDG only = %+v, want XDG_STATE_HOME/yoyodyne", got)
	}

	machinePath := m.writeMachineFile(t, "state_root: /machine/state\n")
	if got := m.resolve(t); got.Path != "/machine/state" || got.Origin != "machine:"+machinePath {
		t.Fatalf("machine key over XDG = %+v, want the machine key naming its file", got)
	}

	m.env[StateHomeVariable] = "/explicit"
	if got := m.resolve(t); got.Path != "/explicit" || got.Origin != RootOriginEnvironment {
		t.Fatalf("variable over machine key = %+v, want YOYODYNE_STATE_HOME", got)
	}
}

func TestTheMachineFileIsFoundInTheConfigurationsHomeItNames(t *testing.T) {
	t.Parallel()

	m := newMachine(t)
	home := t.TempDir()
	m.env["YOYODYNE_CONFIG_HOME"] = home
	if err := os.WriteFile(filepath.Join(home, MachineFileName), []byte("state_root: /relocated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := m.resolve(t); got.Path != "/relocated" {
		t.Fatalf("ResolveRoot() = %+v, want the machine file under YOYODYNE_CONFIG_HOME", got)
	}
}

func TestAMachineFileThatSaysNothingUsefulIsRefusedOrIgnored(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"relative":     "state_root: state\n",
		"unknown key":  "state_rot: /state\n",
		"not yaml map": "- /state\n",
	} {
		m := newMachine(t)
		m.writeMachineFile(t, content)
		if _, err := ResolveRoot(m.getenv, m.homeDir, "linux"); err == nil {
			t.Errorf("%s: ResolveRoot() accepted %q", name, content)
		}
	}
	for name, content := range map[string]string{"empty file": "", "empty key": "state_root: \"\"\n"} {
		m := newMachine(t)
		m.writeMachineFile(t, content)
		if got := m.resolve(t); got.Origin != RootOriginPlatformDefault {
			t.Errorf("%s: ResolveRoot() = %+v, want the platform default", name, got)
		}
	}
}

func gitCheckout(t *testing.T) string {
	t.Helper()
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return checkout
}

func TestTheFirstProcessRecordsTheRootAndASecondRootIsRefused(t *testing.T) {
	t.Parallel()

	checkout := gitCheckout(t)
	first := ResolvedRoot{Path: t.TempDir(), Origin: RootOriginPlatformDefault}
	if err := AgreeRoot(checkout, first); err != nil {
		t.Fatalf("AgreeRoot() first = %v", err)
	}
	marker := filepath.Join(checkout, ".git", "yoyodyne", "state-root")
	content, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if strings.TrimSpace(string(content)) != first.Path {
		t.Fatalf("marker = %q, want %q", content, first.Path)
	}
	if err := AgreeRoot(checkout, first); err != nil {
		t.Fatalf("AgreeRoot() same root again = %v", err)
	}

	second := ResolvedRoot{Path: t.TempDir(), Origin: RootOriginEnvironment}
	err = AgreeRoot(checkout, second)
	var split *SplitRootError
	if !errors.As(err, &split) {
		t.Fatalf("AgreeRoot() second root = %v, want a SplitRootError", err)
	}
	for _, want := range []string{first.Path, second.Path, marker, RootOriginEnvironment, "yoyo stop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %q", err, want)
		}
	}
	if content, _ := os.ReadFile(marker); strings.TrimSpace(string(content)) != first.Path {
		t.Fatalf("the refused process rewrote the marker to %q", content)
	}
}

func TestARootReachedThroughASymlinkIsTheRootItLinksTo(t *testing.T) {
	t.Parallel()

	checkout := gitCheckout(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := AgreeRoot(checkout, ResolvedRoot{Path: real}); err != nil {
		t.Fatal(err)
	}
	if err := AgreeRoot(checkout, ResolvedRoot{Path: link}); err != nil {
		t.Fatalf("AgreeRoot() through a symlink = %v, want agreement", err)
	}
}

func TestALinkedWorktreeSharesItsPrimaryCheckoutsMarker(t *testing.T) {
	t.Parallel()

	primary := gitCheckout(t)
	private := filepath.Join(primary, ".git", "worktrees", "one")
	if err := os.MkdirAll(private, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+private+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := ResolvedRoot{Path: t.TempDir()}
	if err := AgreeRoot(primary, first); err != nil {
		t.Fatal(err)
	}
	if err := AgreeRoot(worktree, ResolvedRoot{Path: t.TempDir()}); err == nil {
		t.Fatal("a process in a linked worktree resolved a second root and was let start")
	}
	if err := AgreeRoot(worktree, first); err != nil {
		t.Fatalf("AgreeRoot() from the worktree on the recorded root = %v", err)
	}
}

func TestADirectoryThatIsNotACheckoutKeepsNoMarker(t *testing.T) {
	t.Parallel()

	plain := t.TempDir()
	if err := AgreeRoot(plain, ResolvedRoot{Path: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := AgreeRoot(plain, ResolvedRoot{Path: t.TempDir()}); err != nil {
		t.Fatalf("AgreeRoot() on a non-checkout = %v, want nothing recorded or compared", err)
	}
	marker, err := ReadRootMarker(plain)
	if err != nil || marker.Path != "" {
		t.Fatalf("ReadRootMarker() = %+v, %v; want no marker", marker, err)
	}
}

// Two products on one machine share the root by default, and the operator hold
// at the root is one switch over both: each product's own checkout records the
// shared root, neither refuses the other, and a hold placed through one is read
// through the other.
func TestTwoProductsShareOneRootAndOneOperatorHold(t *testing.T) {
	t.Parallel()

	shared := ResolvedRoot{Path: t.TempDir(), Origin: RootOriginPlatformDefault}
	yoyodyne, conductor := gitCheckout(t), gitCheckout(t)
	for _, checkout := range []string{yoyodyne, conductor} {
		if err := AgreeRoot(checkout, shared); err != nil {
			t.Fatalf("AgreeRoot(%s) = %v", checkout, err)
		}
	}

	placing, err := NewOperatorHoldStore(shared.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := placing.Hold(time.Now()); err != nil {
		t.Fatal(err)
	}
	reading, err := NewOperatorHoldStore(shared.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, held, err := reading.Held(); err != nil || !held {
		t.Fatalf("Held() through the second product = %v, %v; want the machine-wide hold", held, err)
	}

	first, err := NewStore(shared.Path, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStore(shared.Path, "context-conductor")
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []*Store{first, second} {
		if !strings.HasPrefix(store.Root(), filepath.Join(shared.Path, "products")+string(filepath.Separator)) {
			t.Errorf("store %s is not a product's own directory under the shared root", store.Root())
		}
	}
	if first.Root() == second.Root() {
		t.Fatalf("both products keep their runs in %s", first.Root())
	}
}
