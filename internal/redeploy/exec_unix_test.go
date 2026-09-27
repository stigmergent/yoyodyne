//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package redeploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// takeTargetVariable is how the test below tells the process it starts to
// replace itself rather than to run tests. A restart cannot be observed from
// inside the process that makes it — there is nothing left of it afterwards to
// assert anything — so the observation is made from outside, by a parent reading
// what the replaced image wrote.
const takeTargetVariable = "YOYODYNE_REDEPLOY_TEST_TARGET"

func TestMain(m *testing.M) {
	target := os.Getenv(takeTargetVariable)
	if target == "" {
		os.Exit(m.Run())
	}
	binary, err := at(target, []string{target, "redeployed"}, os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Nothing below this runs when the restart works: this process stops being
	// this process. Reaching the exit at all is the failure the parent reads.
	fmt.Fprintln(os.Stderr, binary.Take(binary.Args()))
	os.Exit(3)
}

// A redeploy is one process where there were two: the image is replaced in
// place, so what the parent started keeps its process identifier and its output
// and is now the other binary.
func TestTakeReplacesTheRunningImage(t *testing.T) {
	echo, err := exec.LookPath("echo")
	if err != nil {
		t.Skipf("this machine has no echo to be replaced by: %v", err)
	}

	restarting := exec.Command(os.Args[0])
	restarting.Env = append(os.Environ(), takeTargetVariable+"="+echo)
	output, err := restarting.CombinedOutput()
	if err != nil {
		t.Fatalf("the process that was to replace itself failed: %v, output %q", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "redeployed" {
		t.Fatalf("output = %q, want the replaced image's own, which is the whole evidence that it was replaced", got)
	}
}

// Handing the watch over starts the deployed build beside this process rather
// than in place of it, as the invocation given, and in a session of its own —
// so the end of the draining process, or of the group it was started in, does
// not take the build with it.
func TestStartRunsTheBuildBesideThisProcessInASessionOfItsOwn(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("this machine has no shell to start: %v", err)
	}
	out := filepath.Join(t.TempDir(), "started")
	binary, err := at(shell, []string{shell}, os.Environ())
	if err != nil {
		t.Fatalf("at() error = %v", err)
	}
	// The process ends by itself a few seconds on, however this test ends.
	pid, err := binary.Start([]string{shell, "-c", `echo "$0" > "$1"; sleep 3`, "handed-over", out})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if pid <= 0 || pid == os.Getpid() {
		t.Fatalf("Start() = pid %d, want a process beside this one", pid)
	}
	if group, err := syscall.Getpgid(pid); err != nil || group != pid {
		t.Fatalf("Getpgid(%d) = %d, %v, want the build leading a group, and a session, of its own", pid, group, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		written, err := os.ReadFile(out)
		if err == nil && strings.TrimSpace(string(written)) == "handed-over" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the started build wrote %q, %v, want the invocation it was given", written, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
