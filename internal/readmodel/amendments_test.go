package readmodel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/amendment"
	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/sweep"
)

type fakeSweeps struct {
	recorded []runstate.Sweep
	fail     error
}

func (f fakeSweeps) List() ([]runstate.Sweep, []runstate.UnreadableSweep, error) {
	return f.recorded, nil, f.fail
}

// queuedProposal is one undecided change against a design, raised the given
// age before the reading's moment.
func queuedProposal(index int, age time.Duration) amendment.Proposal {
	return amendment.Proposal{
		SchemaVersion: amendment.SchemaVersion,
		ID:            fmt.Sprintf("amendment-%032x", index),
		Role:          domain.RoleDeveloper,
		Agent:         "developer",
		RunID:         fmt.Sprintf("run-%032x", index),
		WorkItemID:    fmt.Sprintf("yoyodyne-ifd.%d", 400+index),
		ProductID:     "yoyodyne",
		RepositoryID:  "yoyodyne",
		Artifact:      "v1-design",
		Kind:          artifact.KindDesign,
		Owner:         domain.RoleArchitect,
		Change:        fmt.Sprintf("change %d", index),
		Why:           fmt.Sprintf("reason %d", index),
		RaisedAt:      moment.Add(-age),
	}
}

// architectPass is one recorded firing of the architect's task carrying her
// recommendations.
func architectPass(startedAt time.Time, recommendations ...sweep.Recommendation) runstate.Sweep {
	return runstate.Sweep{
		SchemaVersion: runstate.SweepSchemaVersion,
		ProductID:     "yoyodyne",
		Task:          "architect-amendments",
		Role:          domain.RoleArchitect,
		StartedAt:     startedAt,
		EndedAt:       startedAt.Add(time.Minute),
		Turns:         1,
		Result:        &sweep.Result{Status: sweep.StatusComplete, Summary: "argued", Recommendations: recommendations},
	}
}

// A queue being worked through names each undecided proposal and nothing
// about its age: it is not waiting on a person beyond the decision each
// already asks for. The counts are still carried, for the week's question.
func TestAYoungQueueIsNamedAndCountedButNotAged(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	fresh := queuedProposal(1, 2*24*time.Hour)
	sources.Amendments = fakeAmendments{records: []amendment.Record{{Proposal: &fresh}}}
	standing := ReadStanding(context.Background(), sources)
	if standing.Amendments.Undecided != 1 || standing.Amendments.OldestAge != 2*24*time.Hour {
		t.Fatalf("Amendments = %+v", standing.Amendments)
	}
	if len(standing.NeedsHuman) != 1 || !strings.Contains(standing.NeedsHuman[0].What(), fresh.ID) {
		t.Fatalf("NeedsHuman = %+v, want the one proposal named and nothing about age", standing.NeedsHuman)
	}
	if strings.Contains(standing.Render(), "proposed change(s) are undecided") {
		t.Fatalf("a young queue is reported as aged:\n%s", standing.Render())
	}
}

// The failure the whole amendment channel has: proposals raised faster than
// anything decides them, which no list of them shows and only the oldest one's
// age does. It is the report pile's line, for the queue.
func TestAQueueNothingIsDrainingIsNamedByItsAge(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	old := queuedProposal(1, 23*24*time.Hour)
	fresh := queuedProposal(2, time.Hour)
	sources.Amendments = fakeAmendments{records: []amendment.Record{{Proposal: &old}, {Proposal: &fresh}}}
	standing := ReadStanding(context.Background(), sources)
	rendered := standing.Render()
	for _, want := range []string{
		"2 of 2 proposed change(s) are undecided, the oldest raised 23d ago, against the architect's documents",
		"the operator's — proposals are decided with `yoyo amendment`",
		"not keeping up, or none is enabled",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered is missing %q:\n%s", want, rendered)
		}
	}
}

// What the owner argued on a recurring pass is the batch the operator decides
// from: one finding for him per pass, named ahead of the proposals it covers,
// carrying the pass's decision list. A proposal decided since is dropped, a
// later pass's recommendation on the same proposal stands over an earlier one,
// and a pass left with nothing undecided makes no finding at all.
func TestTheOwnersRecommendationsAreTheBatchTheOperatorDecides(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	first := queuedProposal(1, 10*24*time.Hour)
	second := queuedProposal(2, 9*24*time.Hour)
	decided := queuedProposal(3, 8*24*time.Hour)
	unargued := queuedProposal(4, 2*24*time.Hour)
	decision, err := decided.Decide(amendment.VerdictApproved, amendment.DeciderOperator, "", moment.Add(-time.Hour))
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	sources.Amendments = fakeAmendments{records: []amendment.Record{
		{Proposal: &first}, {Proposal: &second}, {Proposal: &decided}, {Proposal: &unargued}, {Decision: &decision},
	}}
	sources.Sweeps = fakeSweeps{recorded: []runstate.Sweep{
		architectPass(moment.Add(-2*24*time.Hour),
			sweep.Recommendation{Proposal: first.ID, Verdict: sweep.RecommendDecline, Reason: "no"},
			sweep.Recommendation{Proposal: decided.ID, Verdict: sweep.RecommendApprove, Reason: "yes"},
		),
		architectPass(moment.Add(-6*time.Hour),
			sweep.Recommendation{Proposal: first.ID, Verdict: sweep.RecommendApprove, Reason: "on reflection, yes"},
			sweep.Recommendation{Proposal: second.ID, Verdict: sweep.RecommendMerge, Reason: "the same change", Into: first.ID},
		),
	}}
	standing := ReadStanding(context.Background(), sources)
	if len(standing.RecommendedAmendments) != 2 {
		t.Fatalf("RecommendedAmendments = %+v, want the two undecided ones argued, and the decided one dropped", standing.RecommendedAmendments)
	}
	if got := standing.RecommendedAmendments[0]; got.Proposal != first.ID || got.Verdict != sweep.RecommendApprove || got.Reason != "on reflection, yes" {
		t.Fatalf("first = %+v, want the later pass's recommendation to stand", got)
	}
	if got := standing.RecommendedAmendments[1]; got.Proposal != second.ID || got.Verdict != sweep.RecommendMerge || got.Into != first.ID {
		t.Fatalf("second = %+v, want the merge with what it merges into", got)
	}

	// One finding: the earlier pass argued one proposal that a later pass argued
	// again and one the operator has decided, so it has nothing left to put.
	var batches []OperatorAction
	for _, entry := range standing.NeedsHuman {
		if entry.Kind == AttentionOperatorAction && strings.HasPrefix(entry.ID, AmendmentBatchKeyPrefix) {
			batches = append(batches, *entry.OperatorAction)
		}
	}
	if len(batches) != 1 {
		t.Fatalf("batch findings = %+v, want the later pass's alone", batches)
	}
	batch := batches[0]
	if batch.Key != "amendments:architect-amendments@2026-08-30T06:00:00Z" || !batch.Since.Equal(moment.Add(-6*time.Hour)) {
		t.Fatalf("batch = %+v, want it keyed and dated by the pass that argued it", batch)
	}
	for _, want := range []string{
		"decide " + first.ID + " approve; " + second.ID + " merge into " + first.ID + ".",
		first.ID + " (a change to v1-design): on reflection, yes",
		second.ID + " (a change to v1-design): the same change",
	} {
		if !strings.Contains(batch.Needs, want) {
			t.Fatalf("needs = %q, want it to say %q", batch.Needs, want)
		}
	}
	if strings.Contains(batch.Needs, decided.ID) || strings.Contains(batch.Needs, unargued.ID) {
		t.Fatalf("needs = %q names a proposal outside the batch", batch.Needs)
	}
	rendered := standing.Render()
	for _, want := range []string{
		"the architect's batch of 2 proposed changes needs your hand: decide " + first.ID + " approve;",
		"only a person can act on this; " + amendmentBatchEnds,
		"a change to v1-design is proposed and undecided (" + first.ID + ") — the architect's",
		"a change to v1-design is proposed and undecided (" + unargued.ID + ") — the architect's",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered is missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, decided.ID) {
		t.Fatalf("a decided proposal is still on the line:\n%s", rendered)
	}
	// The batch leads the proposals it covers: it is the entry whose next move
	// is the operator's alone.
	if strings.Index(rendered, "batch of 2") > strings.Index(rendered, "undecided ("+first.ID) {
		t.Fatalf("the batch comes after the proposals it covers:\n%s", rendered)
	}

	// The operator decides both: the batch has nothing left, and its finding
	// ends.
	for _, proposal := range []amendment.Proposal{first, second} {
		made, err := proposal.Decide(amendment.VerdictDeclined, amendment.DeciderOperator, "not now", moment)
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		sources.Amendments = fakeAmendments{records: append(sources.Amendments.(fakeAmendments).records, amendment.Record{Decision: &made})}
	}
	if rendered := ReadStanding(context.Background(), sources).Render(); strings.Contains(rendered, "batch of") {
		t.Fatalf("a batch whose every proposal is decided is still named:\n%s", rendered)
	}
}

// Two passes that each argued something still undecided are two findings,
// oldest pass first, because each is a decision list the operator was put once.
func TestEachPassesBatchIsItsOwnFinding(t *testing.T) {
	t.Parallel()

	first := queuedProposal(1, 5*24*time.Hour)
	second := queuedProposal(2, 4*24*time.Hour)
	records := []amendment.Record{{Proposal: &first}, {Proposal: &second}}
	later := architectPass(moment.Add(-time.Hour), sweep.Recommendation{Proposal: second.ID, Verdict: sweep.RecommendDecline, Reason: "already true"})
	earlier := architectPass(moment.Add(-3*time.Hour), sweep.Recommendation{Proposal: first.ID, Verdict: sweep.RecommendApprove, Reason: "right"})
	actions := AmendmentBatchActions(RecommendedAmendments([]runstate.Sweep{later, earlier}, records))
	if len(actions) != 2 {
		t.Fatalf("actions = %+v, want one per pass", actions)
	}
	if !actions[0].Since.Equal(earlier.StartedAt) || !actions[1].Since.Equal(later.StartedAt) || actions[0].Key == actions[1].Key {
		t.Fatalf("actions = %+v, want the earlier pass first and each keyed apart", actions)
	}
	if actions[0].Subject != "the architect's batch of 1 proposed change" {
		t.Fatalf("subject = %q", actions[0].Subject)
	}
}

// A queue that could not be read is never reported as an empty one, and passes
// that could not be read cost the recommendations and say so, never the queue.
func TestAnUnreadableQueueOrPassLogSaysSo(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	sources.Amendments = fakeAmendments{fail: errors.New("the amendment log is torn")}
	standing := ReadStanding(context.Background(), sources)
	if standing.AmendmentsProblem == "" || !strings.Contains(standing.Render(), "the amendment log is torn") {
		t.Fatalf("problem = %q, rendered:\n%s", standing.AmendmentsProblem, standing.Render())
	}

	sources = quietSources()
	fresh := queuedProposal(1, time.Hour)
	sources.Amendments = fakeAmendments{records: []amendment.Record{{Proposal: &fresh}}}
	sources.Sweeps = fakeSweeps{fail: errors.New("the sweep log is torn")}
	standing = ReadStanding(context.Background(), sources)
	if standing.Amendments.Undecided != 1 || len(standing.NeedsHuman) != 1 {
		t.Fatalf("Amendments = %+v, NeedsHuman = %+v; want the queue read whole", standing.Amendments, standing.NeedsHuman)
	}
	if !strings.Contains(standing.AmendmentsProblem, "the sweep log is torn") || !strings.Contains(standing.Render(), "recommendations may be incomplete") {
		t.Fatalf("problem = %q, rendered:\n%s", standing.AmendmentsProblem, standing.Render())
	}
}

// A recommendation on a pass of a role that does not own the proposal's
// document is not the owner's argument and is never shown as one: the
// proposal stays on the line as undecided, and no batch is said.
func TestANonOwnersRecommendationIsNotShownAsTheOwners(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	pending := queuedProposal(1, 3*24*time.Hour)
	sources.Amendments = fakeAmendments{records: []amendment.Record{{Proposal: &pending}}}
	managerPass := architectPass(moment.Add(-time.Hour), sweep.Recommendation{Proposal: pending.ID, Verdict: sweep.RecommendDecline, Reason: "not mine to say"})
	managerPass.Task, managerPass.Role = "development-manager-sweep", domain.RoleDevelopmentManager
	sources.Sweeps = fakeSweeps{recorded: []runstate.Sweep{managerPass}}
	standing := ReadStanding(context.Background(), sources)
	if len(standing.RecommendedAmendments) != 0 {
		t.Fatalf("RecommendedAmendments = %+v, want none from a non-owner's pass", standing.RecommendedAmendments)
	}
	rendered := standing.Render()
	if strings.Contains(rendered, "batch of") {
		t.Fatalf("a non-owner's recommendation is shown as the owner's:\n%s", rendered)
	}
	if !strings.Contains(rendered, "a change to v1-design is proposed and undecided ("+pending.ID+") — the architect's") {
		t.Fatalf("the proposal is not still named as undecided:\n%s", rendered)
	}
}
