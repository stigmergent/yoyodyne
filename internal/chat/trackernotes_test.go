package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/goal"
)

func TestReprioritizingRecordsTheOldAndNewPriorityAndWhy(t *testing.T) {
	t.Parallel()

	tracker := &priorityNoteTracker{fakeTracker: &fakeTracker{items: map[string]beads.WorkItem{
		"yoyodyne-ifd.22": {ID: "yoyodyne-ifd.22", Title: "Make the conversation readable", Status: "open", Priority: 3, Notes: "Earlier decision."},
	}}}
	provider := &fakeBackend{results: []backendapi.RunResult{
		{SessionID: "session-1", FinalText: trackerReply("Put the transcript first.",
			`{"action":"reprioritize","id":"yoyodyne-ifd.22","priority":0,"reason":"the transcript prevents the operator following the work"}`)},
		{SessionID: "session-1", FinalText: "The reason is on the item."},
	}}
	options := testOptions(t, provider)
	options.Tracker = tracker
	session := openTestSession(t, options)
	reply, err := session.Send(context.Background(), "Order the transcript work.")
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].Applied || len(tracker.updates) != 1 {
		t.Fatalf("actions = %#v, writes = %#v", reply.Actions, tracker.updates)
	}
	item := tracker.items["yoyodyne-ifd.22"]
	if item.Priority != 0 {
		t.Fatalf("priority = %d, want 0", item.Priority)
	}
	for _, required := range []string{
		"Earlier decision.", "Reprioritized from priority 3 to 0 by the Lead Product Manager",
		session.Evidence().ConversationID, "after turn 1.",
		"Reason: the transcript prevents the operator following the work",
	} {
		if !strings.Contains(item.Notes, required) {
			t.Fatalf("notes = %q, want %q", item.Notes, required)
		}
	}
	// A single update carries both the priority and the appended reason.
	if tracker.updates[0].change.Priority == nil || tracker.updates[0].change.AppendNotes == "" {
		t.Fatalf("write = %#v", tracker.updates[0])
	}
}

func TestReprioritizingRefusesWhenTheOldPriorityCouldNotBeRead(t *testing.T) {
	t.Parallel()

	tracker := &priorityNoteTracker{fakeTracker: &fakeTracker{}, unread: true}
	provider := &fakeBackend{results: []backendapi.RunResult{
		{SessionID: "session-1", FinalText: trackerReply("Put the transcript first.",
			`{"action":"reprioritize","id":"yoyodyne-ifd.22","priority":0,"reason":"it blocks the operator"}`)},
		{SessionID: "session-1", FinalText: "The old priority could not be read."},
	}}
	options := testOptions(t, provider)
	options.Tracker = tracker
	reply, err := openTestSession(t, options).Send(context.Background(), "Order the transcript work.")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.updates) != 0 || len(reply.Actions) != 1 || reply.Actions[0].Applied ||
		!strings.Contains(reply.Actions[0].Failure, "without reading its old priority") {
		t.Fatalf("actions = %#v, writes = %#v", reply.Actions, tracker.updates)
	}
}

type priorityNoteTracker struct {
	*fakeTracker
	unread bool
}

func (f *priorityNoteTracker) Show(ctx context.Context, id string) (beads.WorkItem, error) {
	if f.unread {
		return beads.WorkItem{}, errors.New("the old priority could not be read")
	}
	return f.fakeTracker.Show(ctx, id)
}

func (f *priorityNoteTracker) Update(ctx context.Context, id string, change beads.WorkItemChange) (beads.WorkItem, error) {
	if _, err := f.fakeTracker.Update(ctx, id, change); err != nil {
		return beads.WorkItem{}, err
	}
	f.append(id, change.AppendNotes)
	item := f.items[id]
	if change.Priority != nil {
		item.Priority = *change.Priority
	}
	f.items[id] = item
	return item, nil
}

func TestReadingLongNotesKeepsTheLatestStopAndRecoveryDecisions(t *testing.T) {
	t.Parallel()

	const crossing = "Triaged: the repair grant cap crossed to 2 on the development manager's own authority, which is crossing 1 of 5 for this item, on the stopped work of run run-1 by the development manager in conversation chat-1, after turn 4.\n\nReason: The approved change needs a conflict repair."
	const grant = "Triaged: handed back for one bounded repair of the change it already has, on the stopped work of run run-1 by the development manager in conversation chat-1, after turn 5.\n\nReason: Resolve the conflict on the preserved change.\nKeep both behaviours."
	const continued = "Triaged: the development manager's triage decided a repair of the stopped work of run run-1, recorded by the development manager in conversation chat-1 after turn 5, and the harness re-entered that run's repair loop on the change it already has, under a grant of 2 further repair attempt(s). The reasoning that decision was recorded with: Resolve the conflict on the preserved change."
	const failure = "Failure: verification failed after 4 of 4 permitted attempt(s): make test exited with 2\ncompiler refused the changed package\nCONFLICT (content): preserve both sides"
	const followUp = "Noted by the Lead Product Manager in conversation chat-2, after turn 8.\n\nReason: Account for the previous grant before considering another.\n\nRecovery follow-up: establish execution or refusal; this note grants no recovery budget and starts no run."
	notes := "Yoyodyne stopped this item: old reason superseded.\nRun: run-old\n\n" +
		crossing + "\n" + grant + "\n" + continued + "\n\n" +
		"Yoyodyne blocked this item; the blocker recorded on the item says what stopped it.\nRun: run-1\n" + failure + "\nPhase: checking\nCaptured output:\n" +
		strings.Repeat("check output and diff: 長い出力\n", 700) + "\n" + followUp + "\n\n" +
		"Named as covering report report-1 by the Lead Product Manager in conversation chat-2, after turn 8.\n\nReason: Keep the recovery follow-up on this item."
	if len(notes) < 20<<10 {
		t.Fatal("fixture must reproduce at least twenty kilobytes of notes")
	}
	item := beads.WorkItem{
		ID: "yoyodyne-ifd.435.10", Title: "Exhausted-provider dispatch suppression", Notes: notes,
		Description: strings.Repeat("Standing description. ", 1000),
	}
	rendered := renderWorkItemEvidence(item, goal.Set{})
	for _, required := range []string{crossing, grant, continued, failure, "Run: run-1", followUp,
		"are cut; treat them as unread rather than absent", "check output and other details may be omitted",
		"These notes do not establish execution beyond what they explicitly record"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("read omitted %q:\n%s", required, rendered)
		}
	}
	if strings.Contains(rendered, "old reason superseded") || !utf8.ValidString(rendered) {
		t.Fatalf("read retained an older stop or split a rune:\n%s", rendered)
	}
	if len(rendered) > maxTrackerItemBytes+len(crossing)+len(grant)+len(continued)+len(failure)+1024 {
		t.Fatalf("read carried the output rather than excerpts: %d bytes", len(rendered))
	}
	for _, role := range []domain.AgentRole{domain.RoleProductManager, domain.RoleDevelopmentManager} {
		t.Run(string(role), func(t *testing.T) {
			provider := &fakeBackend{results: []backendapi.RunResult{
				{SessionID: "session-1", FinalText: trackerReply("Reading the recovery history.", `{"action":"read","id":"yoyodyne-ifd.435.10"}`)},
				{SessionID: "session-1", FinalText: "The grant and later stop are recorded."},
			}}
			options := testOptions(t, provider)
			options.Role = role
			options.Agent = string(role)
			options.Tracker = &fakeTracker{items: map[string]beads.WorkItem{item.ID: item}}
			if _, err := openTestSession(t, options).Send(context.Background(), "Account for the existing repair grant."); err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{grant, continued, failure} {
				if !strings.Contains(provider.requests[1].Prompt, required) {
					t.Fatalf("%s was not shown %q", role, required)
				}
			}
		})
	}
}

func TestARecoveryFollowUpDoesNotInventAGrantOrContinuation(t *testing.T) {
	t.Parallel()

	notes := strings.Repeat("old notes\n", 2500) +
		"Noted by the Lead Product Manager in conversation chat-1, after turn 8.\n\nReason: A previous repair grant must be accounted for.\n\nEstablish execution before spending again."
	rendered := renderTrackerNotes(notes, minTrackerNotesBytes)
	if strings.Contains(rendered, "Latest stop and recorded decisions") || strings.Contains(rendered, "harness re-entered") {
		t.Fatalf("a follow-up was used to infer a missing record:\n%s", rendered)
	}
	if !strings.Contains(rendered, "unread rather than absent") || !strings.Contains(rendered, "previous repair grant") {
		t.Fatalf("read lost the follow-up or concealed its cut:\n%s", rendered)
	}
}

func TestTheLatestDecisionOfEachKindSurvivesLaterNotes(t *testing.T) {
	t.Parallel()

	const old = "Triaged: handed back for one bounded repair of the change it already has, on the stopped work of run run-old by the development manager in conversation chat-1, after turn 1.\n\nReason: Superseded repair."
	const latest = "Triaged: handed back for one bounded repair of the change it already has, on the stopped work of run run-new by the development manager in conversation chat-1, after turn 2.\n\nReason: Current repair."
	const priority = "Reprioritized from priority 3 to 0 by the Lead Product Manager in conversation chat-2, after turn 3.\n\nReason: It blocks the operator."
	notes := old + "\n" + latest + "\n" + priority + "\n" +
		"Yoyodyne stopped this item: current stop.\nRun: run-new\nFailing check: make test (exit 2)\nCaptured output:\n" +
		strings.Repeat("large check output\n", 1500)
	rendered := renderTrackerNotes(notes, minTrackerNotesBytes)
	for _, required := range []string{latest, priority, "current stop", "Run: run-new", "Failing check: make test (exit 2)"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("read lost %q:\n%s", required, rendered)
		}
	}
	if strings.Contains(rendered, "Superseded repair") {
		t.Fatalf("read kept an old decision in place of the newest:\n%s", rendered)
	}
}
