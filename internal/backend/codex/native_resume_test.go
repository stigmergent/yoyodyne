package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

const nativeProbeReply = "native sandbox probe passed"

// The provider is a local scripted Responses server, not a paid model. It asks
// the real CLI to execute one shell command, then ends the turn. The CLI creates
// and restores its own session; neither the rollout nor its policy is faked.
// There is no opt-in flag: an installed CLI whose sandbox cannot run fails this
// check instead of turning absent confinement evidence into a passing suite.
func TestNativeResumeReplacesSavedDirectoryGrants(t *testing.T) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skipf("native resume requires an installed Codex CLI: %v", err)
	}
	home := t.TempDir()
	preflight, err := (execution.OSProcessRunner{}).Run(context.Background(), execution.Command{
		Name: binary, Args: []string{"sandbox", "--config", `sandbox_mode="read-only"`, "--", "sh", "-c", "true"},
		Dir: home, Env: readOnlyEnvironment(home), Timeout: 10 * time.Second,
	}, nil)
	if err != nil || preflight.Status != execution.ProcessSucceeded {
		t.Fatalf("native sandbox could not execute the probe: %v\n%s", err, preflight.Stderr)
	}

	oldRepository, oldWorktree := sandboxRepository(t, true)
	newRepository, newWorktree := sandboxRepository(t, true)
	oldPaths := nativeProbeDirectories(t, oldRepository, oldWorktree)
	newPaths := nativeProbeDirectories(t, newRepository, newWorktree)
	outside := t.TempDir()
	otherScratch := filepath.Join(filepath.Dir(newPaths[1]), "another-run")
	if err := os.Mkdir(otherScratch, 0o700); err != nil {
		t.Fatal(err)
	}
	model := &sandboxResponses{}
	server := httptest.NewServer(model)
	defer server.Close()
	provider := Backend{Binary: binary, Runner: sandboxCLIRunner{home: home, url: server.URL}}
	request := backendapi.RunRequest{
		RunID: testRunID, Role: domain.RoleDeveloper, WorkingDirectory: oldWorktree,
		RepositoryRoot: oldRepository, AccountConfigDir: home, Model: "gpt-5",
		Prompt: "Execute the supplied sandbox probe, then finish.", Timeout: time.Minute, IdleTimeout: 20 * time.Second,
	}
	denied := []string{outside, otherScratch, filepath.Join(oldRepository, ".git"), filepath.Join(newRepository, ".git")}
	model.begin(nativeProbeCommand(t, oldWorktree, oldPaths, append(denied, append(newPaths, newWorktree)...)))
	fresh := nativeProbeTurn(t, provider, request)
	if fresh.SessionID == "" {
		t.Fatal("the fresh CLI invocation did not create a resumable session")
	}
	model.requireTurn(t)

	request.SessionID = fresh.SessionID
	request.RepositoryRoot, request.WorkingDirectory = newRepository, newWorktree
	model.begin(nativeProbeCommand(t, newWorktree, newPaths, append(denied, append(oldPaths, oldWorktree)...)))
	resumed := nativeProbeTurn(t, provider, request)
	if resumed.SessionID != fresh.SessionID {
		t.Fatalf("resume created session %q instead of restoring %q", resumed.SessionID, fresh.SessionID)
	}
	model.requireTurn(t)

	// Restore that same previously writable session under the reviewer's native
	// posture. Old and current cache, scratch, worktrees, and unrelated paths
	// must all be read-only; permission retained from either turn fails the test.
	request.Role = domain.RoleReviewer
	allDenied := append(append(denied, oldPaths...), newPaths...)
	allDenied = append(allDenied, oldWorktree, newWorktree)
	model.begin(nativeProbeCommand(t, newWorktree, nil, allDenied))
	readOnly := nativeProbeTurn(t, provider, request)
	if readOnly.SessionID != fresh.SessionID {
		t.Fatalf("read-only resume created session %q instead of restoring %q", readOnly.SessionID, fresh.SessionID)
	}
	model.requireTurn(t)
}

func nativeProbeDirectories(t *testing.T, repository, worktree string) []string {
	t.Helper()
	for file, body := range map[string]string{"go.mod": "module sandboxprobe\n\ngo 1.23\n", "probe.go": "package sandboxprobe\n"} {
		if err := os.WriteFile(filepath.Join(worktree, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := execution.PrepareDeveloperDirectories(repository, worktree, testRunID)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func nativeProbeCommand(t *testing.T, worktree string, allowed, denied []string) []string {
	t.Helper()
	physical, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		t.Fatal(err)
	}
	mode, cache, scratch := "read-only", "", ""
	if len(allowed) != 0 {
		mode, cache, scratch = "developer", allowed[0], allowed[1]
	}
	script := `set -eu
mode=$1; worktree=$2; cache=$3; scratch=$4; shift 4
if [ "$mode" = developer ]; then
  test "$(pwd -P)" = "$worktree"
  test "$GOCACHE" = "$cache"
  export GOTMPDIR="$scratch/go-tmp" GOTELEMETRY=off
  mkdir -p "$GOTMPDIR"
  go test ./... > "$scratch/check.log" 2>&1
  test -s "$scratch/check.log"
  printf allowed > "$worktree/allowed"
else
  test "$(pwd -P)" != "$worktree"
fi
for directory do
  if (printf forbidden > "$directory/forbidden") 2>/dev/null; then
    printf 'unexpected write access: %s\n' "$directory"; exit 20
  fi
done
printf 'native sandbox probe passed\n'`
	return append([]string{"sh", "-c", script, "sandbox-probe", mode, physical, cache, scratch}, denied...)
}

func nativeProbeTurn(t *testing.T, provider Backend, request backendapi.RunRequest) backendapi.RunResult {
	t.Helper()
	result, err := provider.Run(context.Background(), request)
	if err != nil || result.IsError || result.Process.Status != execution.ProcessSucceeded {
		t.Fatalf("native CLI turn: %v\n%s\n%s", err, result.Process.Stdout, result.Process.Stderr)
	}
	// A fake provider's final reply is not execution evidence. Require the CLI's
	// completed command item to hold the probe's stdout and a successful exit.
	for _, line := range strings.Split(result.Process.Stdout, "\n") {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type     string `json:"type"`
				Output   string `json:"aggregated_output"`
				ExitCode *int   `json:"exit_code"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "item.completed" && event.Item.Type == "command_execution" &&
			event.Item.ExitCode != nil && *event.Item.ExitCode == 0 && strings.TrimSpace(event.Item.Output) == nativeProbeReply {
			return result
		}
	}
	t.Fatalf("the CLI did not report a successful confinement command:\n%s\n%s", result.Process.Stdout, result.Process.Stderr)
	return result
}

// Only transport, optional integrations, and temporary-fixture handling change
// here. The adapter's cwd, sandbox, approvals, and writable roots stay intact.
type sandboxCLIRunner struct{ home, url string }

func (r sandboxCLIRunner) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	prefix := append([]string{"exec"}, readOnlyArgs(command.Dir)...)
	prefix = append(prefix, "--disable", "code_mode", "--disable", "unified_exec",
		"--config", `model_provider="sandbox_probe"`,
		"--config", fmt.Sprintf(`model_providers.sandbox_probe={name="sandbox probe",base_url=%q,wire_api="responses",requires_openai_auth=false,supports_websockets=false,request_max_retries=0,stream_max_retries=0}`, r.url),
		// Temporary fixture siblings would otherwise be writable by default.
		"--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"--config", "sandbox_workspace_write.exclude_slash_tmp=true")
	command.Args = append(prefix, command.Args[1:]...)
	command.Env = append(execution.ExplicitEnvironment(command.Env), ProviderHomeVariable+"="+r.home)
	return (execution.OSProcessRunner{}).Run(ctx, command, observer)
}

type sandboxResponses struct {
	mu      sync.Mutex
	command []string
	calls   int
	err     error
}

func (s *sandboxResponses) begin(command []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.command, s.calls, s.err = command, 0, nil
}

func (s *sandboxResponses) requireTurn(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil || s.calls != 2 {
		t.Fatalf("scripted Responses turn: requests = %d, error = %v", s.calls, s.err)
	}
}

func (s *sandboxResponses) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var request struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&request); err != nil {
		s.err = err
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.calls++
	var item map[string]any
	if s.calls == 1 {
		name := ""
		var arguments any
		for _, tool := range request.Tools {
			switch tool.Name {
			case "shell":
				name, arguments = tool.Name, map[string]any{"command": s.command, "timeout_ms": 45000}
			case "shell_command":
				words := make([]string, len(s.command))
				for i, word := range s.command {
					words[i] = "'" + strings.ReplaceAll(word, "'", "'\"'\"'") + "'"
				}
				name, arguments = tool.Name, map[string]any{"command": strings.Join(words, " "), "timeout_ms": 45000}
			}
		}
		if name == "" {
			s.err = fmt.Errorf("the CLI advertised no supported foreground shell tool: %v", request.Tools)
			http.Error(w, s.err.Error(), http.StatusBadRequest)
			return
		}
		encoded, _ := json.Marshal(arguments)
		item = map[string]any{"type": "function_call", "id": "fc_probe", "call_id": "call_probe", "name": name, "arguments": string(encoded)}
	} else if s.calls == 2 {
		item = map[string]any{"type": "message", "id": "msg_probe", "role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": "probe complete", "annotations": []any{}}}}
	} else {
		s.err = fmt.Errorf("unexpected third request in a bounded probe turn")
		http.Error(w, s.err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range []any{
		map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_probe"}},
		map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item},
		map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item},
		map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_probe", "output": []any{item},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
	} {
		encoded, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", encoded)
	}
}
