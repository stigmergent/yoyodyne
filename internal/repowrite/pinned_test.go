package repowrite

import (
	"os"
	"path/filepath"
	"testing"
)

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
