package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The audit reads every log the product holds, including one no record points
// at, and counts both kinds of cut: the marker replies were cut with before
// cuts were declared, and a declared cut.
func TestCutRepliesCountsEveryCutReplyAcrossTheConversations(t *testing.T) {
	t.Parallel()

	store, err := runstate.NewConversationStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatalf("NewConversationStore() error = %v", err)
	}
	const (
		architect = "chat-11111111111111111111111111111111"
		whole     = "chat-22222222222222222222222222222222"
	)
	say := func(conversation string, sequence uint64, payload map[string]any) {
		t.Helper()
		event, err := execution.NewEvent(conversation, sequence, time.Unix(0, 0), execution.EventAgentMessage, "provider.test", payload)
		if err != nil {
			t.Fatalf("NewEvent() error = %v", err)
		}
		if err := store.AppendEvent(event); err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	}
	say(architect, 1, map[string]any{"text": "the release's notes …[truncated]"})
	say(architect, 2, map[string]any{"text": "a whole ruling"})
	say(architect, 3, execution.ReplyPayload(strings.Repeat("r", execution.MaxReplyTextBytes+10)))
	say(whole, 1, map[string]any{"text": "nothing cut here"})

	report, err := readCutReplies(store)
	if err != nil {
		t.Fatalf("readCutReplies() error = %v", err)
	}
	if report.Conversations != 2 || report.Replies != 4 || report.Cut != 2 || len(report.Affected) != 1 {
		t.Fatalf("report = %+v, want 2 of 4 replies cut in one of two conversations", report)
	}
	cuts := report.Affected[0].Cuts
	if report.Affected[0].ConversationID != architect || cuts[0].Sequence != 1 || cuts[1].Sequence != 3 ||
		cuts[1].WholeBytes != execution.MaxReplyTextBytes+10 {
		t.Fatalf("affected = %+v", report.Affected)
	}
	report.Product = "yoyodyne"
	rendered := renderCutReplies(report)
	for _, want := range []string{
		"2 of 4 recorded replies across 2 conversation log(s) for yoyodyne are held cut, in 1 conversation(s).",
		architect + " (no record points at it any more): 2 cut",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered = %q, want %q", rendered, want)
		}
	}
}
