package chat

// Taking back a proposal this conversation made.
//
// A proposal waits on the operator until he decides it, and before this nothing
// else could end one. That was harmless while every proposal was a question only
// he could answer, and stopped being harmless once a guard's fallback could make
// one: on 2026-09-28 the duplicate guard refused the build of the configurable
// state root against the closed design it was built from, its fallback proposed
// the work, and proposal 959.1 sat on the operator's list asking him to approve
// an admission the Lead Product Manager's own authority covered. An approval
// routed to him for a decision somebody else may make is a defect, and she had no
// way to take it back.
//
// A withdrawal is not a decline. A decline is the operator's answer and keeps his
// words; a withdrawal is the proposer taking the question away before he answers,
// and keeps hers. Both leave the proposal in the conversation's record, because
// what was proposed and what became of it are evidence either way.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

// proposalReference is the shape a proposal is named by: the turn it was made
// on and its position in that turn, as every card and listing shows it.
var proposalReference = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// proposalReferenceProblems checks a withdrawal names a proposal the way the
// proposal was listed. Whether this conversation holds it undecided needs the
// conversation, so that is asked as the withdrawal is carried out.
func proposalReferenceProblems(named string) []error {
	switch trimmed := strings.TrimSpace(named); {
	case trimmed == "":
		return []error{errors.New("withdraw requires \"proposal\", the proposal it takes back, named exactly as it was listed, such as \"959.1\"")}
	case !proposalReference.MatchString(trimmed):
		return []error{fmt.Errorf("withdraw proposal %q is not a proposal identifier; a proposal is named by its turn and position, such as \"959.1\"", trimmed)}
	}
	return nil
}

// withdrawal is what the record keeps about a proposal its proposer took back:
// the proposal itself, why, and which role took it back.
type withdrawal struct {
	PendingProposal
	Reason string           `json:"reason"`
	By     domain.AgentRole `json:"by"`
}

// withdrawProposal takes one undecided proposal of this conversation off the
// operator's list. The withdrawal is recorded before the proposal stops being
// pending, so the record never shows a proposal gone with nothing saying why.
func (s *Session) withdrawProposal(outcome *TrackerOutcome) {
	action := outcome.Action
	record, err := s.awaitingDecision(action.Proposal)
	if err != nil {
		outcome.Failure = singleLine(err.Error(), maxTrackerFailureBytes) + "; only a proposal this conversation made and nobody has decided can be withdrawn"
		return
	}
	reason := strings.TrimSpace(action.Reason)
	if err := s.emit(execution.EventProposalWithdrawn, withdrawal{
		PendingProposal: record.pending,
		Reason:          reason,
		By:              s.state.Role,
	}); err != nil {
		outcome.fail(fmt.Errorf("record the withdrawal of proposal %s: %w", record.pending.ID, err))
		return
	}
	record.decided = true
	if err := s.record(); err != nil {
		// The withdrawal is on the log and this process no longer offers the
		// proposal, but the conversation's record may still list it for a later
		// process. That is neither failed nor done, so it is said as unfinished.
		outcome.noteLanded("the withdrawal of proposal %s is recorded in the conversation's log", record.pending.ID)
		outcome.noteUnknown("whether the conversation's record stopped listing proposal %s as awaiting the operator", record.pending.ID)
		outcome.fail(fmt.Errorf("record the conversation after withdrawing proposal %s: %w", record.pending.ID, err))
		return
	}
	outcome.applied("withdrew proposal %s (%s), so it is no longer awaiting the operator: %s",
		record.pending.ID, singleLine(record.pending.Proposal.Title, maxSurveyTitleBytes), singleLine(reason, maxTrackerFailureBytes))
}
