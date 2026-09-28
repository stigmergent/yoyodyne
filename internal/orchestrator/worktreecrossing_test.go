package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/notify"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A run's worktree creation that crosses a neighbour's cleanup is run again
// rather than failing the run, and the watch log says so. yoyodyne-ifd.428.16
// reported the concurrent-runs test failing on exactly that clash, with Git's
// "Invalid path '.git/worktrees/<other>'", and in production the same clash
// would have cost a real run its worktree. The crossing is absorbed on the
// second attempt, which is why the log has to name it: a re-run that leaves no
// trace makes a repository whose runs cross more and more look like one where
// they never do.
func TestADispatchWhoseWorktreeAddCrossesARemovalGetsItsRunAndTheWatchLogNamesIt(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	watch, err := runstate.NewWatchStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewWatchStore() error = %v", err)
	}
	log := &watchLog{store: watch, session: "watch-0123456789abcdef0123456789abcdef"}
	session := &watchSession{to: log, now: func() time.Time { return time.Now().UTC() }, schedule: &Schedule{}}
	session.enter(runstate.WatchIdle, account{reason: "nothing further pullable this poll", running: 1})

	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-task", Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)
	// The add is refused once in Git's own words for an entry a removal took
	// from under it, and everything else — the add run again included — is Git.
	crossing := &addCrossingRunner{
		delegate: execution.OSProcessRunner{},
		stderr:   "fatal: Invalid path '.git/worktrees/yoyodyne-other-2113a23c': No such file or directory\n",
	}
	worktrees, err := gitworktree.New(gitworktree.Options{
		Runner:                crossing,
		RepositoryRoot:        repository,
		WorktreeRoot:          worktreeRoot,
		AllowedPrimaryChanges: []string{".beads/interactions.jsonl", ".beads/issues.jsonl"},
		CurrentExports:        []string{".beads/issues.jsonl"},
		Timeout:               testGitBudget,
	})
	if err != nil {
		t.Fatalf("gitworktree.New() error = %v", err)
	}
	pipeline.Worktrees = worktrees

	outcome, err := pipeline.Run(session.dispatching(context.Background(), tracker.Item.ID), tracker.Item.ID)
	if err != nil {
		t.Fatalf("Run() error = %v, want the add that crossed a removal to have been run again", err)
	}
	if outcome.Status != runstate.StatusSucceeded {
		t.Fatalf("outcome = %#v, want the run to have got its worktree and finished", outcome)
	}
	if adds := crossing.observed(); adds < 2 {
		t.Fatalf("adds = %d, want the refused add followed by another", adds)
	}

	sessions, err := watch.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var noted []runstate.WatchTransition
	for _, transition := range sessions {
		if transition.WorktreeCrossing != nil {
			noted = append(noted, transition)
		}
	}
	if len(noted) != 1 {
		t.Fatalf("watch log = %#v, want exactly one entry naming the crossing", sessions)
	}
	note := noted[0]
	if got := *note.WorktreeCrossing; got.WorkItemID != "yoyodyne-task" || got.Command != "git worktree add" || got.Attempt != 1 || got.Attempts < 2 ||
		!strings.Contains(got.Refusal, "Invalid path '.git/worktrees/yoyodyne-other-2113a23c'") {
		t.Fatalf("recorded crossing = %#v, want the item, the add, attempt 1, and Git's own words", got)
	}
	if !strings.Contains(note.Reason, "yoyodyne-task") || !strings.Contains(note.Reason, "`git worktree add`") {
		t.Fatalf("reason = %q, want it to name the item and the command run again", note.Reason)
	}
	// It is a note about a dispatch, not where the session got to, and it is
	// said on the log rather than to the channel.
	if latest, _, _ := watch.Latest(); latest.WorktreeCrossing != nil || latest.State != runstate.WatchIdle {
		t.Fatalf("Latest() = %#v, want the idle poll rather than the note", latest)
	}
	if notification, err := notify.FromWatch(note); err != nil || notification.Posts() {
		t.Fatalf("FromWatch() = %#v, %v, want a note that stays on the record", notification, err)
	}
}

// addCrossingRunner refuses the first `git worktree add` it is handed with
// stderr, the way Git refuses an add that crossed another worktree's removal,
// and runs everything else for real.
type addCrossingRunner struct {
	delegate execution.ProcessRunner
	stderr   string
	mu       sync.Mutex
	adds     int
}

func (r *addCrossingRunner) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	isAdd := false
	for index := 0; index+1 < len(command.Args); index++ {
		if command.Args[index] == "worktree" && command.Args[index+1] == "add" {
			isAdd = true
		}
	}
	if !isAdd {
		return r.delegate.Run(ctx, command, observer)
	}
	r.mu.Lock()
	r.adds++
	first := r.adds == 1
	r.mu.Unlock()
	if first {
		return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 128, Stderr: r.stderr}, nil
	}
	return r.delegate.Run(ctx, command, observer)
}

func (r *addCrossingRunner) observed() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adds
}
