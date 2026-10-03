package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// An interrupted wait leaves the shared record alone. Callers wrapping a
// provider turn must preserve that rule when recording their own outcome.
type interruptedProviderWaitError struct{ cause error }

func (e *interruptedProviderWaitError) Error() string { return e.cause.Error() }
func (e *interruptedProviderWaitError) Unwrap() error { return e.cause }

// waitForProvider puts the conversation down while no provider is answering
// this turn. The refused attempt is already recorded. Once the wait ends, the
// hold and the latest durable record are taken back before anything is sent or
// written again. An interrupted wait writes nothing to the shared record.
func (s *Session) waitForProvider(ctx context.Context, duration time.Duration, reason string) (changed bool, err error) {
	if s.options.Hold == nil {
		return false, s.options.sleep(ctx, duration)
	}
	defer func() {
		if err != nil {
			err = &interruptedProviderWaitError{cause: err}
		}
	}()
	if waits, ok := s.options.Hold.(interface {
		Waiting(string, string, time.Time) (func() error, error)
	}); ok {
		clear, recordErr := waits.Waiting(s.state.ConversationID,
			execution.NewRedactor(s.options.RedactValues...).Redact(reason), s.options.clock().Now())
		if recordErr != nil {
			return false, recordErr
		}
		defer func() { err = errors.Join(err, clear()) }()
	}
	before := s.state
	if err := s.releaseHold(); err != nil {
		return false, err
	}
	if err := s.options.sleep(ctx, duration); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.retakeHold(ctx); err != nil {
		return false, err
	}
	if err := s.reload(); err != nil {
		return false, err
	}
	return before.LastSequence != s.state.LastSequence || before.Turns != s.state.Turns ||
		before.ProviderSessionID != s.state.ProviderSessionID || !before.UpdatedAt.Equal(s.state.UpdatedAt), nil
}

// A provider outage has no quoted reset. Probe it on the same interval and
// within the same total waiting budget as a usage window.
func (s *Session) waitOutProviderOutage(ctx context.Context, outage backend.ProviderOutage) (bool, error) {
	probe := s.options.UsageLimitUnknownResetPause
	if s.usageLimitWaited+probe > s.options.UsageLimitPause.bound() {
		return false, fmt.Errorf("%w: waiting %s to ask again would exceed this message's %s waiting budget",
			ErrProviderAway, probe, s.options.UsageLimitPause.bound())
	}
	s.usageLimitWaited += probe
	reason := runstate.DescribeProviderOutage(outage.Cause) + "; asking again at " +
		s.options.clock().Now().Add(probe).Local().Format("2006-01-02 15:04:05 MST")
	s.activity.doing(reason)
	return s.waitForProvider(ctx, probe, reason)
}
