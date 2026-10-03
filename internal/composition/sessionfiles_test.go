package composition

// The two files every AI session in this repository loads on its own — Claude
// Code reads CLAUDE.md and Codex reads AGENTS.md — are the architect's
// instructions to that session, and one file kept in two places. They have
// drifted before: each carried sections the other did not, and each carried a
// section `bd setup` generated telling every session to use the tracker for
// all its work, beneath a section saying a developer run never runs it
// (yoyodyne-ifd.437.25). Nothing in bd can be told not to write that section,
// so what stops it landing again is this check.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionInstructionFiles are the files a session loads without being asked,
// the first being the one the second is held to.
var sessionInstructionFiles = []string{"CLAUDE.md", "AGENTS.md"}

// generatedTrackerMarkers open the sections `bd setup claude` and
// `bd setup codex` write into those files.
var generatedTrackerMarkers = []string{"BEADS INTEGRATION", "BEADS CODEX SETUP"}

func readSessionInstructions(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return content
}

func TestTheSessionInstructionFilesAreOneFile(t *testing.T) {
	t.Parallel()

	first := sessionInstructionFiles[0]
	want := readSessionInstructions(t, first)
	for _, name := range sessionInstructionFiles[1:] {
		got := readSessionInstructions(t, name)
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from %s; they are one set of instructions kept in two places, so make the change in both, byte for byte (cp -f %s %s after editing %s)", name, first, first, name, first)
		}
	}
}

func TestTheSessionInstructionsCarryNoGeneratedTrackerSection(t *testing.T) {
	t.Parallel()

	for _, name := range sessionInstructionFiles {
		content := string(readSessionInstructions(t, name))
		for _, marker := range generatedTrackerMarkers {
			// The file names the markers in prose to say they were taken out, so
			// what is looked for is the comment that opens one.
			if strings.Contains(content, "<!-- BEGIN "+marker) {
				t.Errorf("%s carries the %q section `bd setup` writes, which tells every session to use bd for all its tracking; remove it — the file's own section on the tracker says why", name, marker)
			}
		}
	}
}

func TestTheSessionInstructionsSayWhoseTheyAre(t *testing.T) {
	t.Parallel()

	// The opening is what a session reads first, so it is where the file says
	// it is the architect's and that product intent is elsewhere.
	content := string(readSessionInstructions(t, sessionInstructionFiles[0]))
	opening, _, _ := strings.Cut(content, "\n## ")
	for _, claim := range []string{"the architect's instructions", "no product intent", "(docs/product/README.md)"} {
		if !strings.Contains(strings.Join(strings.Fields(opening), " "), claim) {
			t.Errorf("%s's opening no longer says %q; it has to say whose instructions these are and link the product home", sessionInstructionFiles[0], claim)
		}
	}
}
