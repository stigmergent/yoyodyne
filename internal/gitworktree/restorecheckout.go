package gitworktree

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/repowrite"
)

type restoredEntry struct {
	name, object string
	mode         uint32
	size         uint32
	export       bool
}

// restoreCheckout never asks Git to write through a checkout pathname. Git
// supplies immutable objects; the shared writer writes both the checkout and
// its linked-worktree registration through pinned directory handles. A replaced
// root may make recovery fail, but cannot redirect these writes elsewhere.
func (m *Manager) restoreCheckout(ctx context.Context, worktree Worktree, root *repowrite.PinnedRoot, registered bool) error {
	common, err := m.commonGitDirectory(ctx)
	if err != nil {
		return err
	}
	metadata, err := repowrite.OpenPinnedRoot(common)
	if err != nil {
		return err
	}
	defer metadata.Close()
	listing, err := m.listWorktrees(ctx)
	if err != nil {
		return err
	}
	for _, entry := range listing {
		if entry.branch == worktree.Branch && entry.path != worktree.Path {
			return fmt.Errorf("branch %s is already checked out at %s", worktree.Branch, entry.path)
		}
	}
	// A filter can require bytes absent from the committed object (for example
	// LFS). Never claim the raw object is a fully recovered filtered checkout.
	filters, err := m.run(ctx, "-C", m.repositoryRoot, "config", "--get-regexp", `^filter\.`)
	if err != nil {
		return err
	}
	if filters.Status == execution.ProcessSucceeded {
		return errors.New("safe checkout restoration requires a repository without configured checkout filters")
	}
	if filters.Status != execution.ProcessFailed || filters.ExitCode != 1 {
		return fmt.Errorf("inspect checkout filters ended as %s with exit code %d: %s", filters.Status, filters.ExitCode, strings.TrimSpace(filters.Stderr))
	}
	entries, err := m.restoreEntries(ctx, worktree.HarnessCommit)
	if err != nil {
		return err
	}
	registration, err := m.restoreRegistration(metadata, worktree, registered)
	if err != nil {
		return err
	}
	defer registration.Close()
	checkout, err := root.CreateDirectory(filepath.Base(worktree.Path))
	if err != nil {
		return err
	}
	defer checkout.Close()
	if err := m.restoreBlobs(ctx, checkout, entries); err != nil {
		return err
	}
	// Copy tracked current exports through the same handle, and encode their
	// skip-worktree bits in the index rather than invoking an index writer at an
	// absolute .git pathname. Missing exports keep the committed bytes.
	var ignoredExports []struct {
		name    string
		content []byte
	}
	for _, export := range m.currentExports {
		content, present, err := readPrimaryExport(m.repositoryRoot, export)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		tracked := false
		for i := range entries {
			if entries[i].name != export {
				continue
			}
			if entries[i].mode != 0o100644 && entries[i].mode != 0o100755 {
				return fmt.Errorf("current export %s is not a regular file", export)
			}
			if err := checkout.WriteFile(export, content, 0o644, false); err != nil {
				return err
			}
			entries[i].export = true
			tracked = true
		}
		if !tracked {
			ignoredExports = append(ignoredExports, struct {
				name    string
				content []byte
			}{export, content})
		}
	}
	if err := registration.WriteFile("index", restoredIndex(entries), 0o600, false); err != nil {
		return err
	}
	marker := "gitdir: " + registration.Path() + "\n"
	// The path is only written as Git's link text, never used as a write target.
	if err := checkout.WriteFile(".git", []byte(marker), 0o644, true); err != nil {
		return err
	}
	if err := registration.Remove("locked"); err != nil {
		return err
	}
	heldRegistry(ctx).forgetListing()
	if err := root.Unchanged(); err != nil {
		return err
	}
	if err := checkout.Unchanged(); err != nil {
		return err
	}
	if err := metadata.Unchanged(); err != nil {
		return err
	}
	for _, export := range ignoredExports {
		ignored, err := m.ignoresPath(ctx, worktree.Path, export.name)
		if err != nil {
			return err
		}
		if ignored {
			if err := checkout.WriteFile(export.name, export.content, 0o644, true); err != nil {
				return err
			}
		}
	}
	return root.Unchanged()
}

func (m *Manager) restoreRegistration(metadata *repowrite.PinnedRoot, worktree Worktree, registered bool) (*repowrite.PinnedRoot, error) {
	if err := metadata.MakeDirectory("worktrees", 0o700); err != nil {
		return nil, err
	}
	entries, err := metadata.ReadDirectory("worktrees")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := filepath.Join("worktrees", entry.Name())
		gitdir, err := metadata.ReadFile(filepath.Join(name, "gitdir"))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(gitdir)) != filepath.Join(worktree.Path, ".git") {
			continue
		}
		root, err := metadata.OpenDirectory(name)
		if err != nil {
			return nil, err
		}
		for _, file := range []string{"locked", "config.worktree"} {
			present, err := root.Exists(file)
			if err != nil || present {
				root.Close()
				return nil, fmt.Errorf("missing checkout's registration cannot be restored with %s present: %v", file, err)
			}
		}
		head, headErr := root.ReadFile("HEAD")
		common, commonErr := root.ReadFile("commondir")
		commonPath := strings.TrimSpace(string(common))
		if !filepath.IsAbs(commonPath) {
			commonPath = filepath.Join(root.Path(), commonPath)
		}
		if headErr != nil || commonErr != nil || strings.TrimSpace(string(head)) != "ref: refs/heads/"+worktree.Branch || filepath.Clean(commonPath) != metadata.Path() {
			root.Close()
			return nil, errors.New("missing checkout's registration no longer names its recorded branch and common directory")
		}
		if err := root.WriteFile("locked", []byte("initializing\n"), 0o644, true); err != nil {
			root.Close()
			return nil, err
		}
		return root, nil
	}
	if registered {
		return nil, errors.New("the missing checkout's registration could not be verified")
	}
	for suffix := 0; ; suffix++ {
		name := filepath.Base(worktree.Path)
		if suffix != 0 {
			name += strconv.Itoa(suffix)
		}
		root, err := metadata.CreateDirectory(filepath.Join("worktrees", name))
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, file := range []struct{ name, text string }{
			{"locked", "initializing\n"},
			{"gitdir", filepath.Join(worktree.Path, ".git") + "\n"},
			{"commondir", "../..\n"},
			{"HEAD", "ref: refs/heads/" + worktree.Branch + "\n"},
		} {
			if err := root.WriteFile(file.name, []byte(file.text), 0o644, true); err != nil {
				root.Close()
				return nil, err
			}
		}
		return root, nil
	}
}

func (m *Manager) restoreEntries(ctx context.Context, commit string) ([]restoredEntry, error) {
	tree, err := m.restoreObjectOutput(ctx, nil, m.localTimeout(), "ls-tree", "-r", "-z", commit)
	if err != nil {
		return nil, err
	}
	var entries []restoredEntry
	for _, record := range bytes.Split(tree, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		header, name, ok := strings.Cut(string(record), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || !commitPattern.MatchString(fields[2]) {
			return nil, errors.New("the restored tree could not be verified")
		}
		clean, err := repowrite.Relative(name)
		if err != nil || clean != name {
			return nil, fmt.Errorf("invalid restored tree path %q", name)
		}
		for _, component := range strings.Split(name, "/") {
			if strings.EqualFold(component, ".git") {
				return nil, fmt.Errorf("restored tree contains a Git administrative path: %s", name)
			}
		}
		mode, err := strconv.ParseUint(fields[0], 8, 32)
		if err != nil || (mode != 0o100644 && mode != 0o100755 && mode != 0o120000 && mode != 0o160000) {
			return nil, fmt.Errorf("unsupported restored tree mode: %s", fields[0])
		}
		entries = append(entries, restoredEntry{name: name, mode: uint32(mode), object: fields[2]})
	}
	return entries, nil
}

func (m *Manager) restoreBlobs(ctx context.Context, checkout *repowrite.PinnedRoot, entries []restoredEntry) error {
	var objects strings.Builder
	for _, entry := range entries {
		if entry.mode != 0o160000 {
			objects.WriteString(entry.object + "\n")
		}
	}
	reader, writer := io.Pipe()
	finished := make(chan error, 1)
	go func() {
		err := writeRestoredBlobs(checkout, entries, bufio.NewReader(reader))
		reader.CloseWithError(err)
		finished <- err
	}()
	result, err := m.runner.Run(ctx, execution.Command{
		Name:  m.gitBinary,
		Args:  append(append([]string{}, maintenanceOptions...), "-C", m.repositoryRoot, "cat-file", "--batch"),
		Env:   append(execution.GitEnvironment(nil), "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1"),
		Stdin: strings.NewReader(objects.String()), RawStdout: writer,
		Timeout: m.checkoutTimeout(len(entries), true),
	}, nil)
	writer.CloseWithError(err)
	writeErr := <-finished
	if err != nil || writeErr != nil {
		return errors.Join(err, writeErr)
	}
	if result.Status != execution.ProcessSucceeded {
		return fmt.Errorf("read restored blobs failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func writeRestoredBlobs(checkout *repowrite.PinnedRoot, entries []restoredEntry, reader *bufio.Reader) error {
	for i := range entries {
		entry := &entries[i]
		if parent := filepath.Dir(entry.name); parent != "." {
			if err := checkout.MakeDirectory(parent, 0o755); err != nil {
				return err
			}
		}
		if entry.mode == 0o160000 {
			if err := checkout.MakeDirectory(entry.name, 0o755); err != nil {
				return err
			}
			continue
		}
		header, err := reader.ReadString('\n')
		fields := strings.Fields(header)
		if err != nil || len(fields) != 3 || fields[0] != entry.object || fields[1] != "blob" {
			return errors.New("restored blob header could not be verified")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return errors.New("restored blob size could not be verified")
		}
		entry.size = uint32(size)
		hash := sha1.New()
		fmt.Fprintf(hash, "blob %d%c", size, 0)
		if entry.mode == 0o120000 {
			// Filesystem link targets are bounded, unlike regular blob data.
			if size > 4096 {
				return errors.New("restored symbolic link target exceeds 4096 bytes")
			}
			content := make([]byte, int(size))
			if _, err := io.ReadFull(reader, content); err != nil {
				return err
			}
			hash.Write(content)
			if err := checkout.Symlink(string(content), entry.name); err != nil {
				return err
			}
		} else {
			mode := fs.FileMode(0o644)
			if entry.mode == 0o100755 {
				mode = 0o755
			}
			file, err := checkout.FileWriter(entry.name, mode, true)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(io.MultiWriter(file, hash), reader, size)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
		}
		terminator, err := reader.ReadByte()
		if err != nil || terminator != '\n' || hex.EncodeToString(hash.Sum(nil)) != entry.object {
			return errors.New("restored blob does not match its recorded object")
		}
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		return errors.New("restored object output contains unexpected bytes")
	}
	return nil
}

// restoredIndex is Git's documented index format, version 3 (version 2 with
// optional extended flags). Zero stat fields make Git check actual file content;
// the object IDs, modes and skip-worktree bits describe the recorded tree.
func restoredIndex(entries []restoredEntry) []byte {
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var index bytes.Buffer
	index.WriteString("DIRC")
	binary.Write(&index, binary.BigEndian, uint32(3))
	binary.Write(&index, binary.BigEndian, uint32(len(entries)))
	for _, entry := range entries {
		start := index.Len()
		stat := [10]uint32{}
		stat[6] = entry.mode
		stat[9] = entry.size
		binary.Write(&index, binary.BigEndian, stat)
		object, _ := hex.DecodeString(entry.object)
		index.Write(object)
		flags := uint16(min(len(entry.name), 0xfff))
		if entry.export {
			flags |= 0x4000
		}
		binary.Write(&index, binary.BigEndian, flags)
		if entry.export {
			binary.Write(&index, binary.BigEndian, uint16(0x4000))
		}
		index.WriteString(entry.name)
		index.WriteByte(0)
		for (index.Len()-start)%8 != 0 {
			index.WriteByte(0)
		}
	}
	hash := sha1.Sum(index.Bytes())
	index.Write(hash[:])
	return index.Bytes()
}

type restoreOutput struct{ bytes.Buffer }

func (b *restoreOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 64<<20 {
		return 0, errors.New("safe checkout restoration exceeds the 64 MiB tree-listing bound")
	}
	return b.Buffer.Write(data)
}

func (m *Manager) restoreObjectOutput(ctx context.Context, input io.Reader, timeout time.Duration, args ...string) ([]byte, error) {
	var output restoreOutput
	result, err := m.runner.Run(ctx, execution.Command{
		Name: m.gitBinary,
		Args: append(append(append([]string{}, maintenanceOptions...), "-C", m.repositoryRoot), args...),
		Env:  append(execution.GitEnvironment(nil), "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1"), Stdin: input, RawStdout: &output, Timeout: timeout,
	}, nil)
	if err != nil {
		return nil, err
	}
	if result.Status != execution.ProcessSucceeded {
		return nil, fmt.Errorf("read restored objects failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return output.Bytes(), nil
}
