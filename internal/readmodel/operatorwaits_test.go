package readmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// An owed step and a publication are dated from the run's ending, which is
// when each began to wait. Neither is the operator's — the registry gives the
// owed step to the harness and an unmerged publication to the development
// manager — so the development manager's sweep of what waits on him leaves
// them out and shows his own entries, each dated from its record.
func TestOwedStepsAndOperatorPublicationsAreDatedFromTheRunsEnding(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	ended := now.Add(-5 * time.Hour)
	owed := owedStepAttention(runstate.State{RunID: "run-owed", WorkItemID: "yoyodyne-ifd.1", Status: runstate.StatusSucceeded, CompletedAt: &ended})
	if !owed.Since().Equal(ended) {
		t.Errorf("owed step since = %s, want the run's ending %s", owed.Since(), ended)
	}

	promotedEnded := now.Add(-26 * time.Hour)
	publication := awaitingForgeAttention(runstate.State{
		RunID: "run-published", WorkItemID: "yoyodyne-ifd.2", Status: runstate.StatusSucceeded, CompletedAt: &promotedEnded,
		Branch:         "yoyodyne/yoyodyne-ifd-2/abc",
		Integration:    &runstate.Integration{TargetBranch: "main"},
		PullRequest:    &runstate.PullRequest{Number: 42, URL: "https://example.test/pull/42"},
		PublishFailure: "the forge refused the merge request",
	})
	if publication.Mover != MoverDevelopmentManager || owed.Mover != MoverHarness {
		t.Fatalf("publication mover = %s, owed step mover = %s, want the development manager's and the harness's", publication.Mover, owed.Mover)
	}
	if !publication.Since().Equal(promotedEnded) {
		t.Errorf("publication since = %s, want the run's ending %s", publication.Since(), promotedEnded)
	}

	heldAt := now.Add(-3 * time.Hour)
	diverged := now.Add(-26 * time.Hour)
	hold := operatorHoldAttention(runstate.OperatorHold{HeldAt: heldAt})
	stall := divergedTargetAttention(runstate.DivergedTarget{TargetBranch: "main", Since: diverged})
	if !hold.Mover.IsOperator() || !stall.Mover.IsOperator() {
		t.Fatalf("hold mover = %s, diverged target mover = %s, want both the operator's", hold.Mover, stall.Mover)
	}
	rendered := Standing{NeedsHuman: []Attention{owed, publication, hold, stall}}.RenderOperatorWaits(now)
	for _, want := range []string{
		"2 entries on the needs-a-human line are the operator's",
		"[hold operator]",
		"since " + localMoment(heldAt) + ", 3 hours ago",
		"[stall diverged-target:main]",
		"since " + localMoment(diverged) + ", 26 hours ago",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the section does not carry %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{"run-owed", "run-published", "not recorded on it"} {
		if strings.Contains(rendered, unwanted) {
			t.Errorf("the section carries %q:\n%s", unwanted, rendered)
		}
	}
}
