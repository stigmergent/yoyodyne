package readmodel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestALaneReportWithARetiredWordIsFlaggedWithoutChangingTheRecord(t *testing.T) {
	t.Parallel()
	current := blockedBy("factory-pgm", "report-unknown")
	current.Report.Summary = "The provider's posture changed."
	current.Report.Remaining = []string{"the cadence needs changing"}
	current.Report.Blockers[0].What = "the idle-bound ended the run"
	sources := programManagerSources()
	sources.Repository = "../.."
	sources.LaneReports = fakeLaneReports{reports: map[string]runstate.LaneReport{"factory-pgm": current}}
	answer, err := ReadProgramManagerReport(sources, "factory-pgm")
	if err != nil || answer.Report == nil {
		t.Fatalf("ReadProgramManagerReport() = %+v, %v", answer, err)
	}
	for _, test := range []struct{ text, term, replacement string }{
		{answer.Report.Summary, "posture", "tool access"},
		{answer.Report.Remaining[0], "cadence", "how often it repeats"},
		{answer.Instance.Claims[0].What, "idle bound", "produced no output"},
	} {
		if !strings.Contains(test.text, `[wording: "`+test.term+`" was replaced`) || !strings.Contains(test.text, test.replacement) {
			t.Errorf("text = %q, want a flag naming %q and %q", test.text, test.term, test.replacement)
		}
	}
	stored, _, _ := sources.LaneReports.Current("factory-pgm")
	if stored.Report.Summary != current.Report.Summary || stored.Report.Remaining[0] != current.Report.Remaining[0] || stored.Report.Blockers[0].What != current.Report.Blockers[0].What {
		t.Fatalf("rendering changed the author's report: %+v", stored.Report)
	}
	standing := ReadStanding(context.Background(), sources)
	if !strings.Contains(standing.ProgramManagers[0].Claims[0].What, "[wording:") {
		t.Fatal("the standing did not carry the same language flag as the report query")
	}
}

func TestWordingRenderIsRepeatableAndKeepsTheOriginalText(t *testing.T) {
	t.Parallel()
	words := ReadTextTerms("../..")
	text := "the posture is wedged\n"
	flagged := words.Render(text)
	if flagged == text || !strings.HasPrefix(flagged, strings.TrimSuffix(text, "\n")) || !strings.HasSuffix(flagged, "\n") {
		t.Fatalf("Render() = %q, want the original words with flags before its newline", flagged)
	}
	if again := words.Render(flagged); again != flagged {
		t.Fatalf("second Render() = %q, want %q", again, flagged)
	}
	if plain := words.Render("the tool access changed"); plain != "the tool access changed" {
		t.Errorf("ordinary words were flagged: %q", plain)
	}
}

func TestWordingReadsTheCurrentRegisterAndNamesAnUnreadableOne(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if text := ReadTextTerms(root).Render("the posture changed"); text != "the posture changed" {
		t.Errorf("a project without a register was checked: %q", text)
	}
	directory := filepath.Join(root, "docs")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "terms.md")
	registered := "## The register\n| `posture` | tool access | the report |\n"
	if err := os.WriteFile(path, []byte(registered), 0o644); err != nil {
		t.Fatal(err)
	}
	if text := ReadTextTerms(root).Render("the posture changed"); text != "the posture changed" {
		t.Errorf("a registered term was flagged: %q", text)
	}
	if err := os.WriteFile(path, []byte("## The register\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if text := ReadTextTerms(root).Render("the posture changed"); !strings.Contains(text, "[wording:") {
		t.Errorf("removing the row did not take effect: %q", text)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if text := ReadTextTerms(root).Render("the report"); !strings.Contains(text, "wording could not be checked") {
		t.Errorf("an unreadable register was silent: %q", text)
	}
}

func TestTheAttentionLineFlagsARolesWords(t *testing.T) {
	t.Parallel()
	words := ReadTextTerms("../..")
	entry := Attention{
		Kind:         AttentionUntracedPass,
		UntracedPass: &UntracedPass{Task: "report-pass", Findings: 1, First: "the posture changed"},
		wording:      words,
	}
	if said := entry.CitedWhat(); !strings.Contains(said, `"posture" was replaced; write tool access`) {
		t.Errorf("CitedWhat() = %q, want the language flag beside the role's finding", said)
	}
	if strings.Contains(entry.What(), "[wording:") {
		t.Fatal("the entry's original words were changed")
	}
}
