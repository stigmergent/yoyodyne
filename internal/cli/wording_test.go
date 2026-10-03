package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/console"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/report"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/sweep"
)

func TestTheReportsCommandFlagsWordsInADigestAndItsHandling(t *testing.T) {
	t.Parallel()
	digest := report.Report{ID: "report-one", Role: "program-manager", Message: "digest: the posture changed"}
	handled := map[string]report.Handling{digest.ID: {Reason: "change its cadence"}}
	var out strings.Builder
	writeReports(&out, console.NewTheme(func(string) string { return "" }, nil), []report.Report{digest}, handled, nil, nil, readmodel.ReadTextTerms("../.."))
	for _, term := range []string{"posture", "cadence"} {
		if !strings.Contains(out.String(), `[wording: "`+term+`" was replaced`) {
			t.Errorf("digest = %q, want %s flagged", out.String(), term)
		}
	}
	if digest.Message != "digest: the posture changed" || handled[digest.ID].Reason != "change its cadence" {
		t.Fatal("rendering rewrote a record")
	}
}

func TestTheSweepListingFlagsTheSummaryQuestionsAndFindings(t *testing.T) {
	t.Parallel()
	account := &sweep.Result{Status: sweep.StatusComplete, Summary: "the posture changed", Questions: []string{"change the cadence?"}, Findings: []sweep.Finding{{Issue: "an idle-bound", Detail: "a stall continuation"}}}
	pass := recordedSweep("look", time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC), account, "")
	text := renderSweeps([]runstate.Sweep{pass}, nil, 0, nil, readmodel.ReadTextTerms("../.."))
	for _, term := range []string{"posture", "cadence", "idle bound", "stall continuation"} {
		if !strings.Contains(text, `[wording: "`+term+`" was replaced`) {
			t.Errorf("pass = %q, want %s flagged", text, term)
		}
	}
	if account.Summary != "the posture changed" {
		t.Fatal("rendering rewrote the pass account")
	}
}
