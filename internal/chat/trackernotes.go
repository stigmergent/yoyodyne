package chat

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// renderTrackerNotes keeps the continuous tail and, outside its byte bound,
// quotes the latest stop and each kind of recorded decision that the tail lost.
// A blocker can carry pages of check output after its reason; a later report
// mapping can follow a repair grant. Neither makes those records dispensable.
// These are extracts of notes, not evidence minted from their words: in
// particular, a follow-up asking about a continuation proves no execution.
func renderTrackerNotes(notes string, budget int) string {
	if len(notes) <= budget {
		return notes
	}
	cut := len(notes) - budget
	for cut < len(notes) && !utf8.RuneStart(notes[cut]) {
		cut++
	}
	type record struct {
		start int
		end   int
		kind  string
	}
	var records []record
	offset := 0
	for _, line := range strings.SplitAfter(notes, "\n") {
		if kind, boundary := trackerNoteKind(strings.TrimSuffix(line, "\n")); boundary {
			if len(records) > 0 {
				records[len(records)-1].end = offset
			}
			records = append(records, record{start: offset, end: len(notes), kind: kind})
		}
		offset += len(line)
	}
	latest := make(map[string]int)
	for index, record := range records {
		if record.kind != "" {
			latest[record.kind] = index
		}
	}
	var kept []int
	for _, index := range latest {
		if records[index].start < cut {
			kept = append(kept, index)
		}
	}
	slices.Sort(kept)
	var rendered strings.Builder
	if len(kept) > 0 {
		rendered.WriteString("Latest stop and recorded decisions from the cut notes (verbatim extracts, in note order):\n")
		for _, index := range kept {
			record := records[index]
			text := strings.TrimSpace(notes[record.start:record.end])
			if record.kind == "stop" {
				text = trackerStopExcerpt(text)
			}
			fmt.Fprintf(&rendered, "\n[notes bytes %d–%d]\n%s\n", record.start, record.end, text)
		}
		rendered.WriteString("\nOnly the latest record of each kind is extracted above; check output and other details may be omitted. These notes do not establish execution beyond what they explicitly record.\n\nContinuous end of notes:\n")
	}
	rendered.WriteString(boundTextTail(notes, budget))
	return rendered.String()
}

// trackerNoteKind recognizes the sentences the harness writes, including the
// older notes already in the tracker. Every harness note starts a new record;
// a report mapping or a recovery follow-up gets its own kind and cannot replace
// the triage decision or the harness's account of carrying it out.
func trackerNoteKind(line string) (kind string, boundary bool) {
	switch {
	case strings.HasPrefix(line, "Yoyodyne "):
		if strings.HasPrefix(line, "Yoyodyne stopped ") || strings.HasPrefix(line, "Yoyodyne blocked ") ||
			strings.HasPrefix(line, "Yoyodyne paused ") || strings.HasPrefix(line, "Yoyodyne parked ") ||
			strings.HasPrefix(line, "Yoyodyne bootstrap run failed") || strings.HasPrefix(line, "Yoyodyne run failed") ||
			strings.HasPrefix(line, "Yoyodyne run raised ") {
			return "stop", true
		}
		return "", true
	case strings.HasPrefix(line, "Triaged: the development manager's triage decided "),
		strings.HasPrefix(line, "Continued at its checks:"),
		strings.HasPrefix(line, "The harness did not continue "):
		return "continuation", true
	case strings.HasPrefix(line, "Triaged: the ") && strings.Contains(line, " cap crossed to "):
		cap, _, _ := strings.Cut(line, " cap crossed to ")
		return cap, true
	}
	for _, verb := range triageVerbs {
		if strings.HasPrefix(line, verb+", ") {
			return verb, true
		}
	}
	if strings.Contains(line, " by the ") && strings.Contains(line, " in conversation ") {
		// The first word is stable across values: a new priority, label, or report
		// identifier supersedes the previous note of that kind.
		verb, _, _ := strings.Cut(line, " ")
		_, by, _ := strings.Cut(line, " by the ")
		role, _, _ := strings.Cut(by, " in conversation ")
		return verb + " by the " + role, true
	}
	return "", false
}

// trackerStopExcerpt quotes the reason and the run it is about, keeping a
// multiline failure intact. Output, diffs, and review findings are left to the
// continuous notes view, whose cut is declared separately.
func trackerStopExcerpt(note string) string {
	lines := strings.Split(note, "\n")
	kept := []string{lines[0]}
	keeping := false
	for _, line := range lines[1:] {
		if label, _, field := strings.Cut(line, ":"); field {
			switch label {
			case "Run", "Failure", "Phase", "Round", "Repair attempts", "Failing check", "Waiting out", "Asks again by", "Directive", "Refused paths":
				keeping = true
			case "Captured output", "Changes when the run ended", "Review summary":
				return strings.TrimSpace(strings.Join(kept, "\n"))
			case "Branch", "Worktree", "Base commit", "Claude session", "Developer model", "Developer effort",
				"Reviewer session", "Reviewer model", "Reviewed against", "Review decision", "Invariants delivered",
				"Pull request", "Pull request merged", "Protected by this project", "Granted by this work item", "Role definitions":
				keeping = false
			}
		}
		if keeping {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
