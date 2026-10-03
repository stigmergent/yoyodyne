package beads

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

func TestTrackerWritesDisableAutomaticExportEvenWhenInherited(t *testing.T) {
	t.Parallel()
	environment := withoutAutoExport([]string{"BD_EXPORT_AUTO=true", "PATH=/tools", "BD_EXPORT_AUTO=1"})
	if !slices.Equal(environment, []string{"PATH=/tools", "BD_EXPORT_AUTO=false"}) {
		t.Fatalf("environment = %v", environment)
	}
	runner := &fakeRunner{responses: []string{"updated"}}
	if _, err := (Client{Runner: runner}).run(context.Background(), "update", "item"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(runner.commands[0].Env, "BD_EXPORT_AUTO=false") {
		t.Fatal("the bd invocation did not disable auto-export")
	}
}

func TestTrackerExportPublishesOnlyACompleteSnapshotAndSharesRecentCopies(t *testing.T) {
	t.Parallel()
	repository := exportRepository(t)
	runner := &fakeRunner{responses: []string{"{\"id\":\"new-item\",\"notes\":\"exact bytes\"}\n", "not JSON\n"}}
	client := Client{Dir: repository, Runner: runner}
	if err := client.RefreshExport(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repository, ExportPath)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.RefreshExportIfDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatal("a recent export was rewritten for another reader")
	}
	if err := client.RefreshExport(context.Background()); err == nil {
		t.Fatal("an invalid export was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed export changed the snapshot: %q, %v", after, err)
	}
	if runner.commands[0].RawStdout == nil || !slices.Equal(runner.commands[0].Args, []string{"export"}) {
		t.Fatal("the export was not read as exact stdout bytes")
	}
	// A killed export also leaves the previous file intact.
	runner.results = []execution.ProcessResult{{Status: execution.ProcessTimedOut, Stderr: "killed before export finished"}}
	runner.commands, runner.args = nil, nil
	if err := client.RefreshExport(context.Background()); err == nil {
		t.Fatal("a timed-out export was accepted")
	}
	after, err = os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("timed-out export changed the snapshot: %q, %v", after, err)
	}
}

func TestTrackerExportRefusesAnEscapingDirectory(t *testing.T) {
	t.Parallel()
	repository, outside := t.TempDir(), exportRepository(t)
	if err := os.Symlink(filepath.Join(outside, ".beads"), filepath.Join(repository, ".beads")); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{responses: []string{"{\"id\":\"item\"}\n"}}
	if err := (Client{Dir: repository, Runner: runner}).RefreshExport(context.Background()); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("escaping export error = %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatal("bd was asked before the write destination was confined")
	}
}

func exportRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".beads", "metadata.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repository
}
