package repowrite

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"
)

// PinnedRoot holds the declared root open. Each descendant operation walks
// relative to that descriptor, so replacing a path component cannot redirect a
// subsequent mutation outside it. Close releases the descriptor.
type PinnedRoot struct {
	root *os.Root
}

func OpenPinnedRoot(root string) (*PinnedRoot, error) {
	// These platforms cannot provide descriptor-relative containment through
	// os.Root. Refuse rather than fall back to checking and then using a path.
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return nil, errors.ErrUnsupported
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open the declared filesystem root: %w", err)
	}
	return &PinnedRoot{root: opened}, nil
}

func (r *PinnedRoot) Close() error { return r.root.Close() }

// MakeDirectory creates missing descendants through the root descriptor.
// MkdirAll is deliberately expressed using Mkdir, available in Go 1.24.
func (r *PinnedRoot) MakeDirectory(relative string, mode fs.FileMode) error {
	clean, err := Relative(relative)
	if err != nil {
		return err
	}
	parts := strings.Split(clean, "/")
	for index := range parts {
		name := strings.Join(parts[:index+1], "/")
		if err := r.root.Mkdir(name, mode); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create confined directory %s: %w", name, err)
			}
			info, err := r.root.Stat(name)
			if err != nil {
				return fmt.Errorf("inspect confined directory %s: %w", name, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", name)
			}
		}
	}
	return nil
}

// OpenLock creates or opens a lock file within the pinned root, refusing a
// symlink at the file itself. The caller owns its descriptor and lock lifetime.
func (r *PinnedRoot) OpenLock(relative string) (*os.File, error) {
	clean, err := Relative(relative)
	if err != nil {
		return nil, err
	}
	flags := appendFlags&^(os.O_WRONLY|os.O_APPEND) | os.O_RDWR
	return r.root.OpenFile(clean, flags, 0o600)
}

func (r *PinnedRoot) ReadFile(relative string) ([]byte, error) {
	clean, err := Relative(relative)
	if err != nil {
		return nil, err
	}
	file, err := r.root.Open(clean)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// AppendRecord appends and syncs one newline-delimited record while the caller
// holds the file's lock. completeBytes is the end of the last complete record;
// an interrupted trailing write is removed through the same confined descriptor
// before appending. No temporary path or rename can leave the declared root.
func (r *PinnedRoot) AppendRecord(relative string, record []byte, completeBytes int64) error {
	clean, err := Relative(relative)
	if err != nil {
		return err
	}
	if completeBytes < 0 || len(record) == 0 || record[len(record)-1] != '\n' || bytes.ContainsRune(record[:len(record)-1], '\n') {
		return errors.New("append requires one complete record and a nonnegative offset")
	}
	file, err := r.root.OpenFile(clean, appendFlags, 0o600)
	if err != nil {
		return fmt.Errorf("open confined record %s: %w", clean, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if completeBytes > info.Size() {
		return errors.New("the complete record offset exceeds the file's size")
	}
	if err := file.Truncate(completeBytes); err != nil {
		return fmt.Errorf("remove incomplete confined record: %w", err)
	}
	if n, err := file.Write(record); err != nil {
		return err
	} else if n != len(record) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync confined record: %w", err)
	}
	// Persist creation of the journal and its containing directories too. This
	// opens the directory through the root, never by its replaceable absolute path.
	parts := strings.Split(clean, "/")
	for index := len(parts) - 1; index >= 0; index-- {
		name := "."
		if index > 0 {
			name = strings.Join(parts[:index], "/")
		}
		directory, err := r.root.Open(name)
		if err != nil {
			return err
		}
		err = directory.Sync()
		closeErr := directory.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	return nil
}
