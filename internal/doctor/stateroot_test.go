package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The state finding says where the records are, which layer put them there, and
// whether this checkout's marker agrees. A marker naming another root is the
// problem every command from here would refuse on, and the finding carries both
// roots and the way to move one deliberately.
func TestTheStateFindingReportsTheRootItsOriginAndTheMarker(t *testing.T) {
	t.Parallel()

	world := newWorld(t)
	if err := os.MkdirAll(filepath.Join(world.project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	finding, found := findingFor(world.diagnose(), "state")
	if !found || finding.Status != StatusOK {
		t.Fatalf("state = %+v, want ok before any marker", finding)
	}
	for _, want := range []string{world.stateRoot, runstate.RootOriginEnvironment, "no process has recorded a root"} {
		if !strings.Contains(finding.Summary, want) {
			t.Errorf("state summary %q does not say %q", finding.Summary, want)
		}
	}
	if _, err := os.Stat(filepath.Join(world.project, ".git", "yoyodyne", "state-root")); !os.IsNotExist(err) {
		t.Fatalf("doctor recorded a marker (%v); a diagnosis must never settle which root a product uses", err)
	}

	if err := runstate.AgreeRoot(world.project, runstate.ResolvedRoot{Path: world.stateRoot}); err != nil {
		t.Fatal(err)
	}
	finding, _ = findingFor(world.diagnose(), "state")
	if finding.Status != StatusOK || !strings.Contains(finding.Summary, "agrees") {
		t.Fatalf("state = %s %q, want ok and the marker agreeing", finding.Status, finding.Summary)
	}

	elsewhere := filepath.Join(t.TempDir(), "second-root")
	world.stateRoot = elsewhere
	finding, _ = findingFor(world.diagnose(), "state")
	if finding.Status != StatusProblem {
		t.Fatalf("state = %s %q, want a problem for a marker naming another root", finding.Status, finding.Summary)
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("doctor created %s (%v); a root the marker disagrees with must not be made", elsewhere, err)
	}
	marker := filepath.Join(world.project, ".git", "yoyodyne", "state-root")
	for _, want := range []string{elsewhere, "refuses to start"} {
		if !strings.Contains(finding.Summary, want) {
			t.Errorf("state summary %q does not say %q", finding.Summary, want)
		}
	}
	if !strings.Contains(finding.Remedy, marker) || !strings.Contains(finding.Remedy, "yoyo stop") {
		t.Errorf("state remedy %q does not say how to move the state", finding.Remedy)
	}
}
