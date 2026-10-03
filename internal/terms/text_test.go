package terms

import (
	"reflect"
	"strings"
	"testing"
)

func TestLiveTextUsesTheRegisterAndItsReplacementWords(t *testing.T) {
	t.Parallel()
	directory := root(t, registerReplacing([]string{
		"| `posture` | tool access | `docs/designs/one.md` |",
		"| `new decoration` | ordinary words | |",
	}, "| `brake` | an automatic stop | command output |"), nil)
	checker, err := ReadTextChecker(directory)
	if err != nil {
		t.Fatal(err)
	}
	found := checker.Find("The brake changed posture; posture is new-decoration on the docket.")
	want := []Finding{
		{Term: "docket", Replacement: "the list of stopped runs waiting on the development manager"},
		{Term: "posture", Replacement: "tool access", Retired: true},
		{Term: "new decoration", Replacement: "ordinary words", Retired: true},
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("Find() = %+v, want %+v", found, want)
	}
	if warning := found[1].Warning(); !strings.Contains(warning, `"posture" was replaced; write tool access`) {
		t.Errorf("Warning() = %q, want the term and the register's replacement", warning)
	}
}

func TestLiveTextKeepsTheDocumentChecksSpellingAndCodeRules(t *testing.T) {
	t.Parallel()
	checker := NewTextChecker(nil, nil)
	for _, text := range []string{"idle bound", "idle-bound", "idlebound", "idle\nbound"} {
		found := checker.Find(text)
		if len(found) != 1 || found[0].Term != "idle bound" {
			t.Errorf("Find(%q) = %+v, want idle bound", text, found)
		}
	}
	for _, text := range []string{"idle\n\nbound", "```text\nidle bound\n```", "hand back", "seamless"} {
		if found := checker.Find(text); len(found) != 0 {
			t.Errorf("Find(%q) = %+v, want no findings", text, found)
		}
	}
	registered := NewTextChecker([]Entry{{Term: "idle bound", PlainWords: "a wait", Used: "a command"}}, nil)
	if found := registered.Find("idlebound"); len(found) != 0 {
		t.Errorf("registered spelling was flagged: %+v", found)
	}
}
