package terms

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding is a word in text shown to a person that the register does not
// permit. It is kept beside the author's text, never substituted into it.
type Finding struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement,omitempty"`
	Retired     bool   `json:"retired,omitempty"`
}

func (f Finding) Warning() string {
	reason := "has no definition in " + RegisterPath
	if f.Retired {
		reason = "was replaced"
	}
	if f.Replacement != "" {
		reason += "; write " + f.Replacement
	}
	return fmt.Sprintf("[wording: %q %s]", f.Term, reason)
}

type textRule struct {
	finding Finding
	match   *regexp.Regexp
}

// TextChecker applies the document check's vocabulary and spelling rules to
// live prose. Registered terms are allowed; a replaced row supplies the words
// to write instead, and excuses no live text even when it excuses a document.
type TextChecker struct {
	rules []textRule
}

func ReadTextChecker(root string) (*TextChecker, error) {
	content, err := read(filepath.Join(root, filepath.FromSlash(RegisterPath)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", RegisterPath, err)
	}
	entries := entriesIn(content)
	if problems, _ := checkRegister(entries); len(problems) > 0 {
		return nil, fmt.Errorf("read terms register: %s", problems[0])
	}
	return NewTextChecker(entries, replacementsIn(content)), nil
}

func NewTextChecker(entries []Entry, replacements []Replacement) *TextChecker {
	_, registered := checkRegister(entries)
	replaced := make(map[string]Replacement)
	for _, row := range replacements {
		replaced[row.Term] = row
	}
	checker := &TextChecker{}
	seen := make(map[string]bool)
	add := func(word Coinage, replacement Replacement, retired bool) {
		seen[word.Term] = true
		if registered[word.Term] {
			return
		}
		plain := word.PlainWords
		if retired {
			plain = replacement.PlainWords
		}
		checker.rules = append(checker.rules, textRule{
			finding: Finding{Term: word.Term, Replacement: plain, Retired: retired},
			match:   pattern(word),
		})
	}
	for _, word := range Vocabulary {
		row, retired := replaced[word.Term]
		// The sweep named this phrase without its opening word; the row keeps
		// the whole phrase. Both use the same spelling rule and correction.
		if !retired {
			row, retired = replaced["one "+word.Term]
		}
		if retired {
			seen[row.Term] = true
		}
		add(word, row, retired)
	}
	// A newly retired row is effective without a change to the vocabulary.
	for _, row := range replacements {
		if row.Term != "" && !seen[row.Term] {
			add(Coinage{Term: row.Term, Match: row.Term, Whole: true}, row, true)
		}
	}
	return checker
}

// Find reports each term once. Fenced code and paragraph boundaries are read
// exactly as in documents. A warning already beside text is not checked as
// though the role wrote it, so another surface can render it again safely.
func (c *TextChecker) Find(text string) []Finding {
	if c == nil || text == "" {
		return nil
	}
	for _, rule := range c.rules {
		text = strings.ReplaceAll(text, rule.finding.Warning(), "")
	}
	stretches := passagesFrom(strings.Split(text, "\n"), 1)
	var found []Finding
	for _, rule := range c.rules {
		for _, stretch := range stretches {
			if rule.match.MatchString(stretch.text) {
				found = append(found, rule.finding)
				break
			}
		}
	}
	return found
}

// MergeFindings keeps a pass's corrections small: one entry per term across
// all of its summaries, reports, and turns.
func MergeFindings(prior, found []Finding) []Finding {
	for _, finding := range found {
		seen := false
		for _, earlier := range prior {
			if earlier.Term == finding.Term {
				seen = true
				break
			}
		}
		if !seen {
			prior = append(prior, finding)
		}
	}
	return prior
}
