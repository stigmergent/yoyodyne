package execution

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func replyEvent(t *testing.T, payload map[string]any) Event {
	t.Helper()
	event, err := NewEvent("chat-0123456789abcdef0123456789abcdef", 7, time.Unix(0, 0), EventAgentMessage, "provider.test", payload)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	return event
}

func TestAReplyWithinTheBoundIsRecordedWholeAndIsNoCut(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("x", MaxReplyTextBytes)
	payload := ReplyPayload(text)
	if payload["text"] != text || payload["cut"] != nil {
		t.Fatalf("payload = %v, want the text whole and nothing declared", payload["cut"])
	}
	if _, cut := ReplyCutIn(replyEvent(t, payload)); cut {
		t.Fatal("a whole reply read as cut")
	}
}

// Past the bound the cut falls on a rune boundary, is declared in the text where
// it falls, and carries the reply's whole size.
func TestAReplyPastTheBoundIsCutOnARuneAndDeclared(t *testing.T) {
	t.Parallel()

	text := "x" + strings.Repeat("é", MaxReplyTextBytes)
	payload := ReplyPayload(text)
	kept := payload["recorded_bytes"].(int)
	if payload["cut"] != true || payload["whole_bytes"] != len(text) || kept > MaxReplyTextBytes {
		t.Fatalf("payload declares cut %v, %v of %v bytes", payload["cut"], kept, payload["whole_bytes"])
	}
	recorded := payload["text"].(string)
	if !utf8.ValidString(recorded) || !strings.HasPrefix(text, recorded[:kept]) ||
		!strings.HasSuffix(recorded, "of this reply's 262145 bytes]") {
		t.Fatalf("recorded text ends %q", recorded[len(recorded)-80:])
	}
	cut, ok := ReplyCutIn(replyEvent(t, payload))
	if !ok || cut.Sequence != 7 || cut.RecordedBytes != kept || cut.WholeBytes != len(text) || !strings.HasSuffix(text[:kept], cut.EndsWith) {
		t.Fatalf("ReplyCutIn() = %#v, %t", cut, ok)
	}
}

// A reply cut before cuts were declared is still found, with its size unknown.
func TestAReplyCutUnderTheOldBoundIsFound(t *testing.T) {
	t.Parallel()

	cut, ok := ReplyCutIn(replyEvent(t, map[string]any{"text": "the release's notes " + "…[truncated]"}))
	if !ok || cut.WholeBytes != 0 || cut.EndsWith != "the release's notes " {
		t.Fatalf("ReplyCutIn() = %#v, %t", cut, ok)
	}
	if !strings.Contains(cut.Describe(), "a reply of unrecorded size") {
		t.Fatalf("Describe() = %q", cut.Describe())
	}
}
