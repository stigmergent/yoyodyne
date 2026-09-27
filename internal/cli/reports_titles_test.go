package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/console"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/report"
)

// `yoyo reports` is where a program manager's digest is read without opening a
// conversation, and a digest names work by number. Each item is printed beside
// its title — in the report and in what became of it, without titling one the
// report already titled — and one the tracker does not hold as unknown.
func TestTheReportsCommandShowsEveryItemBesideItsTitle(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	digest := report.Report{
		SchemaVersion: report.SchemaVersion,
		ID:            "report-0123456789abcdef0123456789abcdef",
		Role:          "program-manager",
		Agent:         "factory-pgm",
		ProductID:     "yoyodyne",
		RepositoryID:  "yoyodyne",
		Severity:      report.SeverityNote,
		Message:       "digest: admitted 434.9; yoyodyne-ifd.999.1 is still open",
		RecordedAt:    at,
	}
	handled := map[string]report.Handling{digest.ID: {
		ReportID: digest.ID, Role: "product-manager", RunID: "chat-1",
		Reason: "434.9 already covers it; admitted as 434.3", RecordedAt: at.Add(time.Hour),
	}}
	titles := readmodel.NewWorkItemTitles([]beads.WorkItem{
		{ID: "yoyodyne-ifd.434.9", Title: "Price a resumed session at what it moved by"},
		{ID: "yoyodyne-ifd.434.3", Title: "Say the provider's reset in local time"},
	})

	var out strings.Builder
	writeReports(&out, console.NewTheme(func(string) string { return "" }, nil), []report.Report{digest}, handled, nil, titles)
	printed := out.String()
	for _, want := range []string{
		"434.9 (Price a resumed session at what it moved by)",
		"yoyodyne-ifd.999.1 (unknown to the tracker)",
		"admitted as 434.3 (Say the provider's reset in local time)",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("writeReports() = %q, want it to carry %q", printed, want)
		}
	}
	if got := strings.Count(printed, "Price a resumed session"); got != 1 {
		t.Errorf("writeReports() = %q, want the item the report titled left bare in its handling, titled %d times", printed, got)
	}
}
