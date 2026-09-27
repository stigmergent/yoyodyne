package chat

// The Lead Product Manager deciding about an item whose run is still in flight.
//
// On 2026-09-27 she decided that yoyodyne-ifd.428.34, with a developer run in
// flight, was superseded by yoyodyne-ifd.398 and that the run should be stopped
// rather than merged. Stopping a run is the development manager's decision, and
// nothing carried hers there: a note on the item is read when a stoppage on it is
// decided and never while its run is still going, and the ask channel decides
// nothing. The operator's assistant relayed it by hand, which by the operator's
// rule of 2026-09-26 is a defect.
//
// So the decision is an action. She records that the item is superseded,
// narrowed, or to be retired, with her reason and the item doing the work
// instead, and the harness dockets it for the development manager at once. Her
// next sweep or triage pass puts it in front of her with where the run stands,
// and she decides whether the run stops — "stop", carried out as any stop she
// decides — or finishes — "proceed". Nothing here stops the run and nothing here
// changes the item's status: which of the two the run gets is hers, and retiring
// the item stays the Lead Product Manager's, once the run has stopped or
// finished.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// actionInFlight records what the Lead Product Manager decided about an item
// whose run is in flight, for the development manager to decide the run from.
const actionInFlight = "inflight"

// InFlightDecisions is how a product decision about work in flight reaches the
// development manager's docket. It is optional like the rest, and a conversation
// without one refuses the action rather than recording a decision nobody will be
// handed.
type InFlightDecisions interface {
	// InFlightRun reports the run in flight for a work item, and whether there is
	// one. A decision about work in flight names no run: which run is going is the
	// harness's own record to say, not a word the role typed.
	InFlightRun(ctx context.Context, workItemID string) (string, bool, error)
	// Docket records the decision on the development manager's docket, and
	// reports whether this is what created the entry: the same decision about the
	// same run already standing undecided is not docketed twice.
	Docket(ctx context.Context, decision InFlightDecision) (bool, error)
}

// InFlightDecision is one decision about work in flight, as the harness dockets
// it.
type InFlightDecision struct {
	WorkItemID string
	RunID      string
	Decision   triage.ProductDecision
}

// inFlightProblems checks the decision as far as one action can: a decision from
// the vocabulary, and the item doing the work instead named on a supersession and
// never as the item itself.
func (a TrackerAction) inFlightProblems() []error {
	var problems []error
	switch decision := strings.TrimSpace(a.Decision); {
	case decision == "":
		problems = append(problems, fmt.Errorf("inflight requires \"decision\", one of %s", strings.Join(triage.ProductDecisionVocabulary(), ", ")))
	case !slices.Contains(triage.ProductDecisionVocabulary(), decision):
		problems = append(problems, fmt.Errorf("inflight decision %q is not a decision; the decisions are %s",
			decision, strings.Join(triage.ProductDecisionVocabulary(), ", ")))
	}
	switch superseded := strings.TrimSpace(a.SupersededBy); {
	case superseded == "":
		if strings.TrimSpace(a.Decision) == triage.ProductSuperseded {
			problems = append(problems, errors.New("a supersession names the item doing the work instead in \"superseded_by\""))
		}
	case superseded == strings.TrimSpace(a.ID):
		problems = append(problems, errors.New("an item is not superseded by itself; name the item doing the work instead"))
	default:
		if err := beads.ValidateIssueID(superseded); err != nil {
			problems = append(problems, fmt.Errorf("inflight superseded_by: %w", err))
		}
	}
	return problems
}

// carryOutInFlightDecision dockets the decision and writes it onto the item.
//
// The docket comes first, for the reason a stop's request comes before the stop
// is recorded: whichever write fails must leave the record saying no more than
// happened. A note saying the development manager has been handed a decision
// nobody docketed is a relay nobody made; an entry docketed whose note then
// failed still reaches her, and the note is settled against the item.
func (s *Session) carryOutInFlightDecision(ctx context.Context, outcome *TrackerOutcome) {
	action := outcome.Action
	id := strings.TrimSpace(action.ID)
	if s.options.InFlight == nil {
		outcome.fail(errors.New("no way to docket a decision about work in flight is wired to this conversation, so nothing was recorded"))
		return
	}
	runID, found, err := s.options.InFlight.InFlightRun(ctx, id)
	if err != nil {
		outcome.fail(fmt.Errorf("whether a run is in flight on %s could not be read, so nothing was recorded: %w", id, err))
		return
	}
	if !found {
		outcome.refused(fmt.Errorf("no run is in flight on %s, so there is no run for the development manager to stop and nothing was recorded; a decision about the item itself is yours to act on directly — retire, park, or update it", id))
		return
	}
	decision := triage.ProductDecision{
		Decision:     strings.TrimSpace(action.Decision),
		Reason:       strings.TrimSpace(action.Reason),
		SupersededBy: strings.TrimSpace(action.SupersededBy),
		DecidedBy:    fmt.Sprintf("the %s in conversation %s", RoleTitle(s.state.Role), s.state.ConversationID),
		Conversation: s.state.ConversationID,
		Turn:         s.state.Turns,
	}
	created, err := s.options.InFlight.Docket(ctx, InFlightDecision{WorkItemID: id, RunID: runID, Decision: decision})
	if err != nil {
		outcome.fail(fmt.Errorf("the decision about run %s on %s could not be docketed for the development manager, so nothing was recorded: %w", runID, id, err))
		return
	}
	docketed := "docketed it for the development manager"
	if !created {
		docketed = "the same decision about that run already stands on the development manager's docket undecided, so it was not docketed twice"
	}
	outcome.noteLanded("run %s on %s: %s", runID, id, docketed)
	note := s.trackerProvenance(inFlightVerb(decision)+", while run "+runID+" is in flight; docketed for the development manager to decide whether the run stops or finishes", action.Reason)
	if _, err := s.options.Tracker.Update(ctx, id, beads.WorkItemChange{AppendNotes: note}); err != nil {
		outcome.fail(err)
		s.settleTrackerNote(ctx, outcome, id, note, "the decision recorded on the item")
		return
	}
	outcome.applied("recorded that %s while run %s is in flight, and %s; she decides whether the run stops with its change preserved or finishes, and the item's status is unchanged until you retire or update it",
		decision.Says(id), runID, docketed)
}

// inFlightVerb is what the decision records about itself on the work item.
func inFlightVerb(decision triage.ProductDecision) string {
	switch decision.Decision {
	case triage.ProductSuperseded:
		return "Decided superseded by " + decision.SupersededBy
	case triage.ProductNarrowed:
		return "Decided narrowed"
	case triage.ProductRetired:
		return "Decided to be retired without being done"
	default:
		return "Decided " + decision.Decision
	}
}
