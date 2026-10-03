package chat

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/console"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/report"
)

// A program manager's digest is a report, and it names work the way the lane
// report the operator read on 2026-09-26 did: by number. The listing shows each
// item beside its title, and an item the tracker does not hold as unknown.
func TestTheReportListingShowsEveryItemBesideItsTitle(t *testing.T) {
	t.Parallel()

	digest := report.Report{
		SchemaVersion: report.SchemaVersion,
		ID:            "report-0123456789abcdef0123456789abcdef",
		Role:          "program-manager",
		Agent:         "factory-pgm",
		ProductID:     "yoyodyne",
		RepositoryID:  "yoyodyne",
		Severity:      report.SeverityNote,
		Message:       "digest: admitted 434.9; objecting to 434.3; yoyodyne-ifd.999.1 is still open",
		RecordedAt:    fixedClock{}.Now(),
	}
	titles := readmodel.NewWorkItemTitles([]beads.WorkItem{
		{ID: "yoyodyne-ifd.434.9", Title: "Price a resumed session at what it moved by"},
		{ID: "yoyodyne-ifd.434.3", Title: "Say the provider's reset in local time"},
	})
	rendered := renderCollectedReports(console.NewTheme(func(string) string { return "" }, nil), []report.Report{digest}, nil, nil, titles, fixedClock{}.Now())
	for _, want := range []string{
		"(P0) Price a resumed session at what it moved by (yoyodyne-ifd.434.9)",
		"(P0) Say the provider's reset in local time (yoyodyne-ifd.434.3)",
		"title unavailable (yoyodyne-ifd.999.1)",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("renderCollectedReports() = %q, want it to carry %q", rendered, want)
		}
	}
}
