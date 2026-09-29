package admission

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/beads"
)

// The 2026-09-28 shape: the build of a design matched against the closed design
// it was built from. Naming that design sets it aside and leaves nothing else.
func TestAClosedMatchNamedAsDistinctIsSetAside(t *testing.T) {
	t.Parallel()

	admitted := append(backlog(),
		itemUnder("yoyodyne-ifd.434.2", "The architect designs a configurable state root for the harness", "yoyodyne-ifd.434", "closed"),
	)
	matches := Resembling(Candidate{
		Title:  "Build the configurable state root the architect designed for the harness",
		Parent: "yoyodyne-ifd.434",
	}, admitted)
	if got := ids(matches); len(got) != 1 || got[0] != "yoyodyne-ifd.434.2" {
		t.Fatalf("Resembling() = %v, want the closed design matched, so the case is real", got)
	}

	remaining, distinct, err := Distinguish(matches, admitted, "yoyodyne-ifd.434.2")
	if err != nil {
		t.Fatalf("Distinguish() error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("Distinguish() remaining = %v, want nothing left refusing the build", ids(remaining))
	}
	if distinct.ID != "yoyodyne-ifd.434.2" || distinct.Status != "closed" || distinct.Because == "" {
		t.Fatalf("Distinguish() set aside %#v, want the design, its state, and why it matched", distinct)
	}
}

// Open work is acted on, not named past: a distinction against it is refused and
// the match still stands.
func TestAnOpenMatchCannotBeNamedAsDistinct(t *testing.T) {
	t.Parallel()

	admitted := backlog()
	matches := []Match{{ID: "yoyodyne-ifd.241.2", Status: "open", Because: "it is already a child of yoyodyne-ifd.241 carrying this scope"}}
	remaining, distinct, err := Distinguish(matches, admitted, "yoyodyne-ifd.241.2")
	if err == nil || !strings.Contains(err.Error(), "rather than closed") {
		t.Fatalf("Distinguish() error = %v, want the open item refused", err)
	}
	if len(remaining) != 1 || distinct.ID != "" {
		t.Fatalf("Distinguish() = %v, %#v, want the match left standing and nothing set aside", ids(remaining), distinct)
	}
}

// A distinction sets aside only the item it names, so a second match still
// refuses the creation.
func TestADistinctionSetsAsideOnlyTheItemItNames(t *testing.T) {
	t.Parallel()

	matches := []Match{
		{ID: "yoyodyne-ifd.229", Status: "closed", Because: "a"},
		{ID: "yoyodyne-ifd.276", Status: "closed", Because: "b"},
	}
	remaining, _, err := Distinguish(matches, backlog(), "yoyodyne-ifd.229")
	if err != nil {
		t.Fatalf("Distinguish() error = %v", err)
	}
	if got := ids(remaining); len(got) != 1 || got[0] != "yoyodyne-ifd.276" {
		t.Fatalf("Distinguish() remaining = %v, want the other match kept", got)
	}
}

// An item the tracker does not hold is nothing to be distinct from.
func TestADistinctionFromAnItemNobodyHoldsIsRefused(t *testing.T) {
	t.Parallel()

	if _, _, err := Distinguish(nil, backlog(), "yoyodyne-ifd.9999"); err == nil || !strings.Contains(err.Error(), "holds no item") {
		t.Fatalf("Distinguish() error = %v, want the unknown item refused", err)
	}
}

// A closed item the check did not match may still be named; what is set aside
// says it was not a match, so the record does not claim one.
func TestAClosedItemTheCheckDidNotMatchIsRecordedAsUnmatched(t *testing.T) {
	t.Parallel()

	remaining, distinct, err := Distinguish(nil, backlog(), "yoyodyne-ifd.229")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("Distinguish() = %v, %v", ids(remaining), err)
	}
	if distinct.ID != "yoyodyne-ifd.229" || distinct.Because != "" {
		t.Fatalf("Distinguish() set aside %#v, want the item with no match reason", distinct)
	}
}

func itemUnder(id, title, parent, status string) beads.WorkItem {
	return beads.WorkItem{ID: id, Title: title, Parent: parent, Status: status}
}
