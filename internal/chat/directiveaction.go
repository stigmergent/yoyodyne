package chat

// Ending a directive from the Lead Product Manager's own conversation.
//
// The operator's slash commands resolve and withdraw directives from inside a
// conversation, and `yoyo directive` does it from a terminal, but both are a
// person's hands. On 2026-09-28 the operator asked for four questions and a
// remark that had been recorded as directives to be withdrawn, and for nine
// standing directives to be resolved into the operating-rules document the Lead
// Product Manager was writing, and neither was anything she could do: under his
// rule of 2026-09-26 a step routed to a person is a defect. So she has an action
// for each, recorded on the directive as her act and in her name.
//
// Nothing here widens what a directive is. Withdrawing one is exactly what the
// operator's withdrawal is, with her named as who did it; resolving one into what
// now carries it ends it with that named on the record, and is refused, like a
// withdrawal, on a directive that has already ended.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/directive"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

const (
	// directiveResolve ends a directive by naming the document or work item that
	// now carries it.
	directiveResolve = "resolve"
	// directiveWithdraw takes back a directive that directs nothing: a question or
	// a remark recorded as one.
	directiveWithdraw = "withdraw"
)

// directiveActionProblems checks a directive action as far as the action alone
// can be checked. Whether the directive exists, and whether it is still in force,
// needs the records, so that is asked as the action is carried out.
func (a TrackerAction) directiveActionProblems() []error {
	var problems []error
	switch named := strings.TrimSpace(a.Directive); {
	case named == "":
		problems = append(problems, errors.New("directive requires \"directive\", the directive it ends, named exactly as it was recorded or by any prefix of it that names exactly one"))
	case !directive.ValidReference(named):
		problems = append(problems, fmt.Errorf("directive %q is not a directive identifier; name one exactly as it was recorded, or by any prefix of it that names exactly one", named))
	}
	became := strings.TrimSpace(a.Became)
	switch strings.TrimSpace(a.Decision) {
	case directiveResolve:
		problems = append(problems, boundTrackerText("became", became, maxTrackerTitleBytes, true))
		if strings.ContainsAny(became, "\r\n") {
			problems = append(problems, errors.New("became cannot span lines"))
		}
	case directiveWithdraw:
		// A withdrawal says nobody means the directive, so there is nothing it
		// became; naming something would record a resolution under a withdrawal.
		if became != "" {
			problems = append(problems, errors.New("a directive's \"withdraw\" does not take \"became\"; a directive that became something is resolved, not withdrawn"))
		}
	case "":
		problems = append(problems, fmt.Errorf("directive requires \"decision\", %q or %q", directiveResolve, directiveWithdraw))
	default:
		problems = append(problems, fmt.Errorf("directive decision %q is not a decision; the decisions are %q and %q", a.Decision, directiveResolve, directiveWithdraw))
	}
	// The record holds a reason to a tighter bound than an action's reason, and a
	// reason refused by the record after the action was accepted would be the
	// harness taking a decision it then could not write down.
	if len(strings.TrimSpace(a.Reason)) > directive.MaxWithdrawalBytes {
		problems = append(problems, fmt.Errorf("reason is %d bytes, and a directive records at most %d", len(strings.TrimSpace(a.Reason)), directive.MaxWithdrawalBytes))
	}
	return problems
}

// endDirective resolves or withdraws one directive in this conversation's role's
// name. The record is what refuses one already ended, so the refusal says how it
// ended rather than something this guessed.
func (s *Session) endDirective(ctx context.Context, outcome *TrackerOutcome) {
	action := outcome.Action
	if s.options.Directives == nil {
		outcome.Failure = errNoDirectives.Error()
		return
	}
	reference := strings.TrimSpace(action.Directive)
	reason := strings.TrimSpace(action.Reason)
	by := s.directiveActor()
	switch strings.TrimSpace(action.Decision) {
	case directiveResolve:
		became := strings.TrimSpace(action.Became)
		ended, err := s.options.Directives.ResolveInto(ctx, reference, became, by, s.state.Role, reason)
		if err != nil {
			outcome.Failure = singleLine(err.Error(), maxTrackerFailureBytes)
			return
		}
		if err := s.emit(execution.EventDirectiveResolved, map[string]any{
			"directive_id": ended.ID,
			"became":       ended.Became,
			"resolution":   reason,
			"by":           string(s.state.Role),
		}); err != nil {
			// The directive is ended in the store every run reads; only this
			// conversation's own log is short of it.
			outcome.noteLanded("directive %s is resolved into %s in the product's directive record", ended.ID, ended.Became)
			outcome.fail(fmt.Errorf("record the resolution of directive %s in this conversation's log: %w", ended.ID, err))
			return
		}
		outcome.applied("resolved directive %s (%s) into %s, so it no longer applies as a directive: %s",
			ended.ID, singleLine(ended.Text, maxSurveyTitleBytes), ended.Became, singleLine(reason, maxTrackerFailureBytes))
	case directiveWithdraw:
		ended, err := s.options.Directives.Withdraw(ctx, reference, by, s.state.Role, reason)
		if err != nil {
			outcome.Failure = singleLine(err.Error(), maxTrackerFailureBytes)
			return
		}
		if err := s.emit(execution.EventDirectiveWithdrawn, map[string]any{
			"directive_id": ended.ID,
			"reason":       reason,
			"by":           string(s.state.Role),
		}); err != nil {
			outcome.noteLanded("directive %s is withdrawn in the product's directive record", ended.ID)
			outcome.fail(fmt.Errorf("record the withdrawal of directive %s in this conversation's log: %w", ended.ID, err))
			return
		}
		paused := ""
		if ended.Kind.Pauses() {
			paused = "; the work it paused can carry on, without what it was waiting for having been answered"
		}
		outcome.applied("withdrew directive %s (%s), so it no longer applies: %s%s",
			ended.ID, singleLine(ended.Text, maxSurveyTitleBytes), singleLine(reason, maxTrackerFailureBytes), paused)
	default:
		// Validation admits nothing else.
		outcome.Failure = fmt.Sprintf("the harness does not carry out a directive decision %q", action.Decision)
	}
}

// directiveActor is who a directive ended from this conversation's own tracker
// block is recorded as having been ended by: the conversation's role, not the
// operator, because the role decided it. It names the conversation and the turn,
// as every other durable note this conversation writes does.
func (s *Session) directiveActor() string {
	return fmt.Sprintf("the %s, from conversation %s, after turn %d", RoleTitle(s.state.Role), s.state.ConversationID, s.state.Turns)
}
