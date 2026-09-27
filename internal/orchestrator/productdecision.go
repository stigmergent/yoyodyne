package orchestrator

// A decision the Lead Product Manager made about an item whose run is still in
// flight, docketed for the development manager.
//
// Stopping a run is the development manager's decision, and a run whose item
// the Lead Product Manager has decided is superseded, narrowed, or to be retired
// is exactly the run she might stop. Until this existed the only ways to carry
// that decision to her were a note on the item, which she reads when she decides
// a stoppage on it and never while its run is still going, and a person relaying
// it. So the decision is docketed as it is recorded, and her next sweep or triage
// pass puts it in front of her with where the run now stands.
//
// Two things keep the entry honest after it is written. The run's standing is
// read again from its own record every time the docket is built, because what
// she decides between — stopping it now or letting it finish — turns on how far
// it has got by the time she reads it. And an entry whose run has ended with
// nothing else docketed about it is settled by the build, because there is no
// run left to stop: a run that stopped holding its change is docketed as that
// stoppage, and this entry folds beneath it and is settled by her decision about
// it; a run that finished, or one the operator stopped, leaves nothing for her
// to decide about the run.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// settledProductDecision is the word the harness's own closure of a product
// decision carries, so a reader can tell an entry whose run ended from one the
// development manager decided about.
const settledProductDecision = "settled"

// ProductDecisionDocketer is the part of the docket a product decision is
// recorded through. It needs no triage record and no re-runs, because the entry
// is about a run in flight rather than about anything triage has spent: what the
// item has spent is joined where the docket is built, as it is for every entry.
type ProductDecisionDocketer struct {
	Docket Docket
	Clock  execution.Clock
}

// RecordProductDecision dockets one decision about the item a run in flight was
// made for, and reports whether this call is what created the entry. The same
// decision about the same run already standing undecided is not docketed twice.
//
// The run's record is what the entry is made from, so a run that is not in
// flight is refused: a decision about a run that has ended is a decision about
// its stoppage, or about the item, and neither is this.
func (d ProductDecisionDocketer) RecordProductDecision(state runstate.State, decision triage.ProductDecision) (bool, error) {
	if d.Docket == nil {
		return false, errors.New("a triage docket is required to docket a product decision")
	}
	if !state.Status.InFlight() {
		return false, fmt.Errorf("run %s is not in flight — it ended %s — so there is nothing for the development manager to stop", state.RunID, state.Status)
	}
	now := time.Now()
	if d.Clock != nil {
		now = d.Clock.Now()
	}
	decision.Decision = strings.TrimSpace(decision.Decision)
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.SupersededBy = strings.TrimSpace(decision.SupersededBy)
	joinRunStanding(&decision, state, now)
	entry := triage.Entry{
		SchemaVersion:   triage.SchemaVersion,
		Key:             triage.ProductDecisionKey(state.RunID, decision.Decision),
		Class:           triage.ClassProductDecision,
		ProductID:       state.ProductID,
		RunID:           state.RunID,
		WorkItemID:      state.WorkItemID,
		WorkItemTitle:   state.WorkItemTitle,
		RecordedAt:      now.UTC(),
		ProductDecision: &decision,
		Artifacts:       docketArtifacts(state, triage.Found{}),
	}
	// Nothing was looked for as the entry was written; the build looks for the
	// branch and the worktree every time it is read.
	entry.Artifacts.Found = nil
	if err := entry.Validate(); err != nil {
		return false, fmt.Errorf("docket the product decision about run %s: %w", state.RunID, err)
	}
	return d.Docket.RecordOnce(entry)
}

// joinRunStanding puts where the run stands, as its record says now, onto a
// product decision.
func joinRunStanding(decision *triage.ProductDecision, state runstate.State, at time.Time) {
	decision.RunStatus = string(state.Status)
	decision.RunPhase = string(state.Phase)
	decision.RunInFlight = state.Status.InFlight()
	decision.RunReadAt = at.UTC()
}

// joinProductDecisions reads again where each product decision's run stands, on
// the entries a build hands to a reader, and on what was folded beneath them.
func joinProductDecisions(entries []triage.Entry, byID map[string]runstate.State, at time.Time) {
	for index := range entries {
		joinProductDecision(&entries[index], byID, at)
		for earlier := range entries[index].Earlier {
			joinProductDecision(&entries[index].Earlier[earlier], byID, at)
		}
	}
}

func joinProductDecision(entry *triage.Entry, byID map[string]runstate.State, at time.Time) {
	if entry.ProductDecision == nil {
		return
	}
	state, known := byID[entry.RunID]
	if !known {
		return
	}
	joined := *entry.ProductDecision
	joinRunStanding(&joined, state, at)
	entry.ProductDecision = &joined
}

// settleEndedProductDecisions closes every open product decision whose run is no
// longer in flight and has nothing else open on the docket, and reports how many
// it closed. A run that stopped holding its change has its stoppage open beside
// the product decision, and the two are one question to her: the entry is left
// to fold beneath the stoppage and be settled by her decision about it.
func (d Docketer) settleEndedProductDecisions(byID map[string]runstate.State, now time.Time) (int, error) {
	entries, err := d.Docket.List()
	if err != nil {
		return 0, fmt.Errorf("read the triage docket to settle product decisions whose run has ended: %w", err)
	}
	otherOpen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Class != triage.ClassProductDecision && entry.RunID != "" && entry.Undecided(now) {
			otherOpen[entry.RunID] = true
		}
	}
	closed := 0
	var problems []error
	for _, entry := range entries {
		if entry.Class != triage.ClassProductDecision || !entry.Undecided(now) || otherOpen[entry.RunID] {
			continue
		}
		state, known := byID[entry.RunID]
		if !known || state.Status.InFlight() {
			continue
		}
		took, err := d.Docket.Close(triage.Closure{
			SchemaVersion: triage.ClosureSchemaVersion,
			Key:           entry.Key,
			ProductID:     entry.ProductID,
			RunID:         entry.RunID,
			WorkItemID:    entry.WorkItemID,
			Decision:      settledProductDecision,
			Reason: fmt.Sprintf("run %s ended %s before anybody decided whether to stop it, so there is no run left to stop; the Lead Product Manager's decision stands on the item's notes",
				entry.RunID, state.Status),
			DecidedBy: "the harness, reading the run's record",
			ClosedAt:  now.UTC(),
		})
		if err != nil {
			problems = append(problems, fmt.Errorf("settle the product decision about run %s: %w", entry.RunID, err))
			continue
		}
		if took {
			closed++
		}
	}
	return closed, errors.Join(problems...)
}
