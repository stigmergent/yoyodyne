package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The yoyodyne-ifd.78 shape, 2026-09-28: the development manager escalated a
// stopped run to the operator, and the Lead Product Manager then parked the
// item. The sweep tells the item once that the escalation has ended and why,
// records the ending on the run, and the operator's line no longer names it —
// the channel included, which reads only the run's record.
func TestTheSweepEndsAnEscalationWhoseItemWasParked(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)
	stopped := now.Add(-48 * time.Hour)
	store, err := runstate.NewStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("runstate.NewStore() error = %v", err)
	}
	run := runstate.State{
		SchemaVersion: runstate.StateSchemaVersion,
		RunID:         "run-95b34031a1b2c3d4e5f60718293a4b5c",
		ProductID:     "yoyodyne",
		RepositoryID:  "yoyodyne",
		WorkItemID:    "yoyodyne-ifd.78",
		WorkItemTitle: "Contributor mode",
		Backend:       domain.BackendClaudeCode,
		Status:        runstate.StatusFailed,
		StartedAt:     stopped.Add(-time.Hour),
		UpdatedAt:     stopped,
		CompletedAt:   &stopped,
		Blocker:       "Yoyodyne stopped this item: the change was empty.",
	}
	if err := store.Create(run); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	decisions := decidedItems{"yoyodyne-ifd.78": {Decisions: []runstate.TriageDecision{{
		Decision: runstate.TriageDecisionEscalate, RunID: run.RunID,
		Reason:    "only the operator can say what this epic is for",
		DecidedBy: "development-manager", Conversation: "chat-dm", Turn: 7, DecidedAt: stopped.Add(time.Hour),
	}}}}

	// Escalated and not yet parked: it stands, and the sweep ends nothing.
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: "yoyodyne-ifd.78", Title: "Contributor mode", Status: "blocked"}}
	reconciler := Reconciler{
		Tracker: tracker,
		Store:   store,
		Docket:  &Docketer{Decisions: decisions},
		Clock:   fixedClock{at: now},
	}
	ended, err := reconciler.EndEscalations(context.Background())
	if err != nil || len(ended) != 0 || len(tracker.NoteRecords) != 0 {
		t.Fatalf("EndEscalations() = %#v, %v with notes %q, want nothing ended while the escalation stands", ended, err, tracker.NoteRecords)
	}

	// Parked: the sweep tells the item what ended it and records it on the run.
	tracker.Item.Parking = domain.WorkItemParking("waits on the machine-home design")
	ended, err = reconciler.EndEscalations(context.Background())
	if err != nil || len(ended) != 1 || ended[0].Failure != "" {
		t.Fatalf("EndEscalations() = %#v, %v, want the one escalation ended", ended, err)
	}
	if len(tracker.NoteRecords) != 1 {
		t.Fatalf("notes = %q, want the item told once", tracker.NoteRecords)
	}
	for _, want := range []string{
		"The development manager's escalation of run run-95b34031a1b2c3d4e5f60718293a4b5c to the operator has ended",
		"yoyodyne-ifd.78 was parked, so nothing about it waits on the operator: waits on the machine-home design",
		"no longer name it as needing the operator's hand",
	} {
		if !strings.Contains(tracker.NoteRecords[0], want) {
			t.Fatalf("note = %q, want it to say %q", tracker.NoteRecords[0], want)
		}
	}
	recorded, err := store.Load(run.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if recorded.EscalationEnded == nil || !strings.Contains(recorded.EscalationEnded.Why, "was parked") || !recorded.EscalationEnded.At.Equal(now) {
		t.Fatalf("run record = %#v, want the ending recorded on it", recorded.EscalationEnded)
	}

	// Said once: a second sweep tells the item nothing more.
	if ended, err = reconciler.EndEscalations(context.Background()); err != nil || len(ended) != 0 || len(tracker.NoteRecords) != 1 {
		t.Fatalf("second EndEscalations() = %#v, %v with notes %q, want nothing said again", ended, err, tracker.NoteRecords)
	}

	// The channel's reading, which asks neither the tracker nor the repository,
	// reads the recorded ending and names nothing.
	states, err := store.Recorded()
	if err != nil {
		t.Fatalf("Recorded() error = %v", err)
	}
	if standing, _ := readmodel.EscalatedOperatorActions(states, decisions, nil, nil); len(standing) != 0 {
		t.Fatalf("EscalatedOperatorActions() = %#v, want the ended escalation named nowhere", standing)
	}
}
