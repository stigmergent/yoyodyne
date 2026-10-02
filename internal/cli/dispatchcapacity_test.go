package cli

import (
	"context"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestCapacityProbeUsesReadOnlyAccessAndRecordsSpendAndCapacityEvidence(t *testing.T) {
	t.Parallel()
	for _, refused := range []bool{false, true} {
		t.Run(map[bool]string{false: "served", true: "refused"}[refused], func(t *testing.T) {
			root := t.TempDir()
			limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			served, err := runstate.NewCapacityServedStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			log, err := runstate.NewSpendStore(root, "yoyodyne")
			if err != nil {
				t.Fatal(err)
			}
			cfg := config.Config{Product: config.Product{ID: "yoyodyne"}}
			endpoint := backend.Endpoint{Provider: domain.BackendClaudeCode, AdapterVersion: backend.ClaudeCodeAdapterVersion, AccountAlias: "default", Model: "opus"}
			result := backend.RunResult{Process: execution.ProcessResult{Status: execution.ProcessSucceeded}, CostReported: true, CostUSD: .01}
			if refused {
				result.IsError = true
				result.UsageLimit = &backend.UsageLimit{Kind: "seven_day", AccountWide: true, ResetsAt: time.Now().Add(time.Hour)}
			}
			provider := &capturingBackend{result: result}
			spent, err := probeProviderCapacityWith(context.Background(), components{config: cfg, stateRoot: root, repository: root, usageLimits: limits, capacityServed: served, spend: log}, endpoint, provider)
			if (err != nil) != refused {
				t.Fatalf("probe = %v, refused %t", err, refused)
			}
			if provider.request.Role != domain.RoleReviewer || !provider.request.CapacityProbe || len(provider.request.AllowedTools) != 0 || provider.request.Timeout != 30*time.Second || provider.request.SessionID != "" {
				t.Fatalf("request = %+v", provider.request)
			}
			if spent == nil || spent.CapacityProbeID == "" || spent.RunID != "" || spent.Phase != runstate.SpendPhaseCapacityProbe {
				t.Fatalf("spend = %+v", spent)
			}
			if err := spent.Validate(); err != nil {
				t.Fatal(err)
			}
			lines, err := log.List()
			if err != nil || len(lines) != 1 || lines[0].CapacityProbeID != spent.CapacityProbeID {
				t.Fatalf("spend log = %+v, %v", lines, err)
			}
			capacity, err := served.List()
			if err != nil {
				t.Fatal(err)
			}
			if refused && len(capacity) != 0 {
				t.Fatal("refused probe recorded served evidence")
			}
			if !refused && (len(capacity) != 1 || capacity[0].Provider != endpoint.Provider || capacity[0].Model != endpoint.Model) {
				t.Fatalf("capacity = %+v", capacity)
			}
			if refused {
				records, err := limits.List()
				if err != nil || len(records) != 1 || !records[0].AccountWide || records[0].Provider != endpoint.Provider {
					t.Fatalf("refusal = %+v, %v", records, err)
				}
			}
		})
	}
}

func TestCapacityProbeReleasesUnknownResetForRestrictedDeveloperEndpoints(t *testing.T) {
	t.Parallel()
	for _, adapter := range []domain.Backend{domain.BackendClaudeCode, domain.BackendCodex} {
		for _, restriction := range []string{"developer-only", "worktree-write-only"} {
			t.Run(string(adapter)+"/"+restriction, func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				runs, err := runstate.NewStore(root, "yoyodyne")
				if err != nil {
					t.Fatal(err)
				}
				limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
				if err != nil {
					t.Fatal(err)
				}
				served, err := runstate.NewCapacityServedStore(root, "yoyodyne")
				if err != nil {
					t.Fatal(err)
				}
				log, err := runstate.NewSpendStore(root, "yoyodyne")
				if err != nil {
					t.Fatal(err)
				}
				plugin := backend.ProviderPlugin{Adapter: adapter,
					Roles:    []domain.AgentRole{domain.RoleDeveloper},
					Postures: []backend.Posture{backend.PostureWorktreeWrite, backend.PostureReadOnly},
					Dialect:  backend.DialectSpec{Rules: []backend.DialectRule{{Type: "retry", Answer: backend.AnswerRetrying}}}}
				if restriction == "worktree-write-only" {
					plugin.Roles = append(plugin.Roles, domain.RoleReviewer)
					plugin.Postures = []backend.Posture{backend.PostureWorktreeWrite}
				}
				agent := pooledDeveloper()
				agent.Backend = "restricted"
				cfg := config.Config{Product: config.Product{ID: "yoyodyne"},
					Agents:    map[string]config.AgentConfig{"developers": agent},
					Providers: map[string]backend.ProviderPlugin{"restricted": plugin}}
				parts := components{config: cfg, stateRoot: root, repository: root, usageLimits: limits, capacityServed: served, spend: log}
				endpoints, err := dispatchEndpoints(parts, beads.WorkItem{})
				if err != nil || len(endpoints) != 1 {
					t.Fatalf("dispatch endpoints = %+v, %v", endpoints, err)
				}
				endpoint := endpoints[0]
				registry, err := cfg.ProviderRegistry()
				if err != nil {
					t.Fatal(err)
				}
				if err := registry.EligibleFor(endpoint, domain.RoleReviewer); err == nil {
					t.Fatal("fixture unexpectedly permits ordinary review")
				}
				if err := limits.Record(runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion,
					ProductID: "yoyodyne", At: time.Now().Add(-time.Hour), Waiting: "a developer run",
					Provider: endpoint.Provider, AccountAlias: endpoint.AccountAlias, Model: endpoint.Model}); err != nil {
					t.Fatal(err)
				}
				pool := accountPool{config: cfg, stateRoot: root, runs: runs, usageLimits: limits, capacityServed: served}
				if _, err := pool.ChooseAccount(); err == nil {
					t.Fatal("unknown-reset refusal did not withhold dispatch")
				}
				provider := &capturingBackend{result: backend.RunResult{Process: execution.ProcessResult{Status: execution.ProcessSucceeded}, CostReported: true}}
				spent, err := probeProviderCapacityWith(context.Background(), parts, endpoint, provider)
				if err != nil {
					t.Fatal(err)
				}
				if !provider.request.CapacityProbe || provider.request.Role != domain.RoleDeveloper || provider.request.SessionID != "" || len(provider.request.AllowedTools) != 0 || provider.request.Timeout != backend.CapacityProbeTimeout {
					t.Fatalf("probe request = %+v", provider.request)
				}
				if spent == nil || spent.Phase != runstate.SpendPhaseCapacityProbe {
					t.Fatalf("probe spend = %+v", spent)
				}
				// Rebuild the pool to prove the release is durable, not a local flag.
				fresh := accountPool{config: cfg, stateRoot: root, runs: runs, usageLimits: limits, capacityServed: served}
				if chosen, err := fresh.ChooseAccount(); err != nil || chosen.Alias != endpoint.AccountAlias {
					t.Fatalf("dispatch after served probe = %+v, %v", chosen, err)
				}
				if err := registry.EligibleFor(endpoint, domain.RoleReviewer); err == nil {
					t.Fatal("probe widened ordinary reviewer eligibility")
				}
			})
		}
	}
}

func TestDispatchAndReservationUseTheSameMappedModelAndSkipOnlyExhaustedAccounts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runs, err := runstate.NewStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	limits, err := runstate.NewUsageLimitStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	reset := poolClock.Add(time.Hour)
	if err := limits.Record(runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion, ProductID: "yoyodyne", At: poolClock.Add(-time.Hour), Waiting: "a developer run", Provider: "claude-code", AccountAlias: "one", Model: "opus", ResetsAt: &reset}); err != nil {
		t.Fatal(err)
	}
	cfg := twoAccounts(config.Account{}, config.Account{})
	cfg.Execution.DeveloperModels = []config.DeveloperModelRule{{Label: "complex", Model: "opus"}}
	pool := accountPool{config: cfg, stateRoot: root, runs: runs, usageLimits: limits, now: func() time.Time { return poolClock }}
	account, err := pool.ChooseAccountForModel("opus")
	if err != nil || account.Alias != "two" {
		t.Fatalf("mapped account = %+v, %v", account, err)
	}
	account, err = pool.ChooseAccountForModel("sonnet")
	if err != nil || account.Alias != "one" {
		t.Fatalf("unaffected account = %+v, %v", account, err)
	}
	endpoints, err := dispatchEndpoints(components{config: cfg, stateRoot: root}, beads.WorkItem{Labels: []string{"complex"}})
	if err != nil || len(endpoints) != 2 {
		t.Fatalf("endpoints = %+v, %v", endpoints, err)
	}
	for _, endpoint := range endpoints {
		if endpoint.Model != "opus" {
			t.Fatalf("mapped endpoint = %+v", endpoint)
		}
	}
	if cfg.Agents["developers"].Model != "sonnet" {
		t.Fatal("choosing a mapped model mutated the shared configuration")
	}
	zero := 0.0
	cfg.Accounts["one"] = config.Account{WeeklyBudgetUSD: &zero}
	endpoints, err = dispatchEndpoints(components{config: cfg, stateRoot: root, store: runs}, beads.WorkItem{Labels: []string{"complex"}})
	if err != nil || len(endpoints) != 1 || endpoints[0].AccountAlias != "two" {
		t.Fatalf("budgeted endpoints = %+v, %v; want the stood-down account excluded from probes", endpoints, err)
	}
}
