package repowrite

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Every public mutation is stopped at the former resolve-to-write gap. Replacing
// the root, a parent, or the target there must never change the external tree.
func TestRootMutationsRemainConfinedAfterReplacement(t *testing.T) {
	t.Parallel()
	operations := []struct {
		name      string
		directory bool
		mutate    func(Root) error
	}{
		{"create", false, func(r Root) error { _, _, err := r.CreateFile("docs/target", []byte("changed")); return err }},
		{"replace", false, func(r Root) error { _, err := r.WriteFile("docs/target", []byte("changed")); return err }},
		{"append", false, func(r Root) error {
			file, err := r.OpenAppend("docs/target", 0o600, 0o700)
			if err != nil {
				return err
			}
			_, err = file.WriteString("changed")
			return errors.Join(err, file.Close())
		}},
		{"truncate", false, func(r Root) error { _, err := r.Truncate("docs/target", 0); return err }},
		{"mkdir", true, func(r Root) error { _, err := r.MakeDirectory("docs/target/new", 0o700); return err }},
		{"remove directory", true, func(r Root) error { _, err := r.RemoveDirectory("docs/target"); return err }},
		{"remove file", false, func(r Root) error { _, err := r.RemoveFile("docs/target"); return err }},
	}
	for _, operation := range operations {
		for _, replacement := range []string{"root", "parent", "target"} {
			t.Run(operation.name+"/"+replacement, func(t *testing.T) {
				t.Parallel()
				root, outside := repository(t)
				makeDirectory(t, filepath.Join(root.Path(), "docs"))
				for _, prefix := range []string{root.Path(), outside, filepath.Join(outside, "docs")} {
					if operation.directory {
						writeFile(t, filepath.Join(prefix, "target", "sentinel"), "keep")
					} else {
						writeFile(t, filepath.Join(prefix, "target"), "keep")
					}
				}
				// The normal target belongs under docs, as do root replacements.
				if operation.directory {
					writeFile(t, filepath.Join(root.Path(), "docs", "target", "sentinel"), "keep")
				} else if operation.name != "create" {
					writeFile(t, filepath.Join(root.Path(), "docs", "target"), "keep")
				}
				before := treeSnapshot(t, outside)
				called := false
				root.beforeMutation = func() {
					called = true
					victim, target := root.Path(), outside
					switch replacement {
					case "parent":
						victim = filepath.Join(root.Path(), "docs")
					case "target":
						victim = filepath.Join(root.Path(), "docs", "target")
						target = filepath.Join(outside, "target")
					}
					replaceWhileMutationWaits(t, victim, target)
				}
				// A refusal is fine, but an accepted operation must be equally safe.
				_ = operation.mutate(root)
				if !called {
					t.Fatal("mutation did not reach the replacement barrier")
				}
				if after := treeSnapshot(t, outside); !reflect.DeepEqual(before, after) {
					t.Fatalf("mutation escaped: before = %v, after = %v", before, after)
				}
			})
		}
	}
}

func replaceWhileMutationWaits(t *testing.T, victim, target string) {
	t.Helper()
	t.Cleanup(func() { os.RemoveAll(victim + "-held") })
	replaced := make(chan error)
	go func() {
		if _, err := os.Lstat(victim); err == nil {
			if err := os.Rename(victim, victim+"-held"); err != nil {
				replaced <- err
				return
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			replaced <- err
			return
		}
		replaced <- os.Symlink(target, victim)
	}()
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
}

func TestPinnedPublicationAndCleanupUseTheHeldParent(t *testing.T) {
	t.Parallel()
	root, outside := repository(t)
	pinned, err := OpenPinnedRoot(root.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	writer, err := pinned.FileWriter("docs/target", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := os.Rename(filepath.Join(root.Path(), "docs"), filepath.Join(root.Path(), "held")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root.Path(), "docs")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("complete")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, filepath.Join(root.Path(), "held", "target")); content != "complete" {
		t.Fatalf("held target = %q", content)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("publication escaped: %v, %v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(root.Path(), "held")); err != nil || len(entries) != 1 {
		t.Fatalf("temporary file was not cleaned up: %v, %v", entries, err)
	}
}

func TestMutationsPreserveContainedLinksAndRefuseDanglingLinks(t *testing.T) {
	t.Parallel()
	root, _ := repository(t)
	writeFile(t, filepath.Join(root.Path(), "real", "file"), "before")
	link(t, "real", filepath.Join(root.Path(), "alias"))
	if _, _, err := root.CreateFile("alias/new", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := root.MakeDirectory("alias/directory", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Truncate("alias/file", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := root.RemoveFile("alias/new"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.RemoveDirectory("alias/directory"); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, filepath.Join(root.Path(), "real", "file")); content != "be" {
		t.Fatalf("truncated file = %q", content)
	}
	link(t, "missing", filepath.Join(root.Path(), "dangling"))
	if _, err := root.WriteFile("dangling/new", []byte("new")); err == nil {
		t.Fatal("write treated a dangling link as a missing directory")
	}
	if _, err := os.Stat(filepath.Join(root.Path(), "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dangling link target was created: %v", err)
	}
}

func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[relative] = "directory"
		} else {
			result[relative] = readFile(t, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
