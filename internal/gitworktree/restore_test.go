package gitworktree

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

type restoredZeroBytes struct{}

func (restoredZeroBytes) Read(data []byte) (int, error) {
	clear(data)
	return len(data), nil
}

func TestRestoreCheckoutStreamsALargeBlobWithoutAnAggregateDataBound(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := repowrite.OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	const size = 65 << 20
	hash := sha1.New()
	fmt.Fprintf(hash, "blob %d%c", size, 0)
	if _, err := io.CopyN(hash, restoredZeroBytes{}, size); err != nil {
		t.Fatal(err)
	}
	object := hex.EncodeToString(hash.Sum(nil))
	input := io.MultiReader(strings.NewReader(fmt.Sprintf("%s blob %d\n", object, size)), io.LimitReader(restoredZeroBytes{}, size), strings.NewReader("\n"))
	entries := []restoredEntry{{name: "large", object: object, mode: 0o100644}}
	if err := writeRestoredBlobs(root, entries, bufio.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(path, "large")); err != nil || info.Size() != size || entries[0].size != size {
		t.Fatalf("restored large file = %v, %v", info, err)
	}
}

type replacingRestorationRoot struct {
	delegate execution.ProcessRunner
	before   func()
}

func (r *replacingRestorationRoot) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	if command.RawStdout != nil && r.before != nil {
		before := r.before
		r.before = nil
		before()
	}
	return r.delegate.Run(ctx, command, observer)
}

func TestRestoreCheckoutCannotWriteOutsideARootReplacedBeforeGitExecution(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-replaced-root")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeFile(t, outside, "sentinel", "keep this\n")
	moved := m.worktreeRoot + "-moved"
	replaced := false
	m.runner = &replacingRestorationRoot{delegate: m.runner, before: func() {
		// This is the former gap between path validation and Git execution.
		if err := os.Rename(m.worktreeRoot, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, m.worktreeRoot); err != nil {
			t.Fatal(err)
		}
		replaced = true
	}}
	if _, err := m.RestoreWorktree(context.Background(), w); err == nil {
		t.Fatal("reported recovery through a replaced root")
	}
	if !replaced {
		t.Fatal("restoration did not reach the replacement between validation and Git execution")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("restoration wrote outside its root: %v, %v", entries, err)
	}
	if content := readFile(t, outside, "sentinel"); content != "keep this\n" {
		t.Fatalf("sentinel = %q", content)
	}
	if head, err := m.resolveBranchCommit(context.Background(), w.Branch); err != nil || head != w.HarnessCommit {
		t.Fatalf("preserved branch = %q, %v", head, err)
	}
}

func TestRestoreCheckoutPreservesBinaryExecutableAndSymbolicLinkObjects(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-objects")
	content := []byte("\x00\xff\r\nno final newline")
	if err := os.WriteFile(filepath.Join(w.Path, "executable"), content, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("executable", filepath.Join(w.Path, "link")); err != nil {
		t.Fatal(err)
	}
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, w.Path, "executable"); got != string(content) {
		t.Fatalf("restored bytes = %q", got)
	}
	info, err := os.Stat(filepath.Join(w.Path, "executable"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("executable mode = %v, %v", info, err)
	}
	if target, err := os.Readlink(filepath.Join(w.Path, "link")); err != nil || target != "executable" {
		t.Fatalf("link = %q, %v", target, err)
	}
}

func TestRestoreCheckoutRefreshesExportsWithAConfinedIndex(t *testing.T) {
	t.Parallel()
	repository := newExportRepository(t)
	m := newExportManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-restored-export")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repository, exportPath, currentExport)
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, w.Path, exportPath); content != currentExport {
		t.Fatalf("restored export = %q", content)
	}
	if status := gitOutput(t, w.Path, "status", "--porcelain"); status != "" {
		t.Fatalf("export entered the restored change: %s", status)
	}
	if flags := gitOutput(t, w.Path, "ls-files", "-v", "--", exportPath); !strings.HasPrefix(flags, "S ") {
		t.Fatalf("restored export flags = %q", flags)
	}
}

func TestRestoreCheckoutRefusesCheckoutFiltersBeforeWriting(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-filtered")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "config", "filter.example.smudge", "cat")
	if _, err := m.RestoreWorktree(context.Background(), w); err == nil || !strings.Contains(err.Error(), "checkout filters") {
		t.Fatalf("filtered restoration = %v", err)
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Fatalf("refusal wrote a checkout: %v", err)
	}
}

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
