package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestCapacityDispatchWaitSurvivesFreshPullsAndKeepsUnrelatedWorkUsable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(48 * time.Hour)
	root := t.TempDir()
	limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	refusal := runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion, ProductID: "yoyodyne",
		At: now.Add(-time.Hour), Waiting: "a developer run returned to the backlog", Provider: "claude-code",
		AccountAlias: "one", Model: "opus", Kind: "seven_day", ResetsAt: &reset}
	if err := limits.Record(refusal); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		h := newScheduleHarness(readyItems("work-1", "work-2")...)
		h.now = now
		// A new store and scheduler reproduce a supervisor restart, with no
		// remembered exclusions and only the durable refusal holding work.
		reopened, err := runstate.NewUsageLimitStore(root, "yoyodyne")
		if err != nil {
			t.Fatal(err)
		}
		open := func(ctx context.Context) (Pull, error) {
			pull, err := h.open(ctx)
			pull.UsageLimits = reopened
			pull.DispatchEndpoints = func(item beads.WorkItem) ([]backend.Endpoint, error) {
				model := "opus"
				if item.ID == "work-2" {
					model = "sonnet"
				}
				return []backend.Endpoint{{Provider: "claude-code", AccountAlias: "one", Model: model}}, nil
			}
			return pull, err
		}
		schedule, err := (Scheduler{Open: open, Now: h.clock}).Schedule(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(schedule.Started) != 1 || schedule.Started[0].WorkItemID != "work-2" {
			t.Fatalf("started = %+v", schedule.Started)
		}
		if len(schedule.Deferred) != 1 || schedule.Deferred[0].WorkItemID != "work-1" || !strings.Contains(schedule.Deferred[0].Reason, "waiting for provider capacity") {
			t.Fatalf("deferred = %+v", schedule.Deferred)
		}
	}
	h := newScheduleHarness(readyItems("work-1", "work-2")...)
	h.now = reset
	openAtReset := func(ctx context.Context) (Pull, error) {
		pull, err := h.open(ctx)
		pull.UsageLimits = limits
		pull.DispatchEndpoints = func(beads.WorkItem) ([]backend.Endpoint, error) {
			return []backend.Endpoint{{Provider: "claude-code", AccountAlias: "one", Model: "opus"}}, nil
		}
		return pull, err
	}
	released, err := (Scheduler{Open: openAtReset, Now: h.clock}).Schedule(context.Background())
	if err != nil || len(released.Started) == 0 || released.Started[0].WorkItemID != "work-1" {
		t.Fatalf("reset release = %+v, %v; want the first waiting item to retain priority", released.Started, err)
	}
	// Shared capacity holds the mapped model too, while another account and
	// provider remain available. The deadline releases all models at reset.
	refusal.AccountWide = true
	if err := limits.Record(refusal); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		endpoint backend.Endpoint
		at       time.Time
		waiting  bool
	}{
		{"shared model", backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "sonnet"}, now, true},
		{"other account", backend.Endpoint{Provider: "claude-code", AccountAlias: "two", Model: "sonnet"}, now, false},
		{"other provider", backend.Endpoint{Provider: "codex", AccountAlias: "one", Model: "sonnet"}, now, false},
		{"reset", backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "sonnet"}, reset, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pull := Pull{UsageLimits: limits, DispatchEndpoints: func(beads.WorkItem) ([]backend.Endpoint, error) { return []backend.Endpoint{test.endpoint}, nil }}
			_, waiting := (Scheduler{Now: func() time.Time { return test.at }}).dispatchCapacity(context.Background(), &Schedule{}, pull, beads.WorkItem{})
			if waiting != test.waiting {
				t.Fatalf("waiting = %t, want %t", waiting, test.waiting)
			}
		})
	}
}

func TestUnknownCapacityResetProbesArePacedAcrossRestartsAndOnlySuccessReleasesWork(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	served, err := runstate.NewCapacityServedStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	refusal := runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion, ProductID: "yoyodyne",
		At: now.Add(-time.Hour), Waiting: "a refused developer run", Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	if err := limits.Record(refusal); err != nil {
		t.Fatal(err)
	}
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "one", Model: "opus"}
	probed := 0
	succeed := false
	pull := Pull{UsageLimits: limits, CapacityServed: served, OutageProbe: 30 * time.Minute,
		DispatchEndpoints: func(beads.WorkItem) ([]backend.Endpoint, error) { return []backend.Endpoint{endpoint}, nil },
		ProbeCapacity: func(ctx context.Context, _ backend.Endpoint) (*runstate.Spend, error) {
			probed++
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
				t.Fatal("probe has no short bound")
			}
			if !succeed {
				return nil, errors.New("capacity still exhausted")
			}
			return nil, served.Record(runstate.CapacityServed{Provider: endpoint.Provider, AccountAlias: endpoint.AccountAlias, Model: endpoint.Model, At: now})
		},
	}
	for range 3 {
		pull.CapacityProbes, err = runstate.NewCapacityProbeStore(root, "yoyodyne")
		if err != nil {
			t.Fatal(err)
		}
		schedule := Schedule{}
		_, held := (Scheduler{Now: func() time.Time { return now }}).dispatchCapacity(context.Background(), &schedule, pull, beads.WorkItem{})
		if !held {
			t.Fatal("a refused probe released dispatch")
		}
		if len(schedule.ProviderCapacity) != 1 || !schedule.ProviderCapacity[0].NextProbeAt.Equal(now.Add(30*time.Minute)) {
			t.Fatalf("wait = %+v", schedule.ProviderCapacity)
		}
	}
	if probed != 1 {
		t.Fatalf("probed = %d, want one probe across restarts", probed)
	}
	now = now.Add(30 * time.Minute)
	succeed = true
	_, held := (Scheduler{Now: func() time.Time { return now }}).dispatchCapacity(context.Background(), &Schedule{}, pull, beads.WorkItem{})
	if held || probed != 2 {
		t.Fatalf("held = %t after %d probes, want success to release", held, probed)
	}
}

func TestAUsageWindowBeyondTheRunPauseBudgetIsDurablyRecordedBeforeItReturnsWork(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := Pipeline{UsageLimits: limits, Config: config.Config{
		Agents:    map[string]config.AgentConfig{"developer": {Role: domain.RoleDeveloper, Backend: "claude-code", Model: "opus"}},
		Execution: config.Execution{UsageLimitMaxPause: config.Duration(time.Hour)},
	}}
	a := activeRun{pipeline: pipeline, state: runstate.State{ProductID: "yoyodyne", RunID: "run-1", WorkItemID: "work-1", AccountAlias: "one", Phase: runstate.PhaseDeveloping}}
	reset := time.Now().Add(48 * time.Hour)
	err = a.pauseForUsageLimit(context.Background(), backend.UsageLimit{Kind: "seven_day", AccountWide: true, ResetsAt: reset})
	var stopped usageWindowStop
	if !errors.As(err, &stopped) {
		t.Fatalf("pause = %v, want environmental stop", err)
	}
	reopened, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	refusals, err := reopened.List()
	if err != nil || len(refusals) != 1 {
		t.Fatalf("refusals = %+v, %v", refusals, err)
	}
	if !refusals[0].AccountWide || refusals[0].Provider != "claude-code" || refusals[0].Model != "opus" || !refusals[0].ResetsAt.Equal(reset) {
		t.Fatalf("refusal = %+v", refusals[0])
	}
}

func TestKnownCapacityWindowCanBeProbedEarlyButACallbackAloneReleasesNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Now().UTC()
	reset := now.Add(48 * time.Hour)
	limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	if err := limits.Record(runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion, ProductID: "yoyodyne", At: now.Add(-time.Hour), Waiting: "a refused developer", Model: "opus", ResetsAt: &reset}); err != nil {
		t.Fatal(err)
	}
	served, err := runstate.NewCapacityServedStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	probes, err := runstate.NewCapacityProbeStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := backend.Endpoint{Provider: "claude-code", AccountAlias: "default", Model: "opus"}
	proved := false
	pull := Pull{UsageLimits: limits, CapacityServed: served, CapacityProbes: probes, OutageProbe: 30 * time.Minute,
		DispatchEndpoints: func(beads.WorkItem) ([]backend.Endpoint, error) { return []backend.Endpoint{endpoint}, nil },
		ProbeCapacity: func(context.Context, backend.Endpoint) (*runstate.Spend, error) {
			if proved {
				return nil, served.Record(runstate.CapacityServed{Provider: endpoint.Provider, AccountAlias: endpoint.AccountAlias, Model: endpoint.Model, At: now})
			}
			return &runstate.Spend{Classification: runstate.SpendKnown, AmountUSD: .01}, nil
		},
	}
	schedule := Schedule{}
	_, held := (Scheduler{Now: func() time.Time { return now }}).dispatchCapacity(context.Background(), &schedule, pull, beads.WorkItem{})
	if !held || schedule.SpentUSD != .01 {
		t.Fatalf("held = %t, spent = %f", held, schedule.SpentUSD)
	}
	now = now.Add(30 * time.Minute)
	proved = true
	_, held = (Scheduler{Now: func() time.Time { return now }}).dispatchCapacity(context.Background(), &Schedule{}, pull, beads.WorkItem{})
	if held {
		t.Fatal("successful bounded probe did not release the future window")
	}
}

func TestCapacityProbeSpendStopsDispatchAtTheSessionBudget(t *testing.T) {
	t.Parallel()
	for _, priced := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown cost", true: "budget spent"}[priced], func(t *testing.T) {
			root := t.TempDir()
			now := time.Now().UTC()
			limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			if err := limits.Record(runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion, ProductID: "yoyodyne", At: now.Add(-time.Hour), Waiting: "a refused developer", Model: "opus"}); err != nil {
				t.Fatal(err)
			}
			probes, err := runstate.NewCapacityProbeStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			h := newScheduleHarness(readyItems("work-1", "work-2")...)
			h.now = now
			open := func(ctx context.Context) (Pull, error) {
				pull, err := h.open(ctx)
				pull.UsageLimits, pull.CapacityProbes = limits, probes
				pull.DispatchEndpoints = func(item beads.WorkItem) ([]backend.Endpoint, error) {
					model := "opus"
					if item.ID == "work-2" {
						model = "sonnet"
					}
					return []backend.Endpoint{{Provider: "claude-code", AccountAlias: "one", Model: model}}, nil
				}
				pull.ProbeCapacity = func(context.Context, backend.Endpoint) (*runstate.Spend, error) {
					if priced {
						return &runstate.Spend{Classification: runstate.SpendKnown, AmountUSD: 1}, nil
					}
					return &runstate.Spend{Classification: runstate.SpendUnknown}, nil
				}
				return pull, err
			}
			schedule, err := (Scheduler{Open: open, Budget: 1, Now: h.clock}).Schedule(context.Background())
			wantStop := ScheduleSpendUnreadable
			if priced {
				wantStop = ScheduleBudgetSpent
			}
			if err != nil || len(schedule.Started) != 0 || schedule.Stopped != wantStop {
				t.Fatalf("schedule started %+v, stopped %q, error %v; want no dispatch after probe spend", schedule.Started, schedule.Stopped, err)
			}
		})
	}
}
