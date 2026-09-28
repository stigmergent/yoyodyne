package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
)

// A developer session that writes its final reply and stays alive on work it
// backgrounded is a turn that ended, not a stall. While the harness waits that
// work out the run's record says so; once it is ended at its bound the run goes
// on to its checks and review exactly as a session that exited with its reply
// would, with no stall recorded, nothing left to continue, and the record saying
// the background work was ended.
func TestAFinishedTurnKeptAliveByBackgroundWorkIsNeverRecordedAsAStall(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	served := provider.Respond
	repliedAt := time.Date(2026, 9, 28, 18, 58, 33, 0, time.UTC)
	var saidWhileWaiting string
	provider.Respond = func(request backend.RunRequest) (backend.RunResult, error) {
		result, err := served(request)
		if request.Role != domain.RoleDeveloper || err != nil {
			return result, err
		}
		// The reply is written; the session is still running make test and make
		// race in the background, and the runner says so.
		account := execution.AfterReply{RepliedAt: repliedAt, BoundSeconds: 300}
		if request.AfterReplyWaiting == nil {
			t.Fatal("the developer invocation was given nowhere to say it is waiting on background work")
		}
		request.AfterReplyWaiting(account)
		waiting, loadErr := store.Load(request.RunID)
		if loadErr != nil {
			t.Fatalf("Load() while waiting error = %v", loadErr)
		}
		if waiting.AfterReply == nil || !waiting.AfterReply.Waiting() {
			t.Fatalf("the run's record while waiting = %+v, want the wait recorded", waiting.AfterReply)
		}
		saidWhileWaiting = waiting.AfterReply.Describe(repliedAt.Add(2 * time.Minute))
		account.Outcome = execution.AfterReplyEnded
		account.WaitedSeconds = 300
		result.Process = execution.ProcessResult{Status: execution.ProcessSucceeded, ExitCode: -1, AfterReply: &account}
		return result, nil
	}
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if saidWhileWaiting != "reply written, waiting for background processes: 2m of 5m" {
		t.Fatalf("the record said %q while the background work was waited out", saidWhileWaiting)
	}
	if outcome.Paused || outcome.ProviderStop != "" || outcome.Integration == nil || !tracker.Closed {
		t.Fatalf("outcome = %#v, want a run that went on to land rather than one stopped as a stall", outcome)
	}
	finished, err := store.Load(outcome.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if finished.ProviderStop != "" {
		t.Fatalf("a finished turn was recorded as a provider stop %q", finished.ProviderStop)
	}
	if finished.AfterReply == nil || finished.AfterReply.Outcome != execution.AfterReplyEnded {
		t.Fatalf("the run's record = %+v, want it to say the background work was ended at its bound", finished.AfterReply)
	}
	if strings.Contains(tracker.Notes, "stalled") {
		t.Fatalf("the item was told the run stalled:\n%s", tracker.Notes)
	}
}
