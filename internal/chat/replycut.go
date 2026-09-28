package chat

// A reply the conversation's record could not hold whole.
//
// Until a role can write the document it owns, the conversation's event log is
// where its ruling lives, and a reply is recorded there under a bound
// (execution.MaxReplyTextBytes). A reply past it is kept up to the bound and the
// cut is declared on the event, at the point it falls and with the reply's whole
// size. Declaring it is half of it: on 2026-09-26 two of the architect's rulings
// were found cut in her log by a person transcribing them, and the role that
// wrote them was never told. So the turn after a cut opens by saying which reply
// was cut and where the record stops, and the operator is told as the reply is
// shown. docs/diagnoses/yoyodyne-ifd-430-20-replies-cut-in-the-record.md is the
// whole of it.

import (
	"fmt"
	"io"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// renderReplyCuts is what opens the turn after one whose reply was recorded
// cut, and nothing where none was.
func renderReplyCuts(cuts []execution.ReplyCut) string {
	if len(cuts) == 0 {
		return ""
	}
	var rendered strings.Builder
	rendered.WriteString("# Your last reply was recorded cut\n\n")
	rendered.WriteString("The conversation's durable record holds each reply up to a bound, and your last reply went past it, so the record keeps only its beginning. The operator was shown the whole of it, but anything past the cut exists nowhere durable: a ruling or decision there is lost unless you restate it. Restate what matters of what came after the point below, briefly, before anything else you say.\n\n")
	for _, cut := range cuts {
		rendered.WriteString("- " + cut.Describe() + "\n")
	}
	rendered.WriteString("\n")
	return rendered.String()
}

// RenderRecordCuts is what the operator is told about a reply they were just
// shown whole and the record holds only part of: one line per cut, and nothing
// where the record holds all of it.
func RenderRecordCuts(cuts []execution.ReplyCut) string {
	var rendered strings.Builder
	for _, cut := range cuts {
		fmt.Fprintf(&rendered, "[record] %s; the role is told on its next turn so it can restate the rest.\n", cut.Describe())
	}
	return rendered.String()
}

// reportRecordCuts writes RenderRecordCuts into the conversation.
func reportRecordCuts(out io.Writer, reply Reply) {
	if rendered := RenderRecordCuts(reply.RecordCuts); rendered != "" {
		fmt.Fprint(out, rendered)
		fmt.Fprintln(out)
	}
}
