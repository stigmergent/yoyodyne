package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEveryTermSaysWhatItMeansAndWhatIsProposed holds the inventory to the
// shape its document promises: a meaning in one sentence, a decision, and the
// words to write instead or the reason it is kept.
func TestEveryTermSaysWhatItMeansAndWhatIsProposed(t *testing.T) {
	seen := make(map[string]bool)
	for _, term := range Inventory {
		if term.Term == "" || len(term.Match) == 0 {
			t.Errorf("%q: a term needs a name and something to match", term.Term)
		}
		if seen[term.Term] {
			t.Errorf("%q is listed twice", term.Term)
		}
		seen[term.Term] = true
		if strings.TrimSpace(term.Meaning) == "" || strings.TrimSpace(term.Words) == "" {
			t.Errorf("%q: a term needs its meaning and its proposal in words", term.Term)
		}
		switch term.Decision {
		case Replace, Register, Keep:
		default:
			t.Errorf("%q: decision %q is not one the document renders", term.Term, term.Decision)
		}
	}
	for _, word := range StopWords {
		if word.Meaning == "" || word.Words == "" {
			t.Errorf("stop word %q needs its meaning and its proposal", word.Word)
		}
	}
}

func TestCountMatchesSpellingsAndLeavesOrdinaryUsesOut(t *testing.T) {
	cases := []struct {
		name string
		term Term
		body string
		want int
	}{
		{"a stem counts its inflections", Term{Match: []string{"docket"}}, "the docket; docketed; Docket entries", 3},
		{"parts match however they are spaced", Term{Match: []string{"side thread"}}, "side thread, side-thread, sidethread, side\nthread", 4},
		{"only a word start matches", Term{Match: []string{"lease"}}, "release the lease", 1},
		{"exact holds the hyphen", Term{Match: []string{"carry-out"}, Exact: true}, "a carry-out; carry out her decision", 1},
		{"whole holds the word end", Term{Match: []string{"gate"}, Whole: true}, "a gate; gates; gated", 1},
		{"an exception at the same place is dropped", Term{Match: []string{"pull"}, Except: []string{"pull request"}}, "the next pull; a pull request; pull-request", 1},
	}
	for _, c := range cases {
		if got := Count(c.term, c.body); got != c.want {
			t.Errorf("%s: counted %d in %q, want %d", c.name, got, c.body, c.want)
		}
	}
}

// TestCollectReadsEachSurfaceAndNothingElse measures a small repository laid
// out the way this one is, so a surface that stopped being read, or a test file
// or a comment that started being counted, fails here.
func TestCollectReadsEachSurfaceAndNothingElse(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/cli/status.go", "package cli\n\n// a stoppage in a comment\nconst key = \"stoppage\"\nconst line = \"one stoppage waits\"\n")
	write("internal/cli/status_test.go", "package cli\n\nconst x = \"a stoppage in a test\"\n")
	write("internal/cli/testdata/fixture.go", "package fixture\n\nconst x = \"a stoppage in test data\"\n")
	write("internal/config/builtin/v1/personas/developer.md", "A stoppage, and another stoppage.\n")
	write("README.md", "One stoppage.\n")
	write("docs/operations.md", "A stoppage here.\n")
	write(OutputPath, "Every stoppage counted by itself would be counted twice.\n")
	write("docs/diagnoses/old.md", "A stoppage in a record.\n")
	write("docs/terms.md", "## The register\n\n| Term | In plain words | Where it is used |\n|---|---|---|\n")
	write("docs/designs/design.md", "---\nid: design\nreason: a stoppage in the frontmatter\n---\n\n# Design\n\nA stoppage in the prose.\n")
	write("docs/product/brief.md", "# Brief\n")
	write("docs/decisions/decision.md", "# Decision\n")

	texts, err := Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	measured := Measure([]Term{{Term: "stoppage", Match: []string{"stoppage"}}}, texts)[0]
	want := [surfaces]int{Printed: 1, Personas: 2, Guides: 2, Governed: 1}
	if measured.Totals != want {
		t.Fatalf("counted %v by surface, want %v; places %v", measured.Totals, want, measured.Places)
	}
}

func TestRenderLinksEveryTermToItsEntry(t *testing.T) {
	var out bytes.Buffer
	measurements := Measure(Inventory[:3], []Text{{Surface: Guides, Where: "docs/work.md", Body: "a stoppage on the docket"}})
	Render(&out, measurements, StopWords, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC))
	document := out.String()
	for _, want := range []string{
		"These were measured on 2026-09-28.",
		"| [`stoppage`](#stoppage) | replace |",
		"### stoppage",
		"- **Where:** 1 in all, in 1 place: `docs/work.md` 1.",
		"| `recording` |",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("the rendered inventory does not say %q", want)
		}
	}
}
