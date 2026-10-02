package readmodel

import (
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestDispatchCapacityIsScopedAndReleasesOnlyOnResetOrServedEvidence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(48 * time.Hour)
	refusal := runstate.UsageLimitExhaustion{At: now.Add(-time.Hour), Provider: "claude-code", AccountAlias: "one", Model: "opus", ResetsAt: &reset}
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	for _, test := range []struct {
		name     string
		endpoint backend.Endpoint
		refusal  runstate.UsageLimitExhaustion
		at       time.Time
		evidence CapacityEvidence
		waiting  bool
	}{
		{"same endpoint", endpoint, refusal, now, CapacityEvidence{}, true},
		{"different account", backend.Endpoint{Provider: "claude-code", AccountAlias: "two", Model: "opus"}, refusal, now, CapacityEvidence{}, false},
		{"different provider", backend.Endpoint{Provider: "codex", AccountAlias: "one", Model: "opus"}, refusal, now, CapacityEvidence{}, false},
		{"different model", backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "sonnet"}, refusal, now, CapacityEvidence{}, false},
		{"at reset", endpoint, refusal, reset, CapacityEvidence{}, false},
		{"served same scope", endpoint, refusal, now, CapacityEvidence{Served: []runstate.CapacityServed{{Provider: "claude-code", AccountAlias: "one", Model: "opus", At: now}}}, false},
		{"served other account", endpoint, refusal, now, CapacityEvidence{Served: []runstate.CapacityServed{{Provider: "claude-code", AccountAlias: "two", Model: "opus", At: now}}}, true},
		{"served other provider", endpoint, refusal, now, CapacityEvidence{Served: []runstate.CapacityServed{{Provider: "codex", AccountAlias: "one", Model: "opus", At: now}}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := ReadEndpointCapacity(test.endpoint, []runstate.UsageLimitExhaustion{test.refusal}, test.evidence, test.at, 30*time.Minute, nil)
			if got.Waiting != test.waiting {
				t.Fatalf("capacity = %+v, want waiting %t", got, test.waiting)
			}
		})
	}
	refusal.AccountWide = true
	endpoint.Model = "sonnet"
	if !ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{refusal}, CapacityEvidence{}, now, time.Minute, nil).Waiting {
		t.Fatal("shared account window left another model usable")
	}
	evidence := CapacityEvidence{Served: []runstate.CapacityServed{{Provider: endpoint.Provider, AccountAlias: endpoint.AccountAlias, Model: endpoint.Model, At: now}}}
	if ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{refusal}, evidence, now, time.Minute, nil).Waiting {
		t.Fatal("served shared account window remained closed")
	}
}

func TestAnUnknownResetNeedsAProbeAndAReservedProbeSurvivesElapsedTime(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	refusal := runstate.UsageLimitExhaustion{At: now.Add(-time.Hour), Model: "opus", AccountWide: true}
	key := refusal.CapacityKey(endpoint.Provider, endpoint.AccountAlias, endpoint.Model)
	next := now.Add(30 * time.Minute)
	got := ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{refusal}, CapacityEvidence{}, now, 30*time.Minute, map[string]time.Time{key: next})
	if !got.Waiting || got.ResetsAt != nil || !got.NextProbeAt.Equal(next) {
		t.Fatalf("capacity = %+v, want held with reserved next probe", got)
	}
	got = ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{refusal}, CapacityEvidence{}, next.Add(time.Second), 30*time.Minute, nil)
	if !got.Waiting {
		t.Fatal("elapsed probe interval released dispatch without a served probe")
	}
	// A later refusal with a real deadline replaces the old unknown-reset
	// refusal on the same scope. The old entry cannot hold past the new reset.
	reset := now.Add(time.Hour)
	later := refusal
	later.At, later.ResetsAt = now, &reset
	got = ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{refusal, later}, CapacityEvidence{}, reset, time.Minute, nil)
	if got.Waiting {
		t.Fatal("superseded unknown reset held dispatch past the later reset")
	}
}

func TestALaterShortWindowDoesNotReleaseASeparateWeeklyWindow(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	weeklyReset, shortReset := now.Add(48*time.Hour), now.Add(time.Hour)
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	weekly := runstate.UsageLimitExhaustion{At: now.Add(-time.Hour), Kind: "seven_day", Model: "opus", AccountWide: true, ResetsAt: &weeklyReset}
	short := weekly
	short.At, short.Kind, short.ResetsAt = now, "five_hour", &shortReset
	got := ReadEndpointCapacity(endpoint, []runstate.UsageLimitExhaustion{weekly, short}, CapacityEvidence{}, shortReset, time.Minute, nil)
	if !got.Waiting || got.ResetsAt == nil || !got.ResetsAt.Equal(weeklyReset) {
		t.Fatalf("capacity = %+v, want the weekly window to remain closed", got)
	}
}

func TestEndedRunCapacityRefusalsSurviveTheRunAndDoNotWidenALoggedScope(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	run := runstate.State{RunID: "run-1", WorkItemID: "work-1", ProductID: "yoyodyne", Backend: "claude-code", AccountAlias: "one",
		Status: runstate.StatusCancelled, Phase: runstate.PhaseDeveloping, UsageLimitModel: "opus", UsageLimitKind: "seven_day",
		Environmental: &runstate.EnvironmentalRefusal{Cause: runstate.CauseUsageWindow, RecordedAt: now.Add(-time.Minute), ResetsAt: &reset}}
	refusals := EndedRunCapacityRefusals([]runstate.State{run})
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	if len(refusals) != 1 || !ReadEndpointCapacity(endpoint, refusals, CapacityEvidence{}, now, time.Minute, nil).Waiting {
		t.Fatalf("ended run's refusal = %+v", refusals)
	}
	logged := refusals[0]
	logged.Provider = "codex"
	merged := MergeCapacityRefusals([]runstate.UsageLimitExhaustion{logged}, refusals)
	if len(merged) != 1 || ReadEndpointCapacity(endpoint, merged, CapacityEvidence{}, now, time.Minute, nil).Waiting {
		t.Fatalf("logged scope was widened by a snapshot: %+v", merged)
	}
}
