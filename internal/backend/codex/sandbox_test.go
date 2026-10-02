package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// These are Git's files, without a subprocess or a second real worktree. Spaces
// and quotes exercise the TOML value the CLI actually receives.
func sandboxRepository(t *testing.T, linked bool) (repository, worktree string) {
	t.Helper()
	repository = filepath.Join(t.TempDir(), `repository with "quotes"`)
	git := filepath.Join(repository, ".git")
	if err := os.MkdirAll(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if !linked {
		return repository, repository
	}
	administrative := filepath.Join(git, "worktrees", "one")
	worktree = filepath.Join(t.TempDir(), "worktree")
	if err := os.MkdirAll(administrative, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{
		filepath.Join(worktree, ".git"):            "gitdir: " + administrative + "\n",
		filepath.Join(administrative, "commondir"): "../..\n",
	} {
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repository, worktree
}

func sandboxCommand(t *testing.T, repository, worktree, session string, role domain.AgentRole) execution.Command {
	t.Helper()
	runner := &fakeRunner{results: []execution.ProcessResult{{Status: execution.ProcessSucceeded,
		Stdout: lines(`{"id":"0","msg":{"type":"task_complete","last_agent_message":"ok"}}`)}}}
	_, err := (Backend{Runner: runner, Clock: fixedClock{}}).Run(context.Background(), backendapi.RunRequest{
		RunID: testRunID, Role: role, WorkingDirectory: worktree,
		RepositoryRoot: repository, Prompt: "do the work", SessionID: session,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner.commands[0]
}

func writableDirectories(t *testing.T, args []string) []string {
	t.Helper()
	for _, arg := range args {
		if value, found := strings.CutPrefix(arg, "sandbox_workspace_write.writable_roots="); found {
			var directories []string
			if err := json.Unmarshal([]byte(value), &directories); err != nil {
				t.Fatal(err)
			}
			return directories
		}
	}
	t.Fatalf("no writable directory policy in %q", args)
	return nil
}

func TestDeveloperSandboxGrantsTheDeclaredCacheAndScratchOnEveryTurn(t *testing.T) {
	t.Parallel()
	for _, linked := range []bool{false, true} {
		repository, worktree := sandboxRepository(t, linked)
		scratch, err := execution.PrepareScratchDirectory(repository, worktree, testRunID)
		if err != nil {
			t.Fatal(err)
		}
		git, err := filepath.EvalSymlinks(filepath.Join(repository, ".git"))
		if err != nil {
			t.Fatal(err)
		}
		cache := filepath.Join(git, "yoyodyne", "go-build")
		for _, session := range []string{"", "session-one"} {
			command := sandboxCommand(t, repository, worktree, session, domain.RoleDeveloper)
			if got := writableDirectories(t, command.Args); !reflect.DeepEqual(got, []string{cache, scratch}) {
				t.Fatalf("writable roots = %q, want only the declared cache and scratch", got)
			}
			if !hasEnvironment(command.Env, "GOCACHE="+cache) {
				t.Fatalf("the sandbox grants %q but GOCACHE names another path", cache)
			}
			if err := recordedContract(t).misplacedOption(command.Args); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(command.Args, " ")
			if session != "" && strings.Index(joined, "writable_roots=") > strings.Index(joined, " resume ") {
				t.Fatal("writable directory policy appears after resume")
			}
			for _, forbidden := range []string{"danger-full-access", "--dangerously-bypass", "--approve-for-me", "--add-dir"} {
				if strings.Contains(joined, forbidden) {
					t.Fatalf("unconfined or unsupported option %q in %q", forbidden, command.Args)
				}
			}
		}
	}
}

func TestReadOnlyRolesNeverReceiveDeveloperDirectoryGrants(t *testing.T) {
	t.Parallel()
	repository, worktree := sandboxRepository(t, true)
	for _, role := range domain.Roles() {
		if backendapi.PostureFor(role) != backendapi.PostureReadOnly {
			continue
		}
		for _, session := range []string{"", "session-one"} {
			command := sandboxCommand(t, repository, worktree, session, role)
			if sandboxArgument(t, command.Args) != sandboxReadOnly || strings.Contains(strings.Join(command.Args, " "), "writable_roots=") {
				t.Fatalf("read-only role %s received developer access: %q", role, command.Args)
			}
		}
	}
}

func TestDeveloperSandboxRefusesAnEscapingWorktreeBeforeLaunchOrResume(t *testing.T) {
	t.Parallel()
	repository, worktree := sandboxRepository(t, true)
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+t.TempDir()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"", "session-one"} {
		runner := &fakeRunner{}
		_, err := (Backend{Runner: runner}).Run(context.Background(), backendapi.RunRequest{
			RunID: testRunID, Role: domain.RoleDeveloper, WorkingDirectory: worktree,
			RepositoryRoot: repository, Prompt: "do the work", SessionID: session,
		})
		if err == nil || !strings.Contains(err.Error(), "not inside the repository") || len(runner.commands) != 0 {
			t.Fatalf("Run() = %v, commands = %v, want a refusal before invocation", err, runner.commands)
		}
	}
}

// This exercises the CLI's filesystem enforcement without invoking a model or
// a role. It is opt-in because an outer developer sandbox can refuse a nested
// OS sandbox even though the policy works when the harness launches it.
func TestLocalSandboxConfinesDeveloperAndReadOnlyWrites(t *testing.T) {
	if os.Getenv("YOYODYNE_CODEX_SANDBOX_CONFORMANCE") != "1" {
		t.Skip("set YOYODYNE_CODEX_SANDBOX_CONFORMANCE=1 to exercise the native Codex sandbox")
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	repository, worktree := sandboxRepository(t, true)
	scratch, err := execution.PrepareScratchDirectory(repository, worktree, testRunID)
	if err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{"go.mod": "module sandboxprobe\n\ngo 1.23\n", "probe.go": "package sandboxprobe\n"} {
		if err := os.WriteFile(filepath.Join(worktree, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	otherScratch := filepath.Join(filepath.Dir(scratch), "another-run")
	if err := os.Mkdir(otherScratch, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, role := range []domain.AgentRole{domain.RoleDeveloper, domain.RoleReviewer} {
		for _, session := range []string{"", "session-one"} {
			command := sandboxCommand(t, repository, worktree, session, role)
			if role == domain.RoleReviewer {
				// Run removes its empty launch directory when the fake provider
				// exits. Give this native probe an empty directory of its own.
				command.Dir = t.TempDir()
			}
			args := []string{"sandbox", "--cd", command.Dir, "--config", `sandbox_mode="` + sandboxArgument(t, command.Args) + `"`,
				// Fixture directories live in TMPDIR. Remove the sandbox's normal
				// temporary grants so an unrelated sibling tests actual denial.
				"--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
				"--config", "sandbox_workspace_write.exclude_slash_tmp=true"}
			for index, arg := range command.Args {
				if arg == "--config" {
					args = append(args, arg, command.Args[index+1])
				}
			}
			cache := filepath.Join(repository, ".git", "yoyodyne", "go-build")
			script := `set -eu
if (printf denied > "$3/unrelated") 2>/dev/null; then exit 20; fi
if (printf denied > "$6/unrelated") 2>/dev/null; then exit 24; fi
if (printf denied > "$7/unrelated") 2>/dev/null; then exit 25; fi
if [ "$4" = developer ]; then
  export GOTMPDIR="$2/go-tmp"
  mkdir -p "$GOTMPDIR"
  go test ./... > "$2/check.log" 2>&1
  test -d "$1"
  test -s "$2/check.log"
else
  if (mkdir -p "$1" && printf denied > "$1/read-only") 2>/dev/null; then exit 21; fi
  if (printf denied > "$2/read-only") 2>/dev/null; then exit 22; fi
  if (printf denied > "$5/read-only") 2>/dev/null; then exit 23; fi
fi`
			args = append(args, "--", "sh", "-c", script, "sandbox-probe", cache, scratch, outside, string(role), worktree, filepath.Join(repository, ".git"), otherScratch)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			probe := exec.CommandContext(ctx, binary, args...)
			probe.Env = command.Env
			output, err := probe.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("native sandbox for %s, session %q: %v\n%s", role, session, err, output)
			}
		}
	}
}
