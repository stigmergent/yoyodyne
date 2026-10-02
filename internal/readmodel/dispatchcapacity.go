package readmodel

import (
	"fmt"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// EndpointCapacity is a dispatch waiting for capacity. An unknown reset stays
// held until a paced probe succeeds; merely reaching the probe time releases no work.
type EndpointCapacity struct {
	Endpoint    backend.Endpoint              `json:"endpoint"`
	Waiting     bool                          `json:"waiting"`
	ResetsAt    *time.Time                    `json:"resets_at,omitempty"`
	NextProbeAt time.Time                     `json:"next_probe_at,omitempty"`
	Refusal     runstate.UsageLimitExhaustion `json:"-"`
}

// MergeCapacityRefusals uses the invocation log where it names a run's refusal,
// retaining snapshots only for runs older builds never wrote to that log.
func MergeCapacityRefusals(refusals, snapshots []runstate.UsageLimitExhaustion) []runstate.UsageLimitExhaustion {
	merged := append([]runstate.UsageLimitExhaustion(nil), refusals...)
	for _, snapshot := range snapshots {
		recorded := false
		for _, refusal := range refusals {
			if refusal.Waiting == snapshot.Waiting && refusal.Model == snapshot.Model && refusal.Kind == snapshot.Kind {
				recorded = true
				break
			}
		}
		if !recorded {
			merged = append(merged, snapshot)
		}
	}
	return merged
}

func (c EndpointCapacity) Says() string {
	if !c.Waiting {
		return ""
	}
	if c.ResetsAt != nil {
		return fmt.Sprintf("waiting for provider capacity on %s until %s", c.Endpoint, c.ResetsAt.Local().Format("2006-01-02 15:04 MST"))
	}
	return fmt.Sprintf("waiting for provider capacity on %s; next bounded probe at %s", c.Endpoint, c.NextProbeAt.Local().Format("2006-01-02 15:04 MST"))
}

// ReadEndpointCapacity reads the same refusal and served records as the capacity
// surfaces. Later refusals supersede earlier ones on the same capacity scope.
func ReadEndpointCapacity(endpoint backend.Endpoint, refusals []runstate.UsageLimitExhaustion, evidence CapacityEvidence, now time.Time, interval time.Duration, probes map[string]time.Time) EndpointCapacity {
	c := EndpointCapacity{Endpoint: endpoint}
	latest := map[string]runstate.UsageLimitExhaustion{}
	for _, refusal := range evidence.Standing(refusals) {
		if !refusal.Refuses(endpoint.Provider, endpoint.AccountAlias, endpoint.Model) {
			continue
		}
		key := refusal.CapacityKey(endpoint.Provider, endpoint.AccountAlias, endpoint.Model) + "\x00" + refusal.Kind
		if previous, found := latest[key]; !found || !refusal.At.Before(previous.At) {
			latest[key] = refusal
		}
	}
	for _, refusal := range latest {
		// A real reset releases dispatch at the provider's deadline. A missing
		// or stale reset requires a successful probe, rather than another run.
		known := refusal.ResetsAt != nil && refusal.ResetsAt.After(refusal.At)
		if known && !now.Before(*refusal.ResetsAt) {
			continue
		}
		if c.Waiting {
			if c.ResetsAt != nil && (!known || !refusal.ResetsAt.After(*c.ResetsAt)) {
				continue
			}
			if c.ResetsAt == nil && !known && !refusal.At.After(c.Refusal.At) {
				continue
			}
		}
		c.Waiting, c.Refusal, c.ResetsAt = true, refusal, nil
		c.NextProbeAt = refusal.At.Add(interval)
		if known {
			c.ResetsAt = refusal.ResetsAt
		}
		if next := probes[refusal.CapacityKey(endpoint.Provider, endpoint.AccountAlias, endpoint.Model)]; next.After(c.NextProbeAt) {
			c.NextProbeAt = next
		}
	}
	return c
}
