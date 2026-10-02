package gitworktree

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

func TestFilesAtCommitListsUnchangedBinaryFilesAndBoundsTheListing(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	writeFile(t, repository, "testdata/fixture.bin", "\x00binary\n")
	writeFile(t, repository, "docs/quoted name.txt", "unchanged\n")
	writeFile(t, repository, "docs/line\r\nbreak.txt", "unchanged\n")
	runGit(t, repository, "add", ".")
	runGit(t, repository, "commit", "-m", "fixtures already present")
	commit := gitLine(t, repository, "rev-parse", "HEAD")
	manager := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	whole, err := manager.FilesAtCommit(context.Background(), commit, 20000, 128<<10)
	if err != nil || whole.Omitted != 0 || !slices.Contains(whole.Files, "testdata/fixture.bin") || !slices.Contains(whole.Files, "docs/quoted name.txt") || !slices.Contains(whole.Files, "docs/line\r\nbreak.txt") {
		t.Fatalf("FilesAtCommit() = %#v, %v", whole, err)
	}
	for _, bounds := range [][2]int{{1, 128 << 10}, {20000, 30}} {
		bounded, err := manager.FilesAtCommit(context.Background(), commit, bounds[0], bounds[1])
		if err != nil || bounded.Omitted == 0 || bounded.Omitted+len(bounded.Files) != len(whole.Files) {
			t.Fatalf("bounded FilesAtCommit() = %#v, %v", bounded, err)
		}
	}
}

type shortenedListingRunner struct {
	execution.ProcessRunner
	result execution.ProcessResult
}

func (r shortenedListingRunner) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	if slices.Contains(command.Args, "ls-tree") {
		return r.result, nil
	}
	return r.ProcessRunner.Run(ctx, command, observer)
}

func TestRepositoryListingNeverTreatsShortenedCommandOutputAsComplete(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	manager := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	commit := gitLine(t, repository, "rev-parse", "HEAD")
	for _, result := range []execution.ProcessResult{
		{Status: execution.ProcessSucceeded, Stdout: "README.txt\n", OutputTruncation: "output was cut"},
		{Status: execution.ProcessSucceeded, Stdout: "README.txt…[line truncated at 10 bytes; 20 further bytes were not retained]\n"},
		{Status: execution.ProcessSucceeded, Stdout: "\"unfinished quoted path\n"},
	} {
		manager.runner = shortenedListingRunner{ProcessRunner: execution.OSProcessRunner{}, result: result}
		if listing, err := manager.FilesAtCommit(context.Background(), commit, 20000, 128<<10); err == nil || listing.Commit != "" {
			t.Fatalf("shortened output was a complete listing: %#v, %v", listing, err)
		}
	}
}

// Replays the integrated-before-review incident: comparison with main is now
// empty, but the run's recorded pre-integration base still shows the work.
func TestReviewDiffKeepsTheRecordedBaseAfterTheTargetContainsTheChange(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	manager := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	worktree, err := manager.Create(context.Background(), CreateRequest{RunID: testRunID, WorkItemID: "yoyodyne-already-integrated", BaseRef: "main"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, worktree.Path, "feature.txt", "the work to review\n")
	worktree.HarnessCommit = harnessCommit(t, worktree.Path, "the candidate")
	runGit(t, repository, "merge", "--ff-only", worktree.Branch)
	if patch := gitLine(t, repository, "diff", "main", worktree.Branch); patch != "" {
		t.Fatalf("the incident's branch-versus-target diff is not empty: %s", patch)
	}
	changes, err := manager.UnifiedChanges(context.Background(), worktree, DiffLimits{})
	if err != nil || !strings.Contains(changes.Patch, "+the work to review") || changes.BaseCommit != worktree.BaseCommit || changes.HeadCommit != worktree.HarnessCommit {
		t.Fatalf("UnifiedChanges() = %#v, %v", changes, err)
	}
	// Branch reviews explicitly naming the recorded base can replay it too;
	// a command naming main must not silently substitute a different base.
	branch, err := manager.BranchChanges(context.Background(), BranchRequest{Branch: worktree.Branch, BaseRef: worktree.BaseCommit}, DiffLimits{})
	if err != nil || !strings.Contains(branch.Changes.Patch, "+the work to review") {
		t.Fatalf("BranchChanges() = %#v, %v", branch, err)
	}
}
