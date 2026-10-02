package gitworktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreMissingCheckoutWithItsRegistrationStillStanding(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-missing")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if err := os.RemoveAll(w.Path); err != nil {
		t.Fatal(err)
	}
	restored, err := m.RestoreWorktree(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if restored != w {
		t.Fatalf("restored = %#v, want %#v", restored, w)
	}
	if err := m.VerifyOwnedHead(context.Background(), restored); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(w.Path, "feature.txt"))
	if err != nil || string(content) != "committed work\n" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

func TestRestoreCheckoutRecreatesAMissingWorktreeRoot(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	root := filepath.Join(t.TempDir(), "worktrees")
	m := newManager(t, repository, root)
	w := preservedWorktree(t, m, "yoyodyne-root-gone")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if err := m.VerifyOwnedHead(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreCheckoutRefusesConflictingAndUnverifiableArtifacts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"path occupied", "branch missing", "branch held elsewhere", "root symlink", "locked registration", "foreign commit"} {
		t.Run(name, func(t *testing.T) {
			repository := newRepository(t)
			worktreeRoot := filepath.Join(t.TempDir(), "worktrees")
			m := newManager(t, repository, worktreeRoot)
			w := preservedWorktree(t, m, "yoyodyne-refused")
			writeFile(t, w.Path, "feature.txt", "committed work\n")
			w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
			if name == "locked registration" {
				runGit(t, repository, "worktree", "lock", w.Path)
				if err := os.RemoveAll(w.Path); err != nil {
					t.Fatal(err)
				}
			} else {
				removed, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork)
				if err != nil || !removed.Removed {
					t.Fatalf("removal = %#v, %v", removed, err)
				}
			}
			sentinel := ""
			switch name {
			case "path occupied":
				writeFile(t, w.Path, "somebody.txt", "keep this\n")
				sentinel = filepath.Join(w.Path, "somebody.txt")
			case "branch missing":
				runGit(t, repository, "branch", "-D", w.Branch)
			case "branch held elsewhere":
				other := filepath.Join(t.TempDir(), "other")
				runGit(t, repository, "worktree", "add", other, w.Branch)
				writeFile(t, other, "somebody.txt", "keep this\n")
				sentinel = filepath.Join(other, "somebody.txt")
			case "root symlink":
				outside := t.TempDir()
				if err := os.Remove(worktreeRoot); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, worktreeRoot); err != nil {
					t.Fatal(err)
				}
				writeFile(t, outside, "somebody.txt", "keep this\n")
				sentinel = filepath.Join(outside, "somebody.txt")
			case "foreign commit":
				// The hash matches, but its history cannot pass harness ownership.
				w.BaseCommit = strings.Repeat("a", 40)
			}
			if _, err := m.RestoreWorktree(context.Background(), w); err == nil {
				t.Fatal("restored conflicting or unverifiable work")
			}
			if sentinel != "" {
				content, err := os.ReadFile(sentinel)
				if err != nil || string(content) != "keep this\n" {
					t.Fatalf("sentinel = %q, %v", content, err)
				}
			}
			if name != "branch missing" {
				tip, err := m.resolveBranchCommit(context.Background(), w.Branch)
				if err != nil || tip != w.HarnessCommit {
					t.Fatalf("surviving branch = %q, %v", tip, err)
				}
			}
		})
	}
}
