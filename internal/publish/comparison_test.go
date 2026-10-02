package publish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// comparisonRunner routes only the comparison through the real gh CLI, to a
// local server. All other forge reads are the scripted answers.
type comparisonRunner struct {
	*scriptedRunner
	binary, url string
}

func (r comparisonRunner) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	if len(command.Args) > 3 && strings.Contains(command.Args[3], "/compare/") {
		command.Name = r.binary
		command.Args = append([]string(nil), command.Args...)
		command.Args[3] = r.url
		command.Env = []string{"GH_TOKEN=fixture-token", "GH_ENTERPRISE_TOKEN=fixture-token"}
		return (execution.OSProcessRunner{}).Run(ctx, command, observer)
	}
	return r.scriptedRunner.Run(ctx, command, observer)
}

func TestGitHubChecksReadsALargeComparisonBeforeRetainingOnlyItsDistance(t *testing.T) {
	t.Parallel()
	binary, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh is needed to replay a comparison through its JSON projection")
	}
	// Exercise both retention bounds with a single JSON line. This is a
	// generated response, not a capture from the project's forge.
	response, err := json.Marshal(map[string]any{
		"ahead_by": 31, "behind_by": 0, "status": "ahead",
		"files":                []any{map[string]any{"filename": "other.go", "patch": strings.Repeat("x", 9<<20)}},
		"unknown_future_field": map[string]any{"nested": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()
	for _, number := range []int{732, 700} {
		head := "1111111111111111111111111111111111111111"
		runner := &scriptedRunner{}
		runner.reply("remote get-url", execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: "https://example.invalid/acme/thing"})
		runner.reply("pr view", execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: `{"headRefOid":"` + head + `","files":[]}`})
		runner.reply("/check-runs", execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: `{"check_runs":[{"name":"build","status":"completed","conclusion":"success"}]}`})
		reading, err := (GitHub{Runner: comparisonRunner{runner, binary, server.URL}}).Checks(context.Background(), number, "main")
		if err != nil || reading.BehindBy != 31 || reading.Passing != 1 {
			t.Fatalf("request %d: Checks() = %#v, %v", number, reading, err)
		}
	}
}
