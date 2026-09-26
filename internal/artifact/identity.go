package artifact

// Recording goal identities without amending what the goals say.
//
// A goal's identity is the bracketed identifier its entry opens with —
// `- [traceable-chain] Maintain a traceable chain ...` — and it is what an
// attribution resolves by, so that a re-wording of the sentence orphans nothing.
// Adding one is a change to the file, and every change to one of these files is
// a revision. Recorded as an amendment it made the document read as amended
// since the operator approved it, and under `approvals.work_items: automatic`
// that puts every admission naming one of its goals back to the operator: so
// recording the identifiers without asking would have routed the whole queue to
// a person, and asking was a request to approve a change to nothing they had
// agreed to. yoyodyne-ifd.344's fifteen identifiers waited on exactly that.
//
// So an identity revision is its own action, and this is the only write that
// records one. It is held to the claim it makes: the document it writes has to
// read, line for line, exactly as the one it replaces once the identifiers are
// taken off the entries, and at least one identifier has to have changed. A body
// that moved a single word is an amendment, and is refused here rather than
// recorded as something that leaves the approval standing.
//
// A document edited by hand can record the action too, as it can record any
// revision; the revision log is the file's own account of itself. What this
// write adds is that the harness's path to the action is one that checks.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
)

// entryIdentityPattern finds the identifier a list entry opens with: the list
// marker, then the identifier in square brackets. The identifier's own form is
// the one the goal package reads (internal/goal's identityPattern) — letters,
// digits, and single hyphens between them — so what this treats as an identity
// is what an attribution would resolve by, and a bracketed run of prose is prose.
var entryIdentityPattern = regexp.MustCompile(`(?i)^(\s*[-*+]\s+)\[[a-z0-9]+(?:-[a-z0-9]+)*\]\s*`)

// withoutEntryIdentity is one line with the identifier its entry opens with
// taken off, and the line unchanged when it opens with none.
func withoutEntryIdentity(line string) string {
	match := entryIdentityPattern.FindStringSubmatchIndex(line)
	if match == nil {
		return line
	}
	return line[match[2]:match[3]] + line[match[1]:]
}

// identityOnly reports why a replacement body is not an identity revision of the
// body it replaces, and nothing when it is one: every line reads the same once
// entry identifiers are taken off, and at least one line differs as written.
func identityOnly(before, after string) error {
	previous := strings.Split(strings.TrimSpace(before), "\n")
	replacement := strings.Split(strings.TrimSpace(after), "\n")
	if len(previous) != len(replacement) {
		return fmt.Errorf("the replacement has %d lines and the document has %d; an identity revision changes the identifiers goal entries open with and not a line of anything else", len(replacement), len(previous))
	}
	changed := false
	for index := range previous {
		if previous[index] == replacement[index] {
			continue
		}
		if withoutEntryIdentity(previous[index]) != withoutEntryIdentity(replacement[index]) {
			return fmt.Errorf("line %d changes more than a goal's identifier: %q becomes %q; changing what a goal says is an amendment", index+1, previous[index], replacement[index])
		}
		changed = true
	}
	if !changed {
		return fmt.Errorf("the replacement changes no goal's identifier")
	}
	return nil
}

// Identify records a revision of a goals document that changes only the
// identities its goals carry: the bracketed identifier an entry opens with, added,
// changed, or taken off. Only the role that owns the document may, the same as
// any other change to it. The operator's approval is not asked for and is not
// moved: the revision is recorded as ActionIdentified, which ApprovalState reads
// past and staleness does not report, because not a word of what anybody
// approved has changed.
//
// The body is the whole document below the frontmatter as it should now read.
// It is refused unless it differs from the current one in entry identifiers
// alone.
func (s Store) Identify(role domain.AgentRole, id, body, reason string, now time.Time) (Artifact, error) {
	existing, current, err := s.loadOne(id)
	if err != nil {
		return Artifact{}, err
	}
	if err := Authorize(role, existing.Kind); err != nil {
		return Artifact{}, err
	}
	if existing.Kind != KindGoals {
		return Artifact{}, fmt.Errorf("artifact %q is a %s artifact; goal identities are stated in a goals document, so an identity revision is recorded only against one", id, existing.Kind)
	}
	if ended, hasEnding := existing.Ended(); hasEnding {
		return Artifact{}, fmt.Errorf("artifact %q was %s on %s and is not revised afterwards: %s",
			id, ended.Action, ended.At.Format(time.RFC3339), ended.Reason)
	}
	if err := identityOnly(current, body); err != nil {
		return Artifact{}, fmt.Errorf("artifact %q is not identity-revised: %w", id, err)
	}
	identified := existing
	identified.Revisions = append(append([]Revision(nil), existing.Revisions...), Revision{
		Action: ActionIdentified,
		By:     role,
		At:     now.UTC(),
		Reason: strings.TrimSpace(reason),
	})
	if err := identified.Validate(); err != nil {
		return Artifact{}, err
	}
	if err := s.write(identified.Path, identified, "\n"+strings.TrimSpace(body)+"\n"); err != nil {
		return Artifact{}, err
	}
	return identified, nil
}
