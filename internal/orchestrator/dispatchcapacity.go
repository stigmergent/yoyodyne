package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
)

// dispatchCapacity holds only work whose configured endpoints are exhausted.
// It leaves the item in the backlog at its existing priority and never claims it.
func (s Scheduler) dispatchCapacity(ctx context.Context, schedule *Schedule, pull Pull, item beads.WorkItem) (string, bool) {
	if pull.DispatchEndpoints == nil || pull.UsageLimits == nil {
		return "", false
	}
	endpoints, err := pull.DispatchEndpoints(item)
	if err != nil {
		return fmt.Sprintf("the configured provider endpoints could not be read: %v", err), true
	}
	refusals, err := pull.UsageLimits.List()
	if err != nil {
		schedule.UsageWindowProblem = err.Error()
		return "provider capacity could not be read: " + err.Error(), true
	}
	refusals = readmodel.MergeCapacityRefusals(refusals, pull.CapacityHistory)
	evidence, problem := readmodel.ReadCapacityEvidence(pull.CapacityServed, nil)
	if problem != "" {
		schedule.UsageWindowProblem = problem
	}
	var probes map[string]time.Time
	if pull.CapacityProbes != nil {
		probes, err = pull.CapacityProbes.Next()
		if err != nil {
			schedule.UsageWindowProblem = err.Error()
			return "provider capacity probe times could not be read: " + err.Error(), true
		}
	}
	interval := pull.OutageProbe
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	var waits []readmodel.EndpointCapacity
	for _, endpoint := range endpoints {
		waiting := readmodel.ReadEndpointCapacity(endpoint, refusals, evidence, s.now(), interval, probes)
		if !waiting.Waiting {
			return "", false
		}
		waits = append(waits, waiting)
	}
	// There is no reason to spend a probe while a different configured endpoint
	// can already serve the work. With every endpoint held, ask one due scope.
	for index, waiting := range waits {
		if s.Budget > 0 && (schedule.SpentUSD >= s.Budget || schedule.SpendProblem != "") {
			break
		}
		if s.now().Before(waiting.NextProbeAt) || pull.CapacityProbes == nil || pull.ProbeCapacity == nil {
			continue
		}
		key := waiting.Refusal.CapacityKey(waiting.Endpoint.Provider, waiting.Endpoint.AccountAlias, waiting.Endpoint.Model)
		next, claimed, err := pull.CapacityProbes.Claim(ctx, key, s.now(), interval)
		if err != nil {
			schedule.UsageWindowProblem = err.Error()
			break
		}
		waits[index].NextProbeAt = next
		if !claimed {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		spent, probeErr := pull.ProbeCapacity(probeCtx, waiting.Endpoint)
		err = probeErr
		cancel()
		if spent != nil {
			if spent.Known() {
				schedule.SpentUSD += spent.AmountUSD
			} else if s.Budget > 0 {
				schedule.SpendProblem = "the bounded provider capacity probe's cost is unknown"
			}
		}
		if err != nil {
			schedule.UsageWindowProblem = fmt.Sprintf("bounded provider capacity probe: %v", err)
		}
		if s.Budget > 0 && (schedule.SpentUSD >= s.Budget || schedule.SpendProblem != "") {
			break
		}
		// The durable served record is the proof, even when the callback returned
		// success. A callback alone cannot release dispatch.
		refusals, readErr := pull.UsageLimits.List()
		refusals = readmodel.MergeCapacityRefusals(refusals, pull.CapacityHistory)
		evidence, problem := readmodel.ReadCapacityEvidence(pull.CapacityServed, nil)
		if readErr == nil && problem == "" {
			probes, readErr = pull.CapacityProbes.Next()
			if readErr == nil {
				waits[index] = readmodel.ReadEndpointCapacity(waiting.Endpoint, refusals, evidence, s.now(), interval, probes)
				if !waits[index].Waiting {
					return "", false
				}
			}
		}
		break // one bounded invocation per item evaluation
	}
	var reasons []string
	for _, wait := range waits {
		schedule.ProviderCapacity = append(schedule.ProviderCapacity, wait)
		reasons = append(reasons, wait.Says())
	}
	if len(reasons) == 0 {
		return "no configured provider endpoint is eligible for this developer", true
	}
	return strings.Join(reasons, "; "), true
}
