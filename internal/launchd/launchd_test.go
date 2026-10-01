package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// fakeLaunchctl answers launchctl as a machine would: the job is loaded when
// the test says so, bootstrap loads it, bootout unloads it.
type fakeLaunchctl struct {
	loaded   bool
	commands []string
}

func (f *fakeLaunchctl) Run(_ context.Context, command execution.Command, _ execution.OutputObserver) (execution.ProcessResult, error) {
	joined := command.Name + " " + strings.Join(command.Args, " ")
	f.commands = append(f.commands, joined)
	switch {
	case strings.HasPrefix(joined, "launchctl print"):
		if !f.loaded {
			return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 113, Stderr: "Could not find service \"com.yoyodyne.supervisor.calc\" in domain for uid: 501"}, nil
		}
	case strings.HasPrefix(joined, "launchctl bootstrap"):
		f.loaded = true
	case strings.HasPrefix(joined, "launchctl bootout"):
		f.loaded = false
	}
	return execution.ProcessResult{Status: execution.ProcessSucceeded}, nil
}

// The plist runs the supervisor verb from the installing binary, starts with
// the machine, is restarted by launchd only on a failure, abandons its process
// group so the children survive it, and carries the operator's PATH and
// nothing secret. Every value is escaped, because a path with an ampersand in
// it is a path.
func TestRenderWritesTheSupervisorJob(t *testing.T) {
	t.Parallel()

	rendered := string(Render(Spec{
		Label:            "com.yoyodyne.supervisor.calc",
		Program:          "/Users/mason/github/calc & co/bin/yoyo",
		Args:             []string{"start", "--foreground", "--config", "/Users/mason/github/calc & co/.yoyodyne/config.yaml"},
		WorkingDirectory: "/Users/mason/github/calc & co",
		Log:              "/state/products/calc/supervisor/supervisor.log",
		Environment:      Environment([]string{"PATH=/opt/homebrew/bin:/usr/bin", "SLACK_BOT_TOKEN=xoxb-1", "HOME=/Users/mason", "YOYODYNE_STATE_HOME=/state"}),
	}))
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		"<key>Label</key>\n  <string>com.yoyodyne.supervisor.calc</string>",
		"<key>ProgramArguments</key>\n  <array>\n    <string>/Users/mason/github/calc &amp; co/bin/yoyo</string>\n    <string>start</string>\n    <string>--foreground</string>\n    <string>--config</string>\n    <string>/Users/mason/github/calc &amp; co/.yoyodyne/config.yaml</string>\n  </array>",
		"<key>WorkingDirectory</key>\n  <string>/Users/mason/github/calc &amp; co</string>",
		"<key>RunAtLoad</key>\n  <true/>",
		"<key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>",
		"<key>AbandonProcessGroup</key>\n  <true/>",
		"<key>StandardOutPath</key>\n  <string>/state/products/calc/supervisor/supervisor.log</string>",
		"<key>StandardErrorPath</key>\n  <string>/state/products/calc/supervisor/supervisor.log</string>",
		"<key>EnvironmentVariables</key>\n  <dict>\n    <key>PATH</key>\n    <string>/opt/homebrew/bin:/usr/bin</string>\n    <key>YOYODYNE_STATE_HOME</key>\n    <string>/state</string>\n  </dict>",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered plist lacks %q:\n%s", want, rendered)
		}
	}
	// YOYODYNE_STATE_HOME is carried; HOME is launchd's to set.
	for _, absent := range []string{"SLACK_BOT_TOKEN", "xoxb", "<key>HOME</key>"} {
		if strings.Contains(rendered, absent) {
			t.Errorf("rendered plist carries %q, which the job is not given:\n%s", absent, rendered)
		}
	}
}

// The agent is named for the product under the user's launch agents, written
// there, read back, told apart from another checkout's by the configuration
// it runs, and driven through launchctl in the user's domain.
func TestTheAgentIsInstalledReadAndDrivenThroughLaunchctl(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	launchctl := &fakeLaunchctl{}
	agent := AgentFor("calc", Controller{Runner: launchctl, UserHomeDir: func() (string, error) { return home, nil }, Getuid: func() int { return 501 }})
	if agent.Label != "com.yoyodyne.supervisor.calc" || agent.Path != filepath.Join(home, "Library", "LaunchAgents", "com.yoyodyne.supervisor.calc.plist") {
		t.Fatalf("agent = %+v, want it named for the product under the user's launch agents", agent)
	}
	if _, found, err := agent.Installed(); err != nil || found {
		t.Fatalf("Installed() = %t, %v before anything was written", found, err)
	}
	if loaded, err := agent.Loaded(context.Background()); err != nil || loaded {
		t.Fatalf("Loaded() = %t, %v before anything was loaded", loaded, err)
	}

	content := Render(Spec{Label: agent.Label, Program: "/opt/yoyo", Args: []string{"start", "--foreground", "--config", "/p/.yoyodyne/config.yaml"}})
	if err := agent.Install(content); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	read, found, err := agent.Installed()
	if err != nil || !found || string(read) != string(content) {
		t.Fatalf("Installed() = %q, %t, %v after Install(), want what was written", read, found, err)
	}
	if runs, err := agent.Runs("/p/.yoyodyne/config.yaml"); err != nil || !runs {
		t.Errorf("Runs(this configuration) = %t, %v, want true", runs, err)
	}
	if runs, err := agent.Runs("/elsewhere/.yoyodyne/config.yaml"); err != nil || runs {
		t.Errorf("Runs(another checkout's configuration) = %t, %v, want false", runs, err)
	}

	if err := agent.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if loaded, err := agent.Loaded(context.Background()); err != nil || !loaded {
		t.Fatalf("Loaded() = %t, %v after Bootstrap()", loaded, err)
	}
	if err := agent.Kickstart(context.Background()); err != nil {
		t.Fatalf("Kickstart() error = %v", err)
	}
	if err := agent.Bootout(context.Background()); err != nil {
		t.Fatalf("Bootout() error = %v", err)
	}
	want := []string{
		"launchctl print gui/501/com.yoyodyne.supervisor.calc",
		"launchctl bootstrap gui/501 " + agent.Path,
		"launchctl print gui/501/com.yoyodyne.supervisor.calc",
		"launchctl kickstart gui/501/com.yoyodyne.supervisor.calc",
		"launchctl bootout gui/501/com.yoyodyne.supervisor.calc",
	}
	if strings.Join(launchctl.commands, "\n") != strings.Join(want, "\n") {
		t.Errorf("launchctl was run as:\n%s\nwant:\n%s", strings.Join(launchctl.commands, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(agent.UninstallCommand(), "launchctl bootout gui/$(id -u)/com.yoyodyne.supervisor.calc; rm ") || !strings.Contains(agent.BootstrapCommand(), "launchctl bootstrap gui/$(id -u) ") {
		t.Errorf("commands = %q and %q", agent.UninstallCommand(), agent.BootstrapCommand())
	}

	// A refusal from launchctl is reported with what it said.
	launchctl.loaded = true
	refusing := &fakeLaunchctl{}
	other := AgentFor("calc", Controller{Runner: refusingRunner{refusing}, UserHomeDir: func() (string, error) { return home, nil }})
	if err := other.Bootstrap(context.Background()); err == nil || !strings.Contains(err.Error(), "Bootstrap failed") {
		t.Errorf("Bootstrap() against a refusing launchctl = %v, want its words", err)
	}
	if _, err := os.Stat(agent.Path); err != nil {
		t.Errorf("the plist was removed by something other than the operator: %v", err)
	}
}

// An agent installed from another shell is the same agent: only its PATH
// differs, and replacing it over that would restart the supervisor and could
// leave it without the tools its parts need. Any other difference is not.
func TestSameIgnoresOnlyThePath(t *testing.T) {
	t.Parallel()

	spec := Spec{Label: Label("calc"), Program: "/opt/yoyo", Args: []string{"start", "--foreground"}, Environment: map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin", "YOYODYNE_STATE_HOME": "/state"}}
	installed := Render(spec)
	fromAnotherShell := spec
	fromAnotherShell.Environment = map[string]string{"PATH": "/usr/bin", "YOYODYNE_STATE_HOME": "/state"}
	if !Same(installed, Render(fromAnotherShell)) {
		t.Errorf("a plist differing only in its PATH read as a different agent")
	}
	moved := spec
	moved.Program = "/elsewhere/yoyo"
	if Same(installed, Render(moved)) {
		t.Errorf("a plist running another binary read as the same agent")
	}
	otherState := spec
	otherState.Environment = map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin", "YOYODYNE_STATE_HOME": "/other"}
	if Same(installed, Render(otherState)) {
		t.Errorf("a plist reading another state root read as the same agent")
	}
	if Label("maintenance") == "com.yoyodyne.maintenance" {
		t.Errorf("a product named maintenance would be given the retired maintenance job's label")
	}
}

type refusingRunner struct{ *fakeLaunchctl }

func (refusingRunner) Run(context.Context, execution.Command, execution.OutputObserver) (execution.ProcessResult, error) {
	return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 5, Stderr: "Bootstrap failed: 5: Input/output error"}, nil
}

// The property list is written confined to the home directory, the launch
// agents directory included: one that does not exist yet is created inside the
// home, and one that is a symlink out of it is refused rather than followed.
func TestInstallIsConfinedToTheHomeDirectory(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	agent := AgentFor("calc", Controller{UserHomeDir: func() (string, error) { return home, nil }})
	if err := agent.Install([]byte("plist")); err != nil {
		t.Fatalf("Install() into a home with no launch agents directory = %v", err)
	}
	if content, err := os.ReadFile(agent.Path); err != nil || string(content) != "plist" {
		t.Fatalf("installed = %q, %v", content, err)
	}

	escaping := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(escaping, "Library"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(escaping, "Library", "LaunchAgents")); err != nil {
		t.Fatal(err)
	}
	escaped := AgentFor("calc", Controller{UserHomeDir: func() (string, error) { return escaping, nil }})
	if err := escaped.Install([]byte("plist")); err == nil {
		t.Fatal("Install() through a launch agents directory linked out of the home succeeded")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("the write landed outside the home: %v", entries)
	}
}
