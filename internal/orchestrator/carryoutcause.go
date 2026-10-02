package orchestrator

import (
	"errors"

	"github.com/mason-bryant/yoyodyne/internal/gitworktree"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// PermanentCarryOutError keeps the action's refusal intact while naming why
// another attempt of the same decision cannot succeed.
type PermanentCarryOutError struct {
	Cause triage.CarryOutCause
	Err   error
}

func (e PermanentCarryOutError) Error() string { return e.Err.Error() }
func (e PermanentCarryOutError) Unwrap() error { return e.Err }

func permanentCarryOut(cause triage.CarryOutCause, err error) error {
	return PermanentCarryOutError{Cause: cause, Err: err}
}

func carryOutCause(err error) triage.CarryOutCause {
	var permanent PermanentCarryOutError
	var unmakeable UnrearmablePublicationError
	var missing NoDocketedStoppageError
	switch {
	case errors.As(err, &permanent):
		return permanent.Cause
	case errors.Is(err, gitworktree.ErrOwnedHeadMoved):
		return triage.CarryOutHeadMoved
	case errors.As(err, &unmakeable):
		return triage.CarryOutPublicationUnmakeable
	case errors.Is(err, runstate.ErrRerunTaken):
		return triage.CarryOutDecisionSuperseded
	case errors.As(err, &missing):
		return triage.CarryOutStoppageMissing
	default:
		return ""
	}
}
