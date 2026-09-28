package config

// The comments the template writes that say something a project needs to know,
// compared against what the project's own file carries.
//
// A comment is not a value: nothing loads it, and it is not in the baseline the
// values are compared against. But a comment can go wrong the way a value can.
// The one above product.specifications said until 2026-09-27 that only the
// product manager reads that directory, and the operator's direction that day —
// "everything in docs/product is authoritative", read by every role — made that
// false in every project that already had a copy. A copy nobody is told about is
// a sentence in the project's own file stating the opposite of what runs.
//
// So a tracked comment is compared against the template's own record of what it
// has written there, rather than against a baseline: every wording it ever
// shipped is listed below, which is a record rather than a guess, and it is what
// lets a project that predates any baseline — this repository among them — be
// told too. A copy carrying one of those earlier wordings was never edited and is
// offered the current one; a copy carrying anything else, or no comment at all,
// is the project's own and is never offered anything, which is the safe
// direction the value comparison takes as well.

import (
	"os"
	"regexp"
	"strings"
)

// SpecificationsCommentKey names the comment above product.specifications. It is
// not a key any configuration has, and it reads as `.comment` for the reason a
// persona's text reads as `.text`: it names what is written beside a setting
// rather than a setting.
const SpecificationsCommentKey = "product.specifications.comment"

// specificationsComment is what the template writes above product.specifications,
// one line to an entry, without the indentation and the "# " each is written with.
var specificationsComment = []string{
	"Every role reads the documents under this directory as authoritative",
	"product intent: every document filed here, not only the brief and the goals.",
	"The management conversations are briefed with all of them, and every",
	"developer and reviewer is handed them beside the work item it is given. A",
	"README here is read as the directory index it is, and what it says about",
	"ownership as a rule. Beside them, and labeled as a description of what is",
	"built rather than as intent, the Lead Product Manager is also given the",
	"README, a fixed set of operator-facing documents under docs/, and the help",
	"the commands print -- not this file, not the source, not the design",
	"document. It must stay inside the repository.",
}

// earlierSpecificationsComments is every wording the template wrote there before
// the current one, oldest first. A wording is added here, and never removed,
// whenever the one above changes.
var earlierSpecificationsComments = [][]string{
	{
		"The product manager reads product intent from the specifications under this",
		"directory and from nowhere else in the repository. Beside them, labeled as a",
		"description of what is built rather than as intent, it is given the README,",
		"the configuration guide, and the help the commands print -- not this file,",
		"not the source, not the design document. It must stay inside the repository.",
	},
	{
		"The product manager reads product intent from the specifications under this",
		"directory and from nowhere else in the repository. Beside them, labeled as a",
		"description of what is built rather than as intent, it is given the README,",
		"a fixed set of operator-facing documents under docs/, and the help the",
		"commands print -- not this file, not the source, not the design document. It",
		"must stay inside the repository.",
	},
	{
		"The Lead Product Manager reads product intent from the specifications under",
		"this directory and from nowhere else in the repository. Beside them, labeled",
		"as a description of what is built rather than as intent, it is given the",
		"README, a fixed set of operator-facing documents under docs/, and the help",
		"the commands print -- not this file, not the source, not the design",
		"document. It must stay inside the repository.",
	},
}

// renderSpecificationsComment is the comment as the template writes it, inside
// the product block.
func renderSpecificationsComment() string {
	var rendered strings.Builder
	for _, line := range specificationsComment {
		rendered.WriteString("  # " + line + "\n")
	}
	return rendered.String()
}

// CompareComments compares every tracked comment in the configuration file at
// configPath against the template. It needs no baseline, so it answers for a
// project that has none. A file that cannot be read compares nothing.
func CompareComments(configPath string) []Value {
	if configPath == "" {
		return nil
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	yours, _ := commentAbove(string(content), "product", "specifications")
	return []Value{compareComment(SpecificationsCommentKey, yours, specificationsComment, earlierSpecificationsComments)}
}

// compareComment classifies one comment. Its baseline is the earlier wording the
// project still carries, where it carries one, and otherwise the wording the
// template wrote last before the current one.
func compareComment(key string, yours []string, current []string, earlier [][]string) Value {
	value := Value{Key: key, Yours: joinComment(yours), Bundle: joinComment(current)}
	switch {
	case value.Yours == value.Bundle:
		value.Class, value.Baseline = ClassUnchanged, value.Bundle
	case wroteEarlier(value.Yours, earlier):
		value.Class, value.Baseline = ClassAvailable, value.Yours
	default:
		value.Class = ClassYours
		if len(earlier) > 0 {
			value.Baseline = joinComment(earlier[len(earlier)-1])
		}
	}
	return value
}

func wroteEarlier(yours string, earlier [][]string) bool {
	for _, wording := range earlier {
		if yours == joinComment(wording) {
			return true
		}
	}
	return false
}

// joinComment is a comment as it is compared and shown: its lines joined with
// single spaces, so how the file happens to wrap it is not a difference.
func joinComment(lines []string) string {
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

var (
	topLevelKeyPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*):`)
	commentLinePattern = regexp.MustCompile(`^\s*#\s?(.*)$`)
)

// commentAbove returns the comment lines written directly above one key of a
// top-level block, with their "#" stripped, and whether the key was found. Only
// the unbroken run of comment lines immediately above the key counts: a blank
// line or another key ends it, which is where a comment about something else
// begins.
func commentAbove(content, block, key string) ([]string, bool) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inBlock := false
	keyPattern := regexp.MustCompile(`^\s+` + regexp.QuoteMeta(key) + `:`)
	for index, line := range lines {
		if match := topLevelKeyPattern.FindStringSubmatch(line); match != nil {
			inBlock = match[1] == block
			continue
		}
		if !inBlock || !keyPattern.MatchString(line) {
			continue
		}
		var comment []string
		for above := index - 1; above >= 0; above-- {
			match := commentLinePattern.FindStringSubmatch(lines[above])
			if match == nil {
				break
			}
			comment = append([]string{strings.TrimSpace(match[1])}, comment...)
		}
		return comment, true
	}
	return nil, false
}
