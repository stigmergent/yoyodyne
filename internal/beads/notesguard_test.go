package beads

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/goal"
)

// The command that destroyed twelve attributions, refused before it runs. The
// spellings include the ones the diagnosis found in the session transcripts:
// the flag written with an `=`, the flag written as two words, and the whole
// thing behind a `cd` into the repository.
func TestTheWholesaleNotesWriterIsRefusedHoweverItIsSpelled(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		command string
	}{
		{"the flag joined to its value", `bd update yoyodyne-ifd.45 --notes="Fresh evidence 2026-08-18: a turn failed outright."`},
		{"the flag and its value as two words", `bd update yoyodyne-ifd.45 --notes 'Fresh evidence.'`},
		{"reached through a path rather than the PATH", `/opt/homebrew/bin/bd update yoyodyne-ifd.45 --notes=replaced`},
		{"behind a change of directory", `cd /Users/mbryant/github/yoyodyne && bd update yoyodyne-ifd.45 --notes="replaced"`},
		{"an attribution behind a change of directory", `cd repo && bd update yoyodyne-ifd.45 --notes="Goal served: Run development nearly autonomously."`},
		{"after a command that succeeded", "bd show yoyodyne-ifd.45; bd update yoyodyne-ifd.45 --notes=replaced"},
		{"an invented attribution after a quoted example", `echo 'bd update x --notes=y'; bd update yoyodyne-ifd.45 --notes 'Goal served: Something nobody attributed this to.'`},
		{"the flag named before the item", `bd update --notes="replaced" yoyodyne-ifd.45`},
		{"replacing the notes with nothing at all", `bd update yoyodyne-ifd.45 --notes=""`},
		{"the flag with no value", `bd update yoyodyne-ifd.45 --notes`},
		{"the prefix with no statement after it", `bd update yoyodyne-ifd.45 --notes="Goal served:"`},
		{"spread across a continued line", "bd update yoyodyne-ifd.45 \\\n  --notes='replaced'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			refusal := DestroyedAttribution(test.command)
			if refusal == "" {
				t.Fatalf("DestroyedAttribution(%q) allowed the writer", test.command)
			}
			// The refusal has to be actionable on its own: it names the item at
			// stake and the flag that does the same job without destroying
			// anything. An agent that is only told "no" runs the same command with
			// the quoting changed.
			if !strings.Contains(refusal, "yoyodyne-ifd.45") || !strings.Contains(refusal, appendNotesFlag) {
				t.Fatalf("DestroyedAttribution(%q) = %q, want it to name the item and %s", test.command, refusal, appendNotesFlag)
			}
		})
	}
}

// Preserving a goal line is not preserving every earlier note. Replacements
// are refused regardless of what the command claims the item's goal was.
func TestAReplacementIsRefusedRegardlessOfItsGoalLine(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		notes string
	}{
		{"a genuine-looking attribution", "Admitted by the product manager.\n\n" + goal.Note("Run development nearly autonomously.")},
		{"an invented attribution", goal.Note("Something nobody ever attributed this to.")},
		{"an empty goal line", "Goal served:"},
		{"no goal line", "Fresh evidence."},
		{"empty notes", ""},
	} {
		for _, flag := range []string{notesFlag + "=", notesFlag + " "} {
			t.Run(test.name+"/"+flag, func(t *testing.T) {
				t.Parallel()
				command := "bd update yoyodyne-ifd.45 " + flag + `"` + test.notes + `"`
				if refusal := DestroyedAttribution(command); refusal == "" {
					t.Fatalf("DestroyedAttribution(%q) allowed a wholesale replacement", command)
				}
			})
		}
	}
}

// A correction adds a note rather than replacing the record. The refusal must
// direct the writer to that operation without teaching a replacement escape.
func TestTheRefusalDirectsCorrectionsToAppendWithoutAReplacementEscape(t *testing.T) {
	t.Parallel()

	refusal := DestroyedAttribution(`bd update yoyodyne-ifd.45 --notes="replaced"`)
	for _, stated := range []string{"append-only", "correction", appendNotesFlag} {
		if !strings.Contains(refusal, stated) {
			t.Fatalf("the refusal does not explain appending corrections: %q is missing from %q", stated, refusal)
		}
	}
	for _, escape := range []string{"bd show", "this will allow it", "carry the item's own"} {
		if strings.Contains(refusal, escape) {
			t.Fatalf("the refusal still teaches a replacement escape: %q is in %q", escape, refusal)
		}
	}
}

// Everything else an agent runs passes without a word. A guard that had an
// opinion about ordinary commands would be one somebody turns off, and the
// append spelling -- the one every refusal sends the writer to -- must never be
// read as the spelling being refused.
func TestOrdinaryCommandsAndTheAppendingWriterPassUnremarked(t *testing.T) {
	t.Parallel()

	for _, allowed := range []string{
		`bd update yoyodyne-ifd.45 --append-notes="what I did"`,
		`bd update yoyodyne-ifd.45 --append-notes 'Correction: preserve the earlier notes.'`,
		`bd update yoyodyne-ifd.45 --append-notes="bd update x --notes=y is what broke it"`,
		// Not this rule's to refuse: it destroys no attribution. The status
		// guard beside this one is what refuses it, for what it leaves unsaid.
		`bd update yoyodyne-ifd.45 --status=open`,
		`bd create --title="A new item" --notes="Goal served: nothing yet"`,
		`bd create --title="A new item" --notes 'Goal served: nothing yet'`,
		`bd show yoyodyne-ifd.45 --json`,
		`git commit -m "bd update x --notes=y is what broke it"`,
		`echo 'bd update x --notes=y'`,
		"go test ./...",
		"",
	} {
		if refusal := DestroyedAttribution(allowed); refusal != "" {
			t.Fatalf("DestroyedAttribution(%q) refused an ordinary command: %s", allowed, refusal)
		}
	}
}

// `bd create` is deliberately absent from the refusals above and asserted here:
// it writes notes onto an item that does not exist yet, so there is no
// attribution for it to destroy. Refusing it would refuse the harness's own
// admission path, which is how work acquires a goal in the first place.
func TestCreatingAnItemWithNotesIsNotAReplacement(t *testing.T) {
	t.Parallel()

	if refusal := DestroyedAttribution(`bd create --type=task --title="A new item" --notes="provenance"`); refusal != "" {
		t.Fatalf("creating an item was refused: %s", refusal)
	}
}
