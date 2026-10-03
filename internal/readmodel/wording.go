package readmodel

import (
	"errors"
	"os"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/sweep"
	"github.com/mason-bryant/yoyodyne/internal/terms"
)

// TextTerms is the register used for one rendering. A project with no register
// has no language check; a register that exists but cannot be read says so.
type TextTerms struct {
	checker *terms.TextChecker
	problem string
}

func ReadTextTerms(repository string) *TextTerms {
	if repository == "" {
		return nil
	}
	checker, err := terms.ReadTextChecker(repository)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &TextTerms{problem: "the wording could not be checked: " + err.Error()}
	}
	return &TextTerms{checker: checker}
}

func (t *TextTerms) Find(text string) []terms.Finding {
	if t == nil {
		return nil
	}
	return t.checker.Find(text)
}

// Render flags the words beside the text, preserving the author's words and
// any trailing newline. It is also safe when Slack receives an already flagged
// read-model sentence: each warning appears only once.
func (t *TextTerms) Render(text string) string {
	if t == nil || text == "" {
		return text
	}
	warnings := t.Warnings(text)
	if len(warnings) == 0 {
		return text
	}
	body := strings.TrimRight(text, "\n")
	return body + " " + strings.Join(warnings, " ") + text[len(body):]
}

// Warnings is also used beside a reply whose prose was streamed before its
// ending. Checking the whole reply keeps wrapped words subject to the same
// rule as prose shown all at once.
func (t *TextTerms) Warnings(text string) []string {
	if t == nil {
		return nil
	}
	var warnings []string
	if t.problem != "" && !strings.Contains(text, t.problem) {
		warnings = append(warnings, "[wording: "+t.problem+"]")
	}
	for _, finding := range t.Find(text) {
		if warning := finding.Warning(); !strings.Contains(text, warning) {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func (t *TextTerms) LaneReport(content runstate.LaneReportContent) []terms.Finding {
	found := t.Find(content.Summary)
	for _, text := range content.Remaining {
		found = terms.MergeFindings(found, t.Find(text))
	}
	for _, blocker := range content.Blockers {
		found = terms.MergeFindings(found, t.Find(blocker.What))
	}
	return found
}

func (t *TextTerms) Pass(result *sweep.Result) []terms.Finding {
	if result == nil {
		return nil
	}
	found := t.Find(result.Summary)
	for _, question := range result.Questions {
		found = terms.MergeFindings(found, t.Find(question))
	}
	for _, finding := range result.Findings {
		found = terms.MergeFindings(found, t.Find(finding.Issue))
		found = terms.MergeFindings(found, t.Find(finding.Detail))
	}
	for _, recommendation := range result.Recommendations {
		found = terms.MergeFindings(found, t.Find(recommendation.Reason))
	}
	return found
}

func wordingProgramManager(instance ProgramManager, words *TextTerms) ProgramManager {
	blockers := append([]ProgramManagerBlocker{}, instance.Blockers...)
	for index := range blockers {
		blockers[index].What = words.Render(blockers[index].What)
	}
	claims := append([]ProgramManagerClaim{}, instance.Claims...)
	for index := range claims {
		claims[index].What = words.Render(claims[index].What)
	}
	instance.Blockers, instance.Claims = blockers, claims
	if instance.MissedPass != nil {
		miss := *instance.MissedPass
		miss.Says = words.Render(miss.Says)
		instance.MissedPass = &miss
	}
	return instance
}
