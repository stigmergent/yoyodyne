package maintenancejob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// fakeLaunchd is launchd as launchctl reports it: a job loaded from a path, or
// nothing, and a bootout that unloads it. It records what it was asked.
type fakeLaunchd struct {
	loaded     bool
	loadedFrom string
	refuse     bool
	asked      [][]string
}

func (f *fakeLaunchd) Run(_ context.Context, command execution.Command, _ execution.OutputObserver) (execution.ProcessResult, error) {
	f.asked = append(f.asked, append([]string{command.Name}, command.Args...))
	if command.Name != "launchctl" {
		return execution.ProcessResult{}, errors.New("only launchctl is asked")
	}
	switch command.Args[0] {
	case "print":
		if !f.loaded {
			return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 113, Stderr: "Could not find service"}, nil
		}
		return execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: "gui/501/com.yoyodyne.maintenance = {\n\tactive count = 0\n\tpath = " + f.loadedFrom + "\n\tstate = not running\n}\n"}, nil
	case "bootout":
		if f.refuse {
			return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 5, Stderr: "Boot-out failed: 5: Input/output error"}, nil
		}
		f.loaded = false
		return execution.ProcessResult{Status: execution.ProcessSucceeded}, nil
	}
	return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 1}, nil
}

const jobScript = `#!/bin/bash
cd /Users/someone/github/yoyodyne
bin/yoyo reconcile 2>/dev/null
make build >/dev/null 2>&1
nohup bin/yoyo work --watch >> scheduler.log 2>&1 &
bin/yoyo slack ensure 2>&1
# nohup bin/yoyo dashboard -port 8080 is commented out, so not a duty
`

// installJob writes the job's property list and script under a home, the way
// the operator installed it.
func installJob(t *testing.T, home string) (plist, script string) {
	t.Helper()
	script = filepath.Join(home, ".local", "yoyodyne", "yoyodyne-maintenance.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte(jobScript), 0o755); err != nil {
		t.Fatal(err)
	}
	plist = filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.yoyodyne.maintenance</string>
  <key>ProgramArguments</key>
  <array><string>` + script + `</string></array>
  <key>StartInterval</key><integer>600</integer>
  <key>AbandonProcessGroup</key><true/>
</dict>
</plist>
`
	if err := os.WriteFile(plist, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return plist, script
}

// A loaded job is found with what it runs and what of the product it
// duplicates, read from its script, and a commented-out step is not a duty.
func TestInspectFindsTheLoadedJobAndWhatItDuplicates(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist, script := installJob(t, home)
	launchd := &fakeLaunchd{loaded: true, loadedFrom: plist}
	found, err := Machine{Home: home, UID: 501, GOOS: "darwin", Runner: launchd}.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if !found.Installed || !found.Loaded || !found.Ours() {
		t.Fatalf("found = %+v, want the job installed, loaded, and this user's", found)
	}
	if !slices.Equal(found.Program, []string{script}) {
		t.Errorf("program = %v, want the script", found.Program)
	}
	if got := DutyNames(found.Duplicates); !slices.Equal(got, []string{"scheduler", "slack", "maintenance", "rebuild"}) {
		t.Errorf("duplicates = %v, want the scheduler, the sink, reconcile, and the rebuild", got)
	}
	if found.DutiesInferred {
		t.Error("the duties were read from the script, not inferred")
	}
	if len(launchd.asked) != 1 || strings.Join(launchd.asked[0], " ") != "launchctl print gui/501/com.yoyodyne.maintenance" {
		t.Errorf("asked %v, want one launchctl print of the user's domain", launchd.asked)
	}
}

// No job, and no launchd, are both nothing found.
func TestInspectFindsNothingWhereThereIsNoJob(t *testing.T) {
	t.Parallel()

	found, err := Machine{Home: t.TempDir(), UID: 501, GOOS: "darwin", Runner: &fakeLaunchd{}}.Inspect(context.Background())
	if err != nil || found.Present() {
		t.Fatalf("Inspect() = %+v, %v, want nothing found", found, err)
	}
	launchd := &fakeLaunchd{loaded: true}
	found, err = Machine{Home: t.TempDir(), UID: 501, GOOS: "linux", Runner: launchd}.Inspect(context.Background())
	if err != nil || found.Present() || len(launchd.asked) != 0 {
		t.Fatalf("Inspect() on linux = %+v, %v, asked %v, want nothing found and nothing asked", found, err, launchd.asked)
	}
}

// Retiring the job boots it out and removes its property list, leaves its
// script, and a second retirement finds nothing to do.
func TestRetireUnloadsAndRemovesTheJob(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist, script := installJob(t, home)
	launchd := &fakeLaunchd{loaded: true, loadedFrom: plist}
	machine := Machine{Home: home, UID: 501, GOOS: "darwin", Runner: launchd}
	retirement, err := machine.Retire(context.Background())
	if err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if !retirement.Unloaded || !retirement.Removed || !retirement.Acted() {
		t.Fatalf("retirement = %+v, want the job unloaded and removed", retirement)
	}
	if launchd.loaded {
		t.Error("the job is still loaded")
	}
	if _, err := os.Stat(plist); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the property list is still there: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Errorf("the script was not left where it was: %v", err)
	}
	if !strings.Contains(retirement.Left, script) {
		t.Errorf("left = %q, want the script named as left alone", retirement.Left)
	}
	said := retirement.Describe()
	for _, want := range []string{"booted it out of launchd", "removed " + plist, "yoyo work --watch", "rebuilds bin/yoyo"} {
		if !strings.Contains(said, want) {
			t.Errorf("Describe() = %q, want %q in it", said, want)
		}
	}

	again, err := machine.Retire(context.Background())
	if err != nil || again.Acted() {
		t.Fatalf("second Retire() = %+v, %v, want nothing to do", again, err)
	}
}

// A job of the label launchd loaded from somewhere other than this user's
// LaunchAgents is not the product's to remove: it is left loaded and said so.
func TestRetireLeavesAJobLoadedFromElsewhere(t *testing.T) {
	t.Parallel()

	launchd := &fakeLaunchd{loaded: true, loadedFrom: "/Library/LaunchAgents/com.yoyodyne.maintenance.plist"}
	retirement, err := Machine{Home: t.TempDir(), UID: 501, GOOS: "darwin", Runner: launchd}.Retire(context.Background())
	if err != nil {
		t.Fatalf("Retire() error = %v", err)
	}
	if retirement.Acted() || !launchd.loaded {
		t.Fatalf("retirement = %+v, want nothing done to a job loaded from elsewhere", retirement)
	}
	if !strings.Contains(retirement.Left, "/Library/LaunchAgents/com.yoyodyne.maintenance.plist") {
		t.Errorf("left = %q, want where launchd loaded it named", retirement.Left)
	}
}

// A bootout launchd refuses while the job stays loaded is a failure, and the
// property list is not removed from under a job that is still running.
func TestRetireFailsWhereTheJobWillNotUnload(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist, _ := installJob(t, home)
	launchd := &fakeLaunchd{loaded: true, loadedFrom: plist, refuse: true}
	_, err := Machine{Home: home, UID: 501, GOOS: "darwin", Runner: launchd}.Retire(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Input/output error") {
		t.Fatalf("Retire() error = %v, want launchctl's refusal", err)
	}
	if _, statErr := os.Stat(plist); statErr != nil {
		t.Errorf("the property list was removed from a job that is still loaded: %v", statErr)
	}
}

// A script that cannot be read still names what the job is known to do.
func TestAJobWhoseScriptCannotBeReadIsTakenToDoEverything(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	plist, script := installJob(t, home)
	if err := os.Remove(script); err != nil {
		t.Fatal(err)
	}
	found, err := Machine{Home: home, UID: 501, GOOS: "darwin", Runner: &fakeLaunchd{loaded: true, loadedFrom: plist}}.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !found.DutiesInferred || len(found.Duplicates) != len(knownDuties) {
		t.Fatalf("found = %+v, want every known duty, marked inferred", found)
	}
}
