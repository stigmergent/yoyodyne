package supervise

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// fakeCheckout answers the Git questions the rebuilder asks about a checkout
// whose branch stands at tip, and records every build it is asked for.
type fakeCheckout struct {
	branch   string
	tip      string
	changed  string
	dirty    string
	fail     bool
	builds   []execution.Command
	commands []string
}

func (f *fakeCheckout) Run(_ context.Context, command execution.Command, _ execution.OutputObserver) (execution.ProcessResult, error) {
	joined := strings.Join(append([]string{command.Name}, command.Args...), " ")
	f.commands = append(f.commands, joined)
	succeeded := func(stdout string) (execution.ProcessResult, error) {
		return execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: stdout}, nil
	}
	switch {
	case command.Name == "make":
		f.builds = append(f.builds, command)
		if f.fail {
			return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 2, Stderr: "internal/cli/x.go:1: syntax error"}, nil
		}
		return succeeded("")
	case strings.Contains(joined, "symbolic-ref"):
		if f.branch == "" {
			return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 1}, nil
		}
		return succeeded(f.branch + "\n")
	case strings.Contains(joined, "rev-parse"):
		return succeeded(f.tip + "\n")
	case strings.Contains(joined, " diff "):
		return succeeded(f.changed)
	case strings.Contains(joined, " status "):
		return succeeded(f.dirty)
	}
	return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 1}, nil
}

func newRebuilder(t *testing.T, checkout *fakeCheckout, built *string) *Rebuilder {
	t.Helper()
	return &Rebuilder{
		Checkout:  "/src/yoyodyne",
		Binary:    "/src/yoyodyne/bin/yoyo",
		Runner:    checkout,
		Every:     time.Minute,
		BuiltFrom: func(string) (string, error) { return *built, nil },
		Log:       t.Logf,
	}
}

// A landing that touched what the binary is made of is built, once, into the
// binary the product runs from; the same tip is not built again; and the next
// landing is.
func TestTheSupervisorRebuildsTheBinaryWhenTheBranchLands(t *testing.T) {
	t.Parallel()

	built := "aaaa"
	checkout := &fakeCheckout{branch: "main", tip: "aaaa"}
	rebuilder := newRebuilder(t, checkout, &built)
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	rebuilder.Look(context.Background(), start)
	if len(checkout.builds) != 0 {
		t.Fatalf("built %d times with the binary at the tip, want none", len(checkout.builds))
	}

	checkout.tip = "bbbb"
	checkout.changed = "internal/cli/product.go\n"
	rebuilder.Look(context.Background(), start.Add(30*time.Second))
	if len(checkout.builds) != 0 {
		t.Fatal("built before its own interval had passed")
	}
	rebuilder.Look(context.Background(), start.Add(time.Minute))
	if len(checkout.builds) != 1 {
		t.Fatalf("built %d times after a landing, want once", len(checkout.builds))
	}
	build := checkout.builds[0]
	if build.Dir != "/src/yoyodyne" || strings.Join(build.Args, " ") != "build BINARY=/src/yoyodyne/bin/yoyo" {
		t.Errorf("build = make %v in %s, want make build into the product's binary in the checkout", build.Args, build.Dir)
	}

	// The build would have stamped the binary; this one did not, and the tip it
	// was made for is still not built again.
	rebuilder.Look(context.Background(), start.Add(2*time.Minute))
	if len(checkout.builds) != 1 {
		t.Fatalf("built %d times over one tip, want once", len(checkout.builds))
	}

	checkout.tip = "cccc"
	rebuilder.Look(context.Background(), start.Add(3*time.Minute))
	if len(checkout.builds) != 2 {
		t.Fatalf("built %d times after a second landing, want twice", len(checkout.builds))
	}
}

// A landing that touched nothing the binary is made of builds nothing, and a
// checkout with uncommitted changes to what it is made of is not built over.
func TestALandingOfNothingTheBinaryIsMadeOfAndADirtyCheckoutBuildNothing(t *testing.T) {
	t.Parallel()

	built := "aaaa"
	checkout := &fakeCheckout{branch: "main", tip: "bbbb", changed: ""}
	rebuilder := newRebuilder(t, checkout, &built)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	rebuilder.Look(context.Background(), now)
	if len(checkout.builds) != 0 {
		t.Fatal("built for a landing that touched only what the binary is not made of")
	}

	checkout.tip = "cccc"
	checkout.changed = "internal/supervise/rebuild.go\n"
	checkout.dirty = " M internal/cli/doctor.go\n"
	rebuilder.Look(context.Background(), now.Add(time.Minute))
	if len(checkout.builds) != 0 {
		t.Fatal("built over uncommitted changes to what the binary is made of")
	}
	checkout.dirty = ""
	rebuilder.Look(context.Background(), now.Add(2*time.Minute))
	if len(checkout.builds) != 1 {
		t.Fatalf("built %d times once the checkout was clean, want once", len(checkout.builds))
	}
}

// A checkout on no branch has no landing checked out, and builds nothing.
func TestACheckoutOnNoBranchBuildsNothing(t *testing.T) {
	t.Parallel()

	built := "aaaa"
	checkout := &fakeCheckout{tip: "bbbb", changed: "cmd/yoyo/main.go\n"}
	newRebuilder(t, checkout, &built).Look(context.Background(), time.Now())
	if len(checkout.builds) != 0 {
		t.Fatal("built a detached checkout")
	}
}

// The supervisor carries the rebuild on its own tick, beside its children.
func TestTheSupervisorHostsTheRebuildOnItsTick(t *testing.T) {
	t.Parallel()

	built := "aaaa"
	checkout := &fakeCheckout{branch: "main", tip: "bbbb", changed: "go.mod\n"}
	clock := &clock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
	supervisor := newSupervisor(t, newStore(t), clock, &fakeChild{name: config.ServiceScheduler})
	supervisor.Residents = []Resident{newRebuilder(t, checkout, &built)}
	supervisor.Tick(context.Background())
	if len(checkout.builds) != 1 {
		t.Fatalf("a tick built %d times over a landing, want once", len(checkout.builds))
	}
}

// The rebuild is the duty of a product whose binary is built from its own
// checkout, and of nothing else.
func TestARebuilderIsMadeOnlyForABinaryBuiltFromItsCheckout(t *testing.T) {
	t.Parallel()

	checkout := t.TempDir()
	runner := &fakeCheckout{}
	if _, ok := NewRebuilder(checkout, filepath.Join(checkout, "bin", "yoyo"), runner, nil, nil); ok {
		t.Error("a rebuilder was made for a checkout that is not the harness's source")
	}
	if err := os.MkdirAll(filepath.Join(checkout, "cmd", "yoyo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "Makefile"), []byte("build:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := NewRebuilder(checkout, "/usr/local/bin/yoyo", runner, nil, nil); ok {
		t.Error("a rebuilder was made for a binary installed outside the checkout")
	}
	if _, ok := NewRebuilder(checkout, filepath.Join(checkout, "bin", "yoyo"), runner, nil, nil); !ok {
		t.Error("no rebuilder was made for the harness's own binary in its own checkout")
	}
}

// A build that fails is said, and not tried again until the branch moves.
func TestAFailedBuildIsNotRetriedUntilTheBranchMoves(t *testing.T) {
	t.Parallel()

	built := "aaaa"
	checkout := &fakeCheckout{branch: "main", tip: "bbbb", changed: "internal/x.go\n", fail: true}
	var said []string
	rebuilder := newRebuilder(t, checkout, &built)
	rebuilder.Log = func(format string, args ...any) { said = append(said, strings.TrimSpace(format)) }
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	rebuilder.Look(context.Background(), now)
	rebuilder.Look(context.Background(), now.Add(time.Minute))
	if len(checkout.builds) != 1 {
		t.Fatalf("built %d times over one failing tip, want once", len(checkout.builds))
	}
	if len(said) == 0 {
		t.Fatal("a failed build said nothing")
	}
}
