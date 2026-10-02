package repowrite

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PinnedRoot holds the directory itself, rather than a pathname that can be
// replaced between validation and a write. Close releases its directory handle.
// Unlike Resolve, none of its writes hands an absolute path to another writer.
type PinnedRoot struct {
	root *os.Root
	path string
}

func OpenPinnedRoot(path string) (*PinnedRoot, error) {
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return nil, errors.New("this platform cannot pin a repository directory across replacement")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	base := filepath.VolumeName(absolute) + string(filepath.Separator)
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	pinned := &PinnedRoot{root: root, path: base}
	for _, component := range strings.Split(strings.TrimPrefix(absolute, base), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		child, err := pinned.OpenDirectory(component)
		pinned.Close()
		if err != nil {
			return nil, err
		}
		pinned = child
	}
	return pinned, nil
}

func (r *PinnedRoot) Close() error { return r.root.Close() }

// Path names the pinned directory for link text and diagnostics, never as a
// substitute for the handle when writing.
func (r *PinnedRoot) Path() string { return r.path }

// Unchanged is a refusal after a replacement, not the confinement mechanism:
// the handle keeps writes confined even while this answer becomes false.
func (r *PinnedRoot) Unchanged() error {
	info, err := os.Lstat(r.path)
	if err != nil {
		return err
	}
	opened, err := r.root.Stat(".")
	if err != nil || !info.IsDir() || !os.SameFile(info, opened) {
		return fmt.Errorf("repository write root was replaced: %s", r.path)
	}
	return nil
}

func (r *PinnedRoot) OpenDirectory(relative string) (*PinnedRoot, error) {
	info, err := r.root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository directory must not be a symlink: %s", relative)
	}
	root, err := r.root.OpenRoot(relative)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, fmt.Errorf("repository directory changed while being opened: %s", relative)
	}
	return &PinnedRoot{root: root, path: filepath.Join(r.path, relative)}, nil
}

func (r *PinnedRoot) MakeDirectory(relative string, mode fs.FileMode) error {
	clean, err := Relative(relative)
	if err != nil {
		return err
	}
	for prefix := ""; ; {
		component, remainder, more := strings.Cut(clean, "/")
		prefix = filepath.Join(prefix, component)
		if err := r.root.Mkdir(prefix, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err := r.root.Stat(prefix)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("repository path is not a directory: %s", prefix)
		}
		if !more {
			return nil
		}
		clean = remainder
	}
}

// CreateDirectory reserves a new name; an existing directory is a conflict.
func (r *PinnedRoot) CreateDirectory(relative string) (*PinnedRoot, error) {
	if _, err := Relative(relative); err != nil {
		return nil, err
	}
	if err := r.root.Mkdir(relative, 0o700); err != nil {
		return nil, err
	}
	return r.OpenDirectory(relative)
}

func (r *PinnedRoot) WriteFile(relative string, content []byte, mode fs.FileMode, exclusive bool) error {
	file, err := r.FileWriter(relative, mode, exclusive)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

// FileWriter streams into a file opened through the pinned root. All later
// writes keep that file handle even if its pathname is replaced.
func (r *PinnedRoot) FileWriter(relative string, mode fs.FileMode, exclusive bool) (io.WriteCloser, error) {
	clean, err := Relative(relative)
	if err != nil {
		return nil, err
	}
	if parent := filepath.Dir(clean); parent != "." {
		if err := r.MakeDirectory(parent, 0o755); err != nil {
			return nil, err
		}
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags |= os.O_EXCL
	}
	file, err := r.root.OpenFile(clean, flags, mode)
	if err != nil {
		return nil, err
	}
	return &pinnedFileWriter{file}, nil
}

type pinnedFileWriter struct{ file *os.File }

func (w *pinnedFileWriter) Write(data []byte) (int, error) { return w.file.Write(data) }
func (w *pinnedFileWriter) Close() error                   { return w.file.Close() }

func (r *PinnedRoot) ReadFile(relative string) ([]byte, error) {
	return fs.ReadFile(r.root.FS(), relative)
}

func (r *PinnedRoot) ReadDirectory(relative string) ([]fs.DirEntry, error) {
	return fs.ReadDir(r.root.FS(), relative)
}

func (r *PinnedRoot) Remove(relative string) error { return r.root.Remove(relative) }

func (r *PinnedRoot) Exists(relative string) (bool, error) {
	_, err := r.root.Lstat(relative)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
