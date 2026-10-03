package repowrite

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedCreatePublishesOnlyOneCompleteRecord(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.CreateFile("activation.json", []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.CreateFile("activation.json", []byte("second\n"), 0o600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second create = %v, want the original record retained", err)
	}
	if content, err := os.ReadFile(filepath.Join(path, "activation.json")); err != nil || string(content) != "first\n" {
		t.Fatalf("activation = %q, %v", content, err)
	}
	if entries, err := os.ReadDir(path); err != nil || len(entries) != 1 {
		t.Fatalf("temporary files after create = %v, %v", entries, err)
	}
}

func TestPinnedCreateRemainsConfinedAcrossDirectoryReplacement(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(path, path+"-held"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(path + "-held") })
	outside := t.TempDir()
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := root.CreateFile("activation.json", []byte("record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(path+"-held", "activation.json")); err != nil || string(content) != "record\n" {
		t.Fatalf("record in held root = %q, %v", content, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("record escaped after replacement = %v, %v", entries, err)
	}
}

func TestPinnedReplacementLeavesAnOutsideHardLinkUnchanged(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("keep this\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(path, "target")); err != nil {
		t.Fatal(err)
	}
	writer, err := root.FileWriter("target", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	if _, err := writer.Write([]byte("replacement\n")); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	// Streaming must not expose a partial replacement or change the old inode.
	if content, err := os.ReadFile(filepath.Join(path, "target")); err != nil || string(content) != "keep this\n" {
		writer.Close()
		t.Fatalf("target before close = %q, %v", content, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
	if content, err := os.ReadFile(outside); err != nil || string(content) != "keep this\n" {
		t.Fatalf("outside sentinel = %q, %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(path, "target")); err != nil || string(content) != "replacement\n" {
		t.Fatalf("replacement = %q, %v", content, err)
	}
	original, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Stat(filepath.Join(path, "target"))
	if err != nil || os.SameFile(original, replacement) {
		t.Fatalf("replacement still uses the outside inode: %v", err)
	}
	if entries, err := os.ReadDir(path); err != nil || len(entries) != 1 {
		t.Fatalf("temporary files after replacement = %v, %v", entries, err)
	}
}

func TestPinnedReplacementPreservesTheOriginalAfterAWriteFailure(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(path, "target"), []byte("keep this\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer, err := root.FileWriter("target", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	// Simulate a file that cannot accept the stream, before it is published.
	if err := writer.(*pinnedReplacementWriter).file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("replacement\n")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write failure = %v", err)
	}
	if err := writer.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("close did not report the failed write: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(path, "target")); err != nil || string(content) != "keep this\n" {
		t.Fatalf("original after failed write = %q, %v", content, err)
	}
	if entries, err := os.ReadDir(path); err != nil || len(entries) != 1 {
		t.Fatalf("temporary files after failed write = %v, %v", entries, err)
	}
}

func TestPinnedReplacementStaysConfinedAfterItsParentIsReplaced(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("nested/target", []byte("original\n"), 0o644, true); err != nil {
		t.Fatal(err)
	}
	writer, err := root.FileWriter("nested/target", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target"), []byte("keep this\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(path, "nested"), filepath.Join(path, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(path, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("replacement\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(outside, "target")); err != nil || string(content) != "keep this\n" {
		t.Fatalf("outside target = %q, %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(path, "moved/target")); err != nil || string(content) != "replacement\n" {
		t.Fatalf("pinned replacement = %q, %v", content, err)
	}
}

func TestPinnedWritesStayInTheirDirectoryAfterRootReplacement(t *testing.T) {
	t.Parallel()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "root")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := t.TempDir()
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("nested/file", []byte("confined"), 0o644, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "nested")); !os.IsNotExist(err) {
		t.Fatalf("write escaped: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(moved, "nested/file"))
	if err != nil || string(content) != "confined" {
		t.Fatalf("pinned content = %q, %v", content, err)
	}
	if err := root.Unchanged(); err == nil {
		t.Fatal("replaced root reported unchanged")
	}
}

func TestPinnedWritesRefuseAnEscapingIntermediateSymlink(t *testing.T) {
	t.Parallel()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, err := OpenPinnedRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(path, "nested")); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("nested/file", []byte("escape"), 0o644, true); err == nil {
		t.Fatal("accepted an escaping intermediate link")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside files = %v, %v", entries, err)
	}
}
