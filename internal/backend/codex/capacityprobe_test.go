package codex

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
	repository := t.TempDir()
	runner := &fakeRunner{results: []execution.ProcessResult{{Status: execution.ProcessSucceeded,
		Stdout: lines(`{"id":"0","msg":{"type":"task_complete","last_agent_message":"OK"}}`)}}}
	_, err := (Backend{Runner: runner}).Run(context.Background(), backendapi.RunRequest{
		RunID: testRunID, Role: domain.RoleDeveloper, CapacityProbe: true, WorkingDirectory: repository,
		Prompt: "do developer work", SystemPrompt: "unbounded instructions", Timeout: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := runner.commands[0]
	if sandboxArgument(t, command.Args) != sandboxReadOnly || command.Dir == repository {
		t.Fatalf("probe gained developer sandbox or repository context: %+v", command)
	}
	joined := strings.Join(command.Args, " ")
	for _, required := range []string{"--ignore-user-config", `approval_policy="never"`, `web_search="disabled"`, "agents.enabled=false", "orchestrator.mcp.enabled=false"} {
		if !strings.Contains(joined, required) {
			t.Errorf("probe args missing %q: %v", required, command.Args)
		}
	}
	if strings.Contains(joined, "resume") || runner.prompts[0] != backendapi.CapacityProbePrompt || command.Timeout != backendapi.CapacityProbeTimeout || command.IdleTimeout != backendapi.CapacityProbeTimeout {
		t.Fatalf("probe acquired a session, task, or longer bound: %v, %q", command.Args, runner.prompts[0])
	}
	for _, request := range []backendapi.RunRequest{
		{CapacityProbe: true, SessionID: "developer-session"},
		{CapacityProbe: true, AllowedTools: []string{"Read"}},
	} {
		request.RunID, request.Role, request.WorkingDirectory, request.Prompt = testRunID, domain.RoleDeveloper, repository, backendapi.CapacityProbePrompt
		if _, err := (Backend{Runner: runner}).Run(context.Background(), request); err == nil {
			t.Fatal("probe accepted a session or tool grant")
		}
	}
	if len(runner.commands) != 1 {
		t.Fatal("refused probe started a process")
	}
}
