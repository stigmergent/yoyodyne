package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A mode change or a link retarget after checks passed earns no promotion.
// Only running the checks again binds their credit to the changed tree.
func TestChangedModesAndLinkTargetsRequireFreshChecks(t *testing.T) {
	t.Parallel()
	for _, symlink := range []bool{false, true} {
		name := "executable bit"
		if symlink {
			name = "symlink target"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repository, tracker, _, pipeline, store := automaticFixture(t)
			if err := os.WriteFile(filepath.Join(repository, ".gitignore"), []byte("checks.log\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink("missing-original", filepath.Join(repository, "changed")); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(filepath.Join(repository, "changed"), []byte("original\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runPipelineGit(t, repository, "add", ".")
			runPipelineGit(t, repository, "commit", "-m", "track the original path")
			worktree, err := pipeline.Worktrees.Create(context.Background(), gitworktree.CreateRequest{
				RunID: pipelineRunID, WorkItemID: tracker.Item.ID, BaseRef: "HEAD",
			})
			if err != nil {
				t.Fatal(err)
			}
			changed := filepath.Join(worktree.Path, "changed")
			retarget := func(target string) {
				t.Helper()
				if err := os.Remove(changed); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, changed); err != nil {
					t.Fatal(err)
				}
			}
			if symlink {
				retarget("missing-modified")
			} else if err := os.WriteFile(changed, []byte("modified\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			pipeline.Config.Checks = []string{"printf 'checked\\n' >> checks.log"}
			state := gatedRun(t, &runstate.ChecksPassed{})
			state.RunID, state.WorkItemID = worktree.RunID, tracker.Item.ID
			state.WorktreePath, state.Branch = worktree.Path, worktree.Branch
			state.BaseCommit, state.TargetBranch = worktree.BaseCommit, worktree.TargetBranch
			state.HarnessCommit, state.ChecksPassed = "", nil
			state.Verification = &runstate.Verification{
				Probe:  &runstate.VerificationExecution{Command: "exit 0", Outcome: "passed"},
				Checks: []runstate.VerificationExecution{{Command: "exit 0", Outcome: "passed"}},
			}
			if err := store.Create(state); err != nil {
				t.Fatal(err)
			}
			run := &activeRun{pipeline: pipeline, item: tracker.Item, state: state, worktree: worktree}
			if err := run.verify(context.Background()); err != nil {
				t.Fatalf("verify() error = %v", err)
			}
			before := run.state.ChecksPassed.Content
			if err := run.integrationEarned(context.Background()); err != nil {
				t.Fatalf("unchanged tree lost its check credit: %v", err)
			}
			if symlink {
				retarget("missing-retargeted")
			} else if err := os.Chmod(changed, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := run.integrationEarned(context.Background()); !errors.Is(err, ErrIntegrationUnearned) || !strings.Contains(err.Error(), "passed over content") {
				t.Fatalf("integrationEarned() error = %v, want the changed tree to lose its check credit", err)
			}
			if err := run.verify(context.Background()); err != nil {
				t.Fatalf("verify() after the change error = %v", err)
			}
			if after := run.state.ChecksPassed.Content; after == before {
				t.Fatal("fresh checks retained the old content identity")
			}
			if err := run.integrationEarned(context.Background()); err != nil {
				t.Fatalf("fresh checks did not restore credit: %v", err)
			}
			run.state.ChecksPassed.Content = "sha256:" + strings.TrimPrefix(run.state.ChecksPassed.Content, gitworktree.ContentIdentityPrefix)
			if err := run.integrationEarned(context.Background()); !errors.Is(err, ErrIntegrationUnearned) {
				t.Fatalf("integrationEarned() error = %v, want earlier identity versions refused", err)
			}
			log, err := os.ReadFile(filepath.Join(worktree.Path, "checks.log"))
			if err != nil || string(log) != "checked\nchecked\n" {
				t.Fatalf("check log = %q, %v; want two actual check executions", log, err)
			}
		})
	}
}
