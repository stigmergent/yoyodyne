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
	"slices"
	"strings"
	"testing"
	"time"

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

func TestRestoreCheckoutLeavesAnOutsideHardLinkedIndexUnchanged(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-hard-linked-index")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	registration := strings.TrimSpace(gitOutput(t, w.Path, "rev-parse", "--absolute-git-dir"))
	index := filepath.Join(registration, "index")
	outside := t.TempDir()
	writeFile(t, outside, "sentinel", "keep this\n")
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, index); err != nil {
		t.Fatal(err)
	}
	// Preserve the registration, including the hard-linked index, while the
	// recorded checkout disappears.
	if err := os.RemoveAll(w.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, outside, "sentinel"); content != "keep this\n" {
		t.Fatalf("restoration changed the outside index sentinel: %q", content)
	}
	original, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Stat(index)
	if err != nil || os.SameFile(original, replacement) {
		t.Fatalf("restored index still uses the outside inode: %v", err)
	}
	if status := gitOutput(t, w.Path, "status", "--porcelain"); status != "" {
		t.Fatalf("restored index does not describe a clean checkout: %s", status)
	}
}

type afterRestorationBlobsRunner struct {
	delegate execution.ProcessRunner
	after    func(context.Context) error
}

func (r *afterRestorationBlobsRunner) Run(ctx context.Context, command execution.Command, observer execution.OutputObserver) (execution.ProcessResult, error) {
	result, err := r.delegate.Run(ctx, command, observer)
	if err == nil && result.Status == execution.ProcessSucceeded && command.RawStdout != nil && slices.Contains(command.Args, "cat-file") && r.after != nil {
		after := r.after
		r.after = nil
		err = after(ctx)
	}
	return result, err
}

func TestRestoreCheckoutLeavesAnOutsideHardLinkedExportUnchanged(t *testing.T) {
	t.Parallel()
	repository := newExportRepository(t)
	m := newExportManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-hard-linked-export")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repository, exportPath, currentExport)
	outside := t.TempDir()
	writeFile(t, outside, "sentinel", "keep this\n")
	export := filepath.Join(w.Path, exportPath)
	replaced := false
	m.runner = &afterRestorationBlobsRunner{delegate: m.runner, after: func(ctx context.Context) error {
		// The blob reader runs beside the process runner. Wait for its complete
		// export bytes before replacing the name, and before refresh can start.
		waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			content, err := os.ReadFile(export)
			if err == nil && string(content) == committedExport {
				break
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			select {
			case <-waitCtx.Done():
				return fmt.Errorf("wait for the committed export before linking it: %w", waitCtx.Err())
			case <-ticker.C:
			}
		}
		if err := os.Remove(export); err != nil {
			return err
		}
		if err := os.Link(filepath.Join(outside, "sentinel"), export); err != nil {
			return err
		}
		replaced = true
		return nil
	}}
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("restoration did not encounter the hard-linked export")
	}
	if content := readFile(t, outside, "sentinel"); content != "keep this\n" {
		t.Fatalf("restoration changed the outside export sentinel: %q", content)
	}
	if content := readFile(t, w.Path, exportPath); content != currentExport {
		t.Fatalf("restored export = %q, want the current export", content)
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
	writeFile(t, w.Path, ".gitattributes", "feature.txt filter=example\n")
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

func TestRestoreCheckoutAllowsUnusedConfiguredFilters(t *testing.T) {
	t.Parallel()
	repository := newRepository(t)
	m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
	w := preservedWorktree(t, m, "yoyodyne-unused-filter")
	writeFile(t, w.Path, "feature.txt", "committed work\n")
	w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
	if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
		t.Fatal(err)
	}
	// Runner installations may configure LFS even for repositories using none
	// of it. These commands would fail if recovery tried to run a filter.
	for _, key := range []string{"clean", "smudge", "process"} {
		runGit(t, repository, "config", "filter.lfs."+key, "false")
	}
	runGit(t, repository, "config", "filter.lfs.required", "true")
	if _, err := m.RestoreWorktree(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, w.Path, "feature.txt"); got != "committed work\n" {
		t.Fatalf("restored bytes = %q", got)
	}
	if err := m.VerifyOwnedHead(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreCheckoutReadsFilterAttributesAtTheRecordedRevision(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"recorded tree", "primary tree", "repository attributes"} {
		t.Run(name, func(t *testing.T) {
			repository := newRepository(t)
			m := newManager(t, repository, filepath.Join(t.TempDir(), "worktrees"))
			w := preservedWorktree(t, m, "yoyodyne-filter-source")
			writeFile(t, w.Path, "feature.txt", "committed work\n")
			if name == "recorded tree" {
				writeFile(t, w.Path, ".gitattributes", "feature.txt filter=example\n")
			}
			w.HarnessCommit = harnessCommit(t, w.Path, "yoyodyne: completed attempt")
			if _, err := m.RemovePreservedWorktree(context.Background(), w, KeepUncommittedWork); err != nil {
				t.Fatal(err)
			}
			if name == "primary tree" {
				writeFile(t, repository, ".gitattributes", "feature.txt filter=example\n")
				runGit(t, repository, "add", ".gitattributes")
				runGit(t, repository, "commit", "-m", "primary attributes differ")
			}
			if name == "repository attributes" {
				writeFile(t, repository, ".git/info/attributes", "feature.txt filter=example\n")
			}
			runGit(t, repository, "config", "filter.example.smudge", "false")
			runGit(t, repository, "config", "filter.example.required", "true")
			_, err := m.RestoreWorktree(context.Background(), w)
			if name == "primary tree" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "checkout filters") {
				t.Fatalf("filtered restoration = %v", err)
			}
			if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
				t.Fatalf("refusal wrote a checkout: %v", err)
			}
			if head, err := m.resolveBranchCommit(context.Background(), w.Branch); err != nil || head != w.HarnessCommit {
				t.Fatalf("preserved branch = %q, %v", head, err)
			}
		})
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
