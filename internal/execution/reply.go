package execution

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxReplyTextBytes bounds the text one agent.message or operator.message event
// carries, which is the durable record of what either side of a conversation
// said. It is its own bound rather than MaxEventTextBytes because a reply is the
// one piece of text whose loss is a lost decision: until a role can write the
// document it owns, the conversation's record is where its ruling lives, and at
// 16 KiB the architect's rulings on yoyodyne-ifd.375 and 432.7 were cut off
// before their landing text (docs/diagnoses/yoyodyne-ifd-430-20-replies-cut-in-the-record.md).
//
// 128 KiB is as far as it can go and still be held whole by every reader of the
// log: an event line is read back under a 1 MiB bound, and JSON can spend six
// bytes on one byte of text (`<` is written `<`), so a reply of this size
// encodes within the line whatever it says.
const MaxReplyTextBytes = 128 << 10

// replyCutMarker ends the text of a reply the record could not hold whole. It
// says where the cut falls and how large the reply was, in the text itself, so a
// reader of the text alone — the rebuild a fresh session is handed, a person
// reading the log — is never shown part of a reply as the whole of it.
const replyCutMarker = "…[cut here: the record holds the first %d of this reply's %d bytes]"

// legacyTruncationMarker is what TruncateEventText ended a reply with before a
// reply had its own bound. A reply ending in it was cut at 16 KiB and its whole
// size was never recorded.
const legacyTruncationMarker = "…[truncated]"

// ReplyCut is a reply the record holds only the beginning of: the event it is
// recorded in, how much of it the record holds, and how large it was. It is the
// harness's account of the cut, carried on the event itself and on the
// conversation so the role is told on its next turn.
type ReplyCut struct {
	// Sequence is the agent.message event holding what was kept.
	Sequence uint64 `json:"sequence"`
	// RecordedBytes is how much of the reply the record holds, before the marker.
	RecordedBytes int `json:"recorded_bytes"`
	// WholeBytes is how large the reply was, and zero where a reply cut before
	// the size was recorded never said.
	WholeBytes int `json:"whole_bytes,omitempty"`
	// EndsWith is the last of what the record holds, so whoever restates the
	// reply knows where the record stops.
	EndsWith string `json:"ends_with"`
}

// replyCutTailBytes is how much of the kept text a ReplyCut quotes.
const replyCutTailBytes = 120

// ReplyPayload is the payload of an agent.message or operator.message event
// carrying text: the text whole where MaxReplyTextBytes allows, and otherwise
// the part of it that fits, cut on a rune boundary, with the cut declared twice —
// in the text at the point it falls, and as fields a reader does not have to
// parse prose for.
func ReplyPayload(text string) map[string]any {
	if len(text) <= MaxReplyTextBytes {
		return map[string]any{"text": text}
	}
	cut := MaxReplyTextBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return map[string]any{
		"text":           text[:cut] + fmt.Sprintf(replyCutMarker, cut, len(text)),
		"cut":            true,
		"recorded_bytes": cut,
		"whole_bytes":    len(text),
	}
}

// ReplyCutIn reports whether one recorded agent.message event holds only part of
// a reply: declared as cut by ReplyPayload, or ending in the marker replies were
// cut with before they were declared. Any other event, and a reply recorded
// whole, is not a cut.
func ReplyCutIn(event Event) (ReplyCut, bool) {
	if event.Type != EventAgentMessage || len(event.Payload) == 0 {
		return ReplyCut{}, false
	}
	var payload struct {
		Text          string `json:"text"`
		Cut           bool   `json:"cut"`
		RecordedBytes int    `json:"recorded_bytes"`
		WholeBytes    int    `json:"whole_bytes"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return ReplyCut{}, false
	}
	kept := payload.Text
	switch {
	case payload.Cut:
		if payload.RecordedBytes > 0 && payload.RecordedBytes <= len(kept) {
			kept = kept[:payload.RecordedBytes]
		}
	case strings.HasSuffix(kept, legacyTruncationMarker):
		kept = strings.TrimSuffix(kept, legacyTruncationMarker)
	default:
		return ReplyCut{}, false
	}
	return ReplyCut{
		Sequence:      event.Sequence,
		RecordedBytes: len(kept),
		WholeBytes:    payload.WholeBytes,
		EndsWith:      tailOf(kept, replyCutTailBytes),
	}, true
}

// tailOf is the last limit bytes of text, starting on a rune boundary.
func tailOf(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	start := len(text) - limit
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}

// Describe is the cut in one sentence, as the transcript and the role are told.
func (c ReplyCut) Describe() string {
	size := "a reply of unrecorded size"
	if c.WholeBytes > 0 {
		size = fmt.Sprintf("a reply of %d bytes", c.WholeBytes)
	}
	return fmt.Sprintf("%s is recorded cut: event %d holds its first %d bytes, ending %q",
		size, c.Sequence, c.RecordedBytes, c.EndsWith)
}
