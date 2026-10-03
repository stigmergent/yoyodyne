package gitworktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// ContentIdentityPrefix opens every identity ContentIdentity names, so a reader
// can tell one from a commit: both are hexadecimal, and a commit is what every
// other identifier on a run's record is.
//
// Version 2 includes Git modes and symlink targets. Earlier identities cannot
// carry check credit onto a tree named by this version.
const ContentIdentityPrefix = "sha256-v2:"

// hashObjectBatch bounds how many paths one `git hash-object` is handed, so a
// change touching thousands of files does not build a command line the
// operating system refuses.
const hashObjectBatch = 128

// ContentIdentity names the content of a run's change as it stands in the
// worktree: a digest over the recorded base, every path the change touches
// against it, its Git mode, and the blob Git would store for each regular file
// or the target text of a symlink, or that it is gone. Two readings agree exactly
// when the tree holds the same change against the same base, and a mode change
// or a single byte moved in any file or link target the change touches — tracked
// or untracked — moves the identity. Committing the attempt does not move it:
// a file is named by its path, mode, and content, not by
// whether the index has heard of it, so a publishing run, which commits before
// its checks, and one that does not are bound by the same reading.
//
// It exists for evidence that has to name a revision on a project that has made
// no commit to name. A publishing run commits each attempt before its checks
// run, so a passing check phase is bound to that commit; a project that does not
// publish commits nothing until the promotion itself, and until this the only
// thing binding its checks to the change they passed over was the attempt
// count, which says when the checks ran and nothing about what they ran over.
//
// It only reads. `git hash-object` without `-w` computes the id and stores
// nothing, so the worktree, its index, and the object store are exactly as the
// developer left them. The one Git command that would answer this in a line —
// `write-tree` — needs an index to write from, and the run's index carries the
// held-out exports, so it is not asked.
//
// A symlink is read without following it: its own target text is hashed, never
// the file it points to, which may be outside the worktree. Other non-regular
// paths are still named by their type rather than opened.
func (m *Manager) ContentIdentity(ctx context.Context, worktree Worktree) (string, error) {
	path, _, err := m.verifyOwnedHead(ctx, worktree)
	if err != nil {
		return "", err
	}
	entries, err := m.changedEntries(ctx, path, worktree.BaseCommit)
	if err != nil {
		return "", err
	}
	untracked, err := m.untrackedFiles(ctx, path)
	if err != nil {
		return "", err
	}
	for _, relative := range untracked {
		entries = append(entries, contentEntry{path: relative})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	// Regular files are hashed by Git; links contribute their own text and
	// other types are named without opening them. Every path carries its mode.
	var toHash []int
	for i := range entries {
		if entries[i].status == "D" {
			entries[i].mode = "000000"
			entries[i].blob = "deleted"
			continue
		}
		info, err := os.Lstat(filepath.Join(path, filepath.FromSlash(entries[i].path)))
		switch {
		case err != nil:
			return "", fmt.Errorf("inspect %s for its content identity: %w", entries[i].path, err)
		case info.Mode().IsRegular():
			entries[i].mode = "100644"
			// Git records the owner's executable bit, not the other permission
			// bits the filesystem carries.
			if info.Mode()&0o100 != 0 {
				entries[i].mode = "100755"
			}
			toHash = append(toHash, i)
		case info.Mode()&os.ModeSymlink != 0:
			entries[i].mode = "120000"
			target, err := os.Readlink(filepath.Join(path, filepath.FromSlash(entries[i].path)))
			if err != nil {
				return "", fmt.Errorf("read link %s for its content identity: %w", entries[i].path, err)
			}
			sum := sha256.Sum256([]byte(target))
			entries[i].blob = "target-sha256:" + hex.EncodeToString(sum[:])
		default:
			// Git supplies the mode for tracked directories, including gitlinks.
			// Other directories and types have no tracked mode to carry over.
			if entries[i].mode == "" {
				entries[i].mode = "type:" + info.Mode().Type().String()
				if info.IsDir() {
					entries[i].mode = "040000"
				}
			}
			entries[i].blob = "type:" + info.Mode().Type().String()
		}
	}
	for start := 0; start < len(toHash); start += hashObjectBatch {
		end := min(start+hashObjectBatch, len(toHash))
		args := []string{"-C", path, "hash-object", "--"}
		for _, i := range toHash[start:end] {
			args = append(args, entries[i].path)
		}
		result, err := m.run(ctx, args...)
		if err != nil {
			return "", err
		}
		if result.Status != execution.ProcessSucceeded {
			return "", fmt.Errorf("hash changed worktree files failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		blobs := strings.Fields(result.Stdout)
		if len(blobs) != end-start {
			return "", fmt.Errorf("hash changed worktree files reported %d ids for %d paths", len(blobs), end-start)
		}
		for offset, i := range toHash[start:end] {
			entries[i].blob = blobs[offset]
		}
	}

	digest := sha256.New()
	fmt.Fprintf(digest, "base %s\n", worktree.BaseCommit)
	for _, entry := range entries {
		fmt.Fprintf(digest, "%s\x00%s\x00%s\x00", entry.path, entry.mode, entry.blob)
	}
	return ContentIdentityPrefix + hex.EncodeToString(digest.Sum(nil)), nil
}

// contentEntry is one path a change touches: what became of it against the
// base, in Git's one-letter status where the file is tracked, its Git mode (or
// type for other non-regular paths), and the digest of its content now.
type contentEntry struct {
	status string
	path   string
	mode   string
	blob   string
}

// changedEntries lists the tracked half of a change against the base, with
// rename detection off for the reason ChangedPaths turns it off: a rename is a
// deletion and an addition, and an identity has to name both sides.
func (m *Manager) changedEntries(ctx context.Context, path, baseCommit string) ([]contentEntry, error) {
	result, err := m.run(ctx, "-C", path, "diff", "--raw", "-z", "--no-renames", "--no-ext-diff", baseCommit, "--")
	if err != nil {
		return nil, err
	}
	if result.Status != execution.ProcessSucceeded {
		return nil, fmt.Errorf("list changed worktree paths failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	// Each entry is a metadata field (old/new modes, old/new blobs, status) and
	// then a path field, both NUL-terminated; with renames off there is never a
	// second path. The new mode names types such as gitlinks that Lstat alone
	// cannot distinguish from a directory.
	listing := strings.TrimSuffix(strings.TrimSuffix(result.Stdout, "\n"), "\x00")
	if listing == "" {
		return nil, nil
	}
	fields := strings.Split(listing, "\x00")
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("list changed worktree paths reported %q, which does not pair into statuses and paths", fields)
	}
	entries := make([]contentEntry, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		metadata := strings.Fields(fields[i])
		if len(metadata) != 5 || !strings.HasPrefix(metadata[0], ":") {
			return nil, fmt.Errorf("list changed worktree paths reported invalid metadata %q", fields[i])
		}
		entries = append(entries, contentEntry{status: metadata[4], path: fields[i+1], mode: metadata[1]})
	}
	return entries, nil
}
