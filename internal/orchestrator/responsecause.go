package orchestrator

import (
	"fmt"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
)

func (a *activeRun) responseCause(role domain.AgentRole, detail string, from, to time.Time) (string, error) {
	if a.pipeline.Availability == nil {
		return detail, nil
	}
	if from.IsZero() {
		from = a.state.UpdatedAt
	}
	if to.IsZero() {
		to = a.pipeline.clock().Now()
	}
	observed := a.pipeline.Availability(from, to, "")
	why := observed.Why
	if why == "" {
		why = "no machine sleep or harness downtime was established during this response; its cause is unavailable"
	}
	if observed.Problem != "" {
		why += "; machine observations incomplete: " + observed.Problem
	}
	account := "observations during the stopped response: " + why
	// A response that is relaunched still needs an account of its interruption.
	// Keep it in the event stream even when a later attempt completes the run.
	event, err := execution.NewEvent(a.state.RunID, a.state.LastSequence+1, a.pipeline.clock().Now(), execution.EventProcessOutput, "harness", map[string]any{
		"role": role, "text": boundedFailureDetail(account), "from": from, "to": to,
	})
	if err == nil {
		err = a.sink(event)
	}
	if err != nil {
		return detail + "; " + account, fmt.Errorf("record observations of the stopped response: %w", err)
	}
	return detail + "; " + account, nil
}
