package readmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// An owed step and a publication named as the operator's are dated from the
// run's ending, which is when each began to wait, so the development manager's
// sweep shows how long they have waited rather than that nothing says. They
// were two of the kinds her sweep could not see before it carried his line.
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
	if publication.Mover != MoverOperator {
		t.Fatalf("publication mover = %s, want the operator's for this test to cover the case", publication.Mover)
	}
	if !publication.Since().Equal(promotedEnded) {
		t.Errorf("publication since = %s, want the run's ending %s", publication.Since(), promotedEnded)
	}

	rendered := Standing{NeedsHuman: []Attention{owed, publication}}.RenderOperatorWaits(now)
	for _, want := range []string{
		"[owed-step run-owed, item yoyodyne-ifd.1]",
		"since " + localMoment(ended) + ", 5 hours ago",
		"[publication run-published, item yoyodyne-ifd.2]",
		"since " + localMoment(promotedEnded) + ", 26 hours ago",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the section does not carry %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "not recorded on it") {
		t.Errorf("an entry was rendered with no moment:\n%s", rendered)
	}
}
