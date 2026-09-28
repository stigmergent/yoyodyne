package staleness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/goal"
)

// Two documents in the product's specification home that contradict each other
// are reported naming both, read end to end through the goals and non-goals the
// documents themselves state.
func TestContradictingDocumentsInTheSpecificationHomeAreReportedNamingBoth(t *testing.T) {
	root := t.TempDir()
	write := func(relative, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/v1-goals.md", "# Goals\n\nWhy.\n\n## Goals\n\n- [hosted] Offer a hosted control plane.\n- [local] Run locally.\n")
	write("docs/v1-non-goals.md", "# Non-goals\n\nWhat v1 stops at.\n\n## Non-goals\n\n- Offer a hosted control plane.\n- Replace Git.\n")
	write("docs/ops-goals.md", "# Ops\n\nWhy.\n\n## Goals\n\n- [local] Keep the harness observable.\n")

	artifacts := set(
		document("brief", artifact.KindBrief, nil, created("2026-08-01T00:00:00Z")),
		document("second-brief", artifact.KindBrief, nil, created("2026-08-01T00:00:00Z")),
		document("v1-goals", artifact.KindGoals, []string{"brief"}, created("2026-08-01T00:00:00Z")),
		document("ops-goals", artifact.KindGoals, []string{"brief"}, created("2026-08-01T00:00:00Z")),
		document("v1-non-goals", artifact.KindNonGoals, []string{"v1-goals"}, created("2026-08-01T00:00:00Z")),
		// A superseded brief states replaced intent and contradicts nothing.
		ended("old-brief", artifact.KindBrief, nil, created("2026-07-01T00:00:00Z"),
			artifact.Revision{Action: artifact.ActionRetired, By: "product-manager", At: moment("2026-07-02T00:00:00Z"), Reason: "replaced"}),
	)
	report := Survey(artifacts, goal.Collect(root, artifacts), nil)

	kinds := map[ContradictionKind]Contradiction{}
	for _, contradiction := range report.Contradictions {
		kinds[contradiction.Kind] = contradiction
	}
	if len(report.Contradictions) != 3 {
		t.Fatalf("contradictions = %+v, want three", report.Contradictions)
	}
	briefs := kinds[ContradictionTwoBriefs]
	if strings.Join(briefs.Documents, "|") != "brief (docs/brief.md)|second-brief (docs/second-brief.md)" {
		t.Fatalf("two briefs named %v", briefs.Documents)
	}
	ruledOut := kinds[ContradictionGoalAndNonGoal]
	if strings.Join(ruledOut.Documents, "|") != "v1-goals (docs/v1-goals.md)|v1-non-goals (docs/v1-non-goals.md)" ||
		ruledOut.Statement != "Offer a hosted control plane." {
		t.Fatalf("goal and non-goal reported as %+v", ruledOut)
	}
	shared := kinds[ContradictionSharedIdentity]
	if len(shared.Documents) != 2 || !strings.Contains(shared.Reason, "[local]") {
		t.Fatalf("shared identity reported as %+v", shared)
	}
	if !report.Anything() {
		t.Fatal("a report carrying contradictions must say it found something")
	}
}

// Documents that agree are not reported, and nothing is refused either way.
func TestDocumentsThatAgreeAreNotReportedAsContradicting(t *testing.T) {
	artifacts := set(document("brief", artifact.KindBrief, nil, created("2026-08-01T00:00:00Z")))
	goals := statedGoals("Run locally.")
	goals.NonGoals = []goal.NonGoal{{Statement: "Replace Git.", ArtifactID: "v1-non-goals", Path: "docs/v1-non-goals.md"}}
	if report := Survey(artifacts, goals, nil); len(report.Contradictions) != 0 {
		t.Fatalf("contradictions = %+v, want none", report.Contradictions)
	}
}
