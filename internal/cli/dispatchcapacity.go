package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/spend"
)

func dispatchEndpoints(parts components, item beads.WorkItem) ([]backend.Endpoint, error) {
	cfg := parts.config
	name := agentNameForRole(cfg, domain.RoleDeveloper)
	provider, err := cfg.ProviderRegistry()
	if err != nil {
		return nil, err
	}
	agent := cfg.Agents[name]
	model := agent.Model
	if choice := config.ResolveDeveloperModel(cfg.Execution.DeveloperModels, item.Labels, model); choice.Chosen() {
		model = choice.Model
	}
	aliases := cfg.AccountAliases()
	if len(aliases) == 0 {
		aliases = []string{cfg.AccountAlias()}
	}
	var spent map[string]float64
	if cfg.HasAccountBudgets() {
		if parts.store == nil {
			return nil, errors.New("account budget evidence is unavailable")
		}
		spent, err = parts.store.SpentByAccountSince(time.Now().Add(-weeklyBudgetWindow))
		if err != nil {
			return nil, err
		}
	}
	var endpoints []backend.Endpoint
	for _, alias := range aliases {
		if budget, bounded := cfg.Accounts[alias].Budgeted(); bounded && spent[alias] >= budget {
			continue
		}
		choice, err := cfg.EndpointFor(provider, parts.stateRoot, name, alias)
		if err != nil {
			continue
		}
		choice.Endpoint.Model = model
		if err := provider.EligibleFor(choice.Endpoint, domain.RoleDeveloper); err != nil {
			continue
		}
		endpoints = append(endpoints, choice.Endpoint)
	}
	return endpoints, nil
}

// probeProviderCapacity gives the provider no assignment or session to continue.
// The compiled adapter narrows its access and work bound regardless of which
// configured role the endpoint serves. Spend uses the ordinary meter.
func probeProviderCapacity(ctx context.Context, parts components, endpoint backend.Endpoint) (*runstate.Spend, error) {
	account, err := parts.config.Endpoint(parts.stateRoot, endpoint.AccountAlias)
	if err != nil {
		return nil, err
	}
	return probeProviderCapacityWith(ctx, parts, endpoint, providerBackendIn(parts.config, endpoint.Provider, parts.runner, account.Directory))
}

func probeProviderCapacityWith(ctx context.Context, parts components, endpoint backend.Endpoint, invoker spend.Provider) (spent *runstate.Spend, failure error) {
	registry, err := parts.config.ProviderRegistry()
	if err != nil {
		return nil, err
	}
	role := domain.RoleReviewer
	if err := registry.EligibleFor(endpoint, role); err != nil {
		role = domain.RoleDeveloper
		if err := registry.EligibleFor(endpoint, role); err != nil {
			return nil, err
		}
	}
	account, err := parts.config.Endpoint(parts.stateRoot, endpoint.AccountAlias)
	if err != nil {
		return nil, err
	}
	identity := make([]byte, 16)
	if _, err := rand.Read(identity); err != nil {
		return nil, err
	}
	id := "capacity-probe-" + hex.EncodeToString(identity)
	provider := spend.Metered{
		Provider: invoker,
		Log:      parts.spend,
		Recorded: func(line runstate.Spend) { spent = &line },
		Attribution: spend.Attribution{ProductID: parts.config.Product.ID, CapacityProbeID: id,
			Phase: runstate.SpendPhaseCapacityProbe, Backend: endpoint.Provider,
			AccountAlias: endpoint.AccountAlias, ConfigRevision: parts.config.Revision()},
	}
	result, err := provider.Run(ctx, backend.RunRequest{
		RunID: id, Role: role, CapacityProbe: true, WorkingDirectory: parts.repository,
		Prompt: backend.CapacityProbePrompt,
		Model:  endpoint.Model, AccountAlias: endpoint.AccountAlias, AccountConfigDir: account.Directory,
		AllowedTools: []string{},
		Timeout:      backend.CapacityProbeTimeout, IdleTimeout: backend.CapacityProbeTimeout,
	})
	if result.UsageLimit != nil {
		limit := result.UsageLimit
		refusal := runstate.UsageLimitExhaustion{SchemaVersion: runstate.UsageLimitSchemaVersion,
			ProductID: parts.config.Product.ID, At: time.Now().UTC(), Waiting: "a bounded provider capacity probe " + id,
			Provider: endpoint.Provider, AccountAlias: endpoint.AccountAlias, Model: endpoint.Model,
			Kind: limit.Kind, AccountWide: limit.AccountWide}
		if !limit.ResetsAt.IsZero() {
			reset := limit.ResetsAt.UTC()
			refusal.ResetsAt = &reset
		}
		return spent, errors.Join(err, parts.usageLimits.Record(refusal), errors.New("the provider still refuses capacity"))
	}
	if err != nil {
		return spent, err
	}
	if !result.ServedCleanly() {
		return spent, fmt.Errorf("capacity was not established: %s", result.DescribeFailure())
	}
	return spent, parts.capacityServed.Record(runstate.CapacityServed{Provider: endpoint.Provider,
		AccountAlias: endpoint.AccountAlias, Model: endpoint.Model, At: time.Now().UTC(), What: "bounded capacity probe " + id})
}
