package orchestrator

// A stop the development manager decided about a run in flight. It is the
// operator's stop made on her behalf, so the run stops exactly as it does for
// theirs; what differs is who the record names and the docket entry it leaves.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/orchestrator/orchestratortest"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// The decision is recorded while the developer is working, in the shape her
// conversation records it: the stop on the item's triage record, and the request
// beside the run naming her. The run stops at its next boundary with its change
// preserved, frees its slot, says who stopped it and why, and is docketed as a
// stoppage already decided rather than one waiting on her.
func TestAStopTheDevelopmentManagerDecidedEndsTheRunInFlightAsDecided(t *testing.T) {
	t.Parallel()

	const decidedBy = "the development manager in conversation chat-0123456789abcdef"
	const reason = "superseded: the dashboard rewrite does this work (superseded by yoyodyne-ifd.398)"
	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: docketedItem, Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		if _, err := store.Triage().RecordDecision(context.Background(), docketedItem, runstate.TriageDecision{
			Decision:     runstate.TriageDecisionStop,
			RunID:        request.RunID,
			Reason:       "superseded: the dashboard rewrite does this work",
			SupersededBy: "yoyodyne-ifd.398",
			DecidedBy:    "development manager",
			Conversation: "chat-0123456789abcdef",
			Turn:         7,
		}, baseTime); err != nil {
			return err
		}
		if err := store.RecordStop(runstate.StopRequest{
			SchemaVersion: runstate.StopSchemaVersion,
			ProductID:     "yoyodyne",
			RunID:         request.RunID,
			WorkItemID:    docketedItem,
			RequestedAt:   baseTime,
			Reason:        reason,
			RequestedBy:   decidedBy,
			Decision:      runstate.TriageDecisionStop,
		}); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(request.WorkingDirectory, "feature.txt"), []byte("implemented\n"), 0o600)
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)
	docket, err := runstate.NewDocketStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewDocketStore() error = %v", err)
	}
	pipeline.Docket = docketerOverStore(docket, store, pipeline.Config)

	outcome, err := pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil {
		t.Fatal("Run() error = nil, want the stop reported as what ended the run")
	}
	// The stop: the run ended cancelled at the review boundary, before a verdict
	// was bought on a change nobody is going to take, and promoted nothing.
	if !strings.Contains(err.Error(), decidedBy+" stopped this run") || !strings.Contains(err.Error(), reason) {
		t.Fatalf("Run() error = %v, want it to name who stopped the run and why", err)
	}
	if outcome.Status != runstate.StatusCancelled {
		t.Fatalf("status = %q, want the stopped run recorded as cancelled", outcome.Status)
	}
	if reviews := countRoleRequests(provider.Requests, "reviewer"); reviews != 0 {
		t.Fatalf("reviewer invocations = %d, want the stop to have landed before the review", reviews)
	}
	if outcome.Integration != nil || tracker.Closed {
		t.Fatalf("a stopped run promoted its work: %#v (closed=%t)", outcome.Integration, tracker.Closed)
	}

	// The preserved change: the branch and the worktree are where an operator's
	// stop leaves them, and the branch carries what the developer wrote.
	stopped, err := store.Load(outcome.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if stopped.WorktreeRemoved || stopped.BranchRemoved {
		t.Fatalf("stopped state = %#v, want the artifacts preserved", stopped)
	}
	if _, err := os.Stat(filepath.Join(stopped.WorktreePath, "feature.txt")); err != nil {
		t.Fatalf("the stopped run's change did not survive in its worktree: %v", err)
	}
	if listed := gitOutput(t, repository, "branch", "--list", stopped.Branch); !strings.Contains(listed, stopped.Branch) {
		t.Fatalf("branch %s is gone, want it preserved", stopped.Branch)
	}
	if shown := gitOutput(t, repository, "show", stopped.Branch+":feature.txt"); !strings.Contains(shown, "implemented") {
		t.Fatalf("branch %s does not carry the change: %q", stopped.Branch, shown)
	}

	// The freed slot: a terminal record is not in flight, and nothing is left
	// incomplete to hold a developer slot or the race guard.
	if stopped.Status.InFlight() || stopped.CompletedAt == nil {
		t.Fatalf("stopped state = %#v, want a terminal run holding no slot", stopped)
	}
	incomplete, err := store.Incomplete()
	if err != nil {
		t.Fatalf("Incomplete() error = %v", err)
	}
	if len(incomplete) != 0 {
		t.Fatalf("runs in flight = %d, want the stopped run's slot free", len(incomplete))
	}

	// The record: the run and the item both say who stopped it and why.
	if !strings.Contains(stopped.Failure, decidedBy+" stopped this run") || !strings.Contains(stopped.Failure, "yoyodyne-ifd.398") {
		t.Fatalf("run failure = %q, want who stopped it and the superseding item", stopped.Failure)
	}
	if !strings.Contains(tracker.Notes, decidedBy+" stopped this run") || !strings.Contains(tracker.Notes, "superseded by yoyodyne-ifd.398") {
		t.Fatalf("the work item was not told who stopped it and why: %q", tracker.Notes)
	}
	counters, err := store.Triage().Counters(docketedItem)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	decision, decided := counters.DecisionOf(outcome.RunID)
	if !decided || decision.Decision != runstate.TriageDecisionStop || decision.SupersededBy != "yoyodyne-ifd.398" {
		t.Fatalf("decision = %#v (decided=%t), want the stop on the item's triage record", decision, decided)
	}
	if counters.AwaitingCarryOut(outcome.RunID) {
		t.Fatal("the stop reads as a decision the harness has still to carry out")
	}

	// The docket: one entry for the stopped run, closed by her decision, so the
	// docket reads it as decided and puts nothing to her.
	entries, err := docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Class != triage.ClassStoppedRun || entries[0].RunID != outcome.RunID {
		t.Fatalf("docket = %#v, want the stopped run docketed once", entries)
	}
	closure := entries[0].Closed
	if closure == nil || closure.Decision != runstate.TriageDecisionStop || closure.DecidedBy != decidedBy || !strings.Contains(closure.Reason, "yoyodyne-ifd.398") {
		t.Fatalf("closure = %#v, want the entry settled by her stop", closure)
	}
	if !strings.Contains(entries[0].Failure, decidedBy+" stopped this run") {
		t.Fatalf("entry failure = %q, want the entry to say who stopped the run", entries[0].Failure)
	}
	built, err := pipeline.Docket.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(built.Entries) != 0 || built.Closed != 1 {
		t.Fatalf("docket build = %d open, %d closed; want the stop read as decided and nothing awaiting her", len(built.Entries), built.Closed)
	}
}

// The operator's own stop is unchanged by any of this: it names the operator and
// leaves nothing on the docket, since it hands nobody a decision.
func TestAnOperatorStopIsStillDocketedNowhere(t *testing.T) {
	t.Parallel()

	repository, worktreeRoot, store := restartableFixture(t)
	tracker := &orchestratortest.Tracker{Item: beads.WorkItem{ID: docketedItem, Title: "Task", Status: "open"}}
	provider := orchestratortest.RoleBackend(func(request backend.RunRequest) error {
		return store.RecordStop(runstate.StopRequest{
			SchemaVersion: runstate.StopSchemaVersion,
			ProductID:     "yoyodyne",
			RunID:         request.RunID,
			WorkItemID:    docketedItem,
			RequestedAt:   baseTime,
		})
	}, approveVerdict)
	pipeline := automatic(newSharedPipeline(t, repository, worktreeRoot, store, tracker, provider, []string{"exit 0"}), provider)
	docket, err := runstate.NewDocketStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewDocketStore() error = %v", err)
	}
	pipeline.Docket = docketerOverStore(docket, store, pipeline.Config)

	_, err = pipeline.Run(context.Background(), tracker.Item.ID)
	if err == nil || !strings.Contains(err.Error(), "the operator stopped this run") {
		t.Fatalf("Run() error = %v, want the operator named", err)
	}
	entries, err := docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("docket = %#v, want an operator's stop docketed nowhere", entries)
	}
}
