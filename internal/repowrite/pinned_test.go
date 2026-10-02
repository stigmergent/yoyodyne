package repowrite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedOperationsRefuseADirectoryReplacedAfterItWasChecked(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"directory", "lock", "append"} {
		t.Run(operation, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			confined, err := OpenPinnedRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			defer confined.Close()
			if err := confined.MakeDirectory("checked", 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(root, "checked"), filepath.Join(root, "original")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "checked")); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "directory":
				err = confined.MakeDirectory("checked/new", 0o700)
			case "lock":
				file, openErr := confined.OpenLock("checked/record.lock")
				if file != nil {
					file.Close()
				}
				err = openErr
			case "append":
				err = confined.AppendRecord("checked/record.jsonl", []byte("{}\n"), 0)
			}
			if err == nil {
				t.Fatal("replacement with an outside symlink was accepted")
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("outside entries = %v, %v", entries, err)
			}
		})
	}
}

func TestPinnedRootSurvivesReplacementOfItsPath(t *testing.T) {
	t.Parallel()
	base, outside := t.TempDir(), t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	confined, err := OpenPinnedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer confined.Close()
	original := filepath.Join(base, "original")
	if err := os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if err := confined.AppendRecord("record.jsonl", []byte("{}\n"), 0); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(original, "record.jsonl")); err != nil || string(content) != "{}\n" {
		t.Fatalf("original root = %q, %v", content, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside entries = %v, %v", entries, err)
	}
}
