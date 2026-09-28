package staleness

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/goal"
)

// Two documents of the product's intent that contradict each other.
//
// Every document in the specification home is authoritative — the operator's
// direction of 2026-09-27 — and every role is handed all of them. Two
// authoritative documents saying opposite things is then not a style problem: it
// is intent that tells whoever reads it two things at once, and each role would
// otherwise settle it by whichever document it happened to read last. So it is
// reported here, beside what a change upstream left unanswered, naming both.
//
// What is reported is what the documents' own structure makes readable rather
// than a judgement about their prose. Deciding that two paragraphs mean opposite
// things is a reading only a person or the owning role can make, and a report
// that guessed at it would be one nobody could trust; the three below are true
// by construction whenever they are reported:
//
//   - two active briefs, each saying what the product is;
//   - one statement that one document states as a goal and another rules out as
//     a non-goal, compared by identity where both carry one and otherwise by the
//     words, folded as an attribution by wording is;
//   - one goal identity two documents in force each give to a different goal.
//
// Like everything else here it is reported and never refuses: which document is
// right is the owner's decision, and the contradiction is the evidence for it.

// ContradictionKind names which of the readable contradictions this is.
type ContradictionKind string

const (
	ContradictionTwoBriefs      ContradictionKind = "two-briefs"
	ContradictionGoalAndNonGoal ContradictionKind = "goal-and-non-goal"
	ContradictionSharedIdentity ContradictionKind = "shared-identity"
)

// Contradiction is two documents in force saying opposite things, each named by
// its id and the file to open.
type Contradiction struct {
	Kind ContradictionKind `json:"kind"`
	// Documents names every document the contradiction is between, as "id
	// (path)", in the order the artifact set holds them.
	Documents []string `json:"documents"`
	// Statement is what they disagree about, where that is one statement.
	Statement string `json:"statement,omitempty"`
	Reason    string `json:"reason"`
}

// contradictions reads what the artifacts and the goals make readable. Only
// documents in force are compared: a superseded document states intent that was
// replaced, and one agreeing or disagreeing with it is no longer anybody's
// question.
func contradictions(artifacts artifact.Set, goals goal.Set) []Contradiction {
	var found []Contradiction

	var briefs []string
	for _, recorded := range artifacts.OfKind(artifact.KindBrief) {
		if recorded.InForce() {
			briefs = append(briefs, named(recorded.ID, recorded.Path))
		}
	}
	if len(briefs) > 1 {
		found = append(found, Contradiction{
			Kind:      ContradictionTwoBriefs,
			Documents: briefs,
			Reason: fmt.Sprintf("%d briefs are active — %s — and each says what the product is; the product has one brief, so one of them is to be superseded or retired by its owner",
				len(briefs), strings.Join(briefs, " and ")),
		})
	}

	for _, stated := range goals.Goals {
		if !stated.InForce {
			continue
		}
		for _, ruledOut := range goals.NonGoals {
			if !sameStatement(stated, ruledOut) {
				continue
			}
			documents := []string{named(stated.ArtifactID, stated.Path)}
			if ruledOut.ArtifactID != stated.ArtifactID {
				documents = append(documents, named(ruledOut.ArtifactID, ruledOut.Path))
			}
			found = append(found, Contradiction{
				Kind:      ContradictionGoalAndNonGoal,
				Documents: documents,
				Statement: stated.Statement,
				Reason: fmt.Sprintf("%s states it as a goal and %s rules it out as a non-goal; work may not both serve it and stay out of it",
					documents[0], documents[len(documents)-1]),
			})
		}
	}

	for _, problem := range goals.IdentityProblems {
		if len(problem.Stated) < 2 {
			// One document giving an identity to two of its own goals is a defect
			// in that document, which `yoyo goals list` reports; it is not two
			// documents disagreeing.
			continue
		}
		found = append(found, Contradiction{
			Kind:      ContradictionSharedIdentity,
			Documents: problem.Stated,
			Statement: "[" + problem.Identity + "]",
			Reason: fmt.Sprintf("%s each give the goal identity [%s] to a goal of their own, so work naming it names no one goal",
				strings.Join(problem.Stated, " and "), problem.Identity),
		})
	}
	return found
}

// sameStatement reports a goal and a non-goal stating one thing: by identity
// where both carry one, since that is what the goal is, and otherwise by the
// words as an attribution by wording is matched.
func sameStatement(stated goal.Goal, ruledOut goal.NonGoal) bool {
	if stated.Identity != "" && ruledOut.Identity != "" {
		return stated.Identity == ruledOut.Identity
	}
	return goal.Folded(stated.Statement) == goal.Folded(ruledOut.Statement)
}

func named(id, path string) string {
	return fmt.Sprintf("%s (%s)", id, path)
}
