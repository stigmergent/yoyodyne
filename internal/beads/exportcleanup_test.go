package beads

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

func TestExportCleanupKeepsYoungAndLiveFilesAndRemovesAbandonedOnes(t *testing.T) {
	t.Parallel()
	repository := exportRepository(t)
	now := time.Now().UTC()
	old := now.Add(-ExportTemporaryAge - time.Second)
	dead := temporaryExport(t, repository, ".~issues.jsonl.1", old)
	partial := temporaryExport(t, repository, ".~interactions.jsonl.2", old)
	live := temporaryExport(t, repository, ".~issues.jsonl.3", old)
	snapshot := temporaryExport(t, repository, ".yoyo-write-SNAPSHOT.tmp", old)
	liveSnapshot := temporaryExport(t, repository, ".yoyo-write-LIVE.tmp", old)
	boundary := temporaryExport(t, repository, ".~issues.jsonl.4", now.Add(-ExportTemporaryAge))
	young := temporaryExport(t, repository, ".~issues.jsonl.5", now.Add(-time.Hour))
	future := temporaryExport(t, repository, ".~issues.jsonl.6", now.Add(time.Hour))
	unrelated := temporaryExport(t, repository, ".~issues.jsonl.not-bd", old)
	link := filepath.Join(repository, ".beads", ".~issues.jsonl.7")
	if err := os.Symlink(dead, link); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(repository, ".beads", ".~issues.jsonl.8")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &exportCleanupRunner{output: lsofIdentity(t, live) + lsofIdentity(t, liveSnapshot)}
	cleaned, err := CleanExportTemporaries(context.Background(), repository, runner, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleaned.Removed) != 3 || len(cleaned.Kept) != 2 {
		t.Fatalf("cleanup = %+v", cleaned)
	}
	for _, path := range []string{dead, partial, snapshot} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("abandoned file %s remains: %v", path, err)
		}
	}
	for _, path := range []string{live, liveSnapshot, boundary, young, future, unrelated, link, directory} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("protected file %s was removed: %v", path, err)
		}
	}
	for _, removed := range cleaned.Removed {
		if removed.Bytes != int64(len("partial export\n")) || !removed.ModifiedAt.Equal(old) {
			t.Fatalf("removal lacks its size and age: %+v", removed)
		}
	}
}

func TestExportCleanupKeepsEverythingWhenOpenFilesCannotBeEstablished(t *testing.T) {
	t.Parallel()
	for name, runner := range map[string]*exportCleanupRunner{
		"missing lsof":  {err: errors.New("lsof not found")},
		"timed out":     {result: execution.ProcessResult{Status: execution.ProcessTimedOut}},
		"permission":    {result: execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 1, Stderr: "permission denied"}},
		"truncated":     {result: execution.ProcessResult{Status: execution.ProcessSucceeded, OutputTruncation: "cut"}},
		"malformed":     {output: "p123\x00\nf3\x00i1\x00\n"},
		"empty success": {result: execution.ProcessResult{Status: execution.ProcessSucceeded}},
	} {
		t.Run(name, func(t *testing.T) {
			repository := exportRepository(t)
			now := time.Now()
			path := temporaryExport(t, repository, ".~issues.jsonl.1", now.Add(-2*ExportTemporaryAge))
			cleaned, err := CleanExportTemporaries(context.Background(), repository, runner, now)
			if err == nil || len(cleaned.Removed) != 0 {
				t.Fatalf("cleanup = %+v, %v; want no removal", cleaned, err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("a file with unknown holders was removed")
			}
		})
	}
}

func TestExportCleanupKeepsAFileReplacedDuringInspection(t *testing.T) {
	t.Parallel()
	repository := exportRepository(t)
	now := time.Now()
	path := temporaryExport(t, repository, ".~issues.jsonl.1", now.Add(-2*ExportTemporaryAge))
	runner := &exportCleanupRunner{result: noOpenExports(), inspect: func() {
		if err := os.Rename(path, path+".saved"); err != nil {
			t.Fatal(err)
		}
		temporaryExport(t, repository, ".~issues.jsonl.1", now.Add(-2*ExportTemporaryAge))
	}}
	cleaned, err := CleanExportTemporaries(context.Background(), repository, runner, now)
	if err != nil || len(cleaned.Removed) != 0 || len(cleaned.Kept) != 1 {
		t.Fatalf("replacement cleanup = %+v, %v", cleaned, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the replacement file was removed")
	}
}

func TestExportCleanupRefusesADirectoryReplacedDuringInspection(t *testing.T) {
	t.Parallel()
	repository, outside := exportRepository(t), exportRepository(t)
	now := time.Now()
	old := now.Add(-2 * ExportTemporaryAge)
	temporaryExport(t, repository, ".~issues.jsonl.1", old)
	out := temporaryExport(t, outside, ".~issues.jsonl.1", old)
	runner := &exportCleanupRunner{result: noOpenExports(), inspect: func() {
		if err := os.Rename(filepath.Join(repository, ".beads"), filepath.Join(repository, "saved")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(outside, ".beads"), filepath.Join(repository, ".beads")); err != nil {
			t.Fatal(err)
		}
	}}
	cleaned, err := CleanExportTemporaries(context.Background(), repository, runner, now)
	if err == nil || len(cleaned.Removed) != 0 {
		t.Fatalf("changed directory cleanup = %+v, %v", cleaned, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("cleanup escaped the held directory")
	}
}

func TestExportCleanupRecognizesARealOpenFileThroughAHardLink(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skipf("SKIPPED, not passed: lsof is unavailable: %v", err)
	}
	repository := exportRepository(t)
	now := time.Now()
	live := temporaryExport(t, repository, ".~issues.jsonl.1", now.Add(-2*ExportTemporaryAge))
	dead := temporaryExport(t, repository, ".~issues.jsonl.2", now.Add(-2*ExportTemporaryAge))
	alias := filepath.Join(t.TempDir(), "held-through-another-name")
	if err := os.Link(live, alias); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cleaned, err := CleanExportTemporaries(context.Background(), repository, execution.OSProcessRunner{}, now)
	if err != nil || len(cleaned.Removed) != 1 || len(cleaned.Kept) != 1 {
		t.Fatalf("real open-file cleanup = %+v, %v", cleaned, err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("the live file was removed")
	}
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the abandoned file was kept")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	// Once the holder is gone, the same old temporary can be removed.
	cleaned, err = CleanExportTemporaries(context.Background(), repository, execution.OSProcessRunner{}, now)
	if err != nil || len(cleaned.Removed) != 1 {
		t.Fatalf("closed file cleanup = %+v, %v", cleaned, err)
	}
}

func temporaryExport(t *testing.T, repository, name string, at time.Time) string {
	t.Helper()
	path := filepath.Join(repository, ".beads", name)
	if err := os.WriteFile(path, []byte("partial export\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	return path
}

func lsofIdentity(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := exportIdentity(info)
	if !ok {
		t.Skip("this platform cannot identify open files")
	}
	return fmt.Sprintf("p123\x00\nf3r\x00D0x%x\x00i%d\x00\n", identity.device, identity.inode)
}

func noOpenExports() execution.ProcessResult {
	return execution.ProcessResult{Status: execution.ProcessFailed, ExitCode: 1}
}

type exportCleanupRunner struct {
	output  string
	result  execution.ProcessResult
	err     error
	inspect func()
}

func (r *exportCleanupRunner) Run(_ context.Context, command execution.Command, _ execution.OutputObserver) (execution.ProcessResult, error) {
	if command.Name != "lsof" || !strings.Contains(strings.Join(command.Args, " "), "-F0fDi") {
		return execution.ProcessResult{}, errors.New("unexpected open-file inspection")
	}
	if r.inspect != nil {
		r.inspect()
	}
	if r.err != nil {
		return execution.ProcessResult{}, r.err
	}
	if r.output != "" {
		_, err := io.WriteString(command.RawStdout, r.output)
		return execution.ProcessResult{Status: execution.ProcessSucceeded}, err
	}
	return r.result, nil
}
