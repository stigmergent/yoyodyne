package claudecode

import (
	"context"
	"strings"
	"testing"
	"time"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

func TestDeveloperCapacityProbeCannotUseDeveloperAccess(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{results: []execution.ProcessResult{{Status: execution.ProcessSucceeded,
		Stdout: `{"type":"result","subtype":"success","is_error":false,"result":"OK"}` + "\n"}}}
	_, err := (Backend{Runner: runner}).Run(context.Background(), backendapi.RunRequest{
		RunID: testRunID, Role: domain.RoleDeveloper, CapacityProbe: true, WorkingDirectory: "/worktree",
		Prompt: "do developer work", SystemPrompt: "unbounded instructions", Timeout: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := runner.commands[0]
	mode, named := sessionModeArgument(command.Args)
	if !named || mode != readOnlySessionMode {
		t.Fatalf("probe permission mode = %q, %t", mode, named)
	}
	joined := strings.Join(command.Args, " ")
	for _, forbidden := range []string{"--settings", "--allowedTools", "--append-system-prompt", "--resume"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("probe gained %s: %v", forbidden, command.Args)
		}
	}
	if !strings.Contains(joined, "--safe-mode") || !strings.Contains(joined, "--tools ") {
		t.Fatalf("probe lacks safe mode or disabled tools: %v", command.Args)
	}
	if runner.prompts[0] != backendapi.CapacityProbePrompt || command.Timeout != backendapi.CapacityProbeTimeout || command.IdleTimeout != backendapi.CapacityProbeTimeout {
		t.Fatalf("probe prompt or bounds changed: %q, %v, %v", runner.prompts[0], command.Timeout, command.IdleTimeout)
	}
	for _, request := range []backendapi.RunRequest{
		{CapacityProbe: true, SessionID: "developer-session"},
		{CapacityProbe: true, AllowedTools: []string{"Read"}},
	} {
		request.RunID, request.Role, request.WorkingDirectory, request.Prompt = testRunID, domain.RoleDeveloper, "/worktree", backendapi.CapacityProbePrompt
		if _, err := (Backend{Runner: runner}).Run(context.Background(), request); err == nil {
			t.Fatal("probe accepted a session or tool grant")
		}
	}
	if len(runner.commands) != 1 {
		t.Fatal("refused probe started a process")
	}
}
