package chat

import (
	"context"
	"testing"
	"time"

	backendapi "github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A conversation turn asks for the agent's effort level, and the conversation's
// record, its evidence, and the turn's cost line all say what was asked.
func TestAConversationTurnAsksForTheAgentsEffortAndRecordsIt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	log := &recordingSpendLog{}
	provider := &fakeBackend{results: []backendapi.RunResult{
		{SessionID: "session-1", FinalText: "Noted.", CostUSD: 0.02, CostReported: true},
	}}
	options := testOptions(t, provider)
	options.Store = newTestStore(t, root)
	options.Spend = log
	options.Effort = "medium"

	reply, err := openTestSession(t, options).Send(context.Background(), "what is next?")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(provider.requests) != 1 || provider.requests[0].Effort != "medium" {
		t.Fatalf("requests = %#v, want one at medium", provider.requests)
	}
	if reply.Evidence.Effort != "medium" {
		t.Fatalf("evidence effort = %q, want medium", reply.Evidence.Effort)
	}
	if len(log.lines) != 1 || log.lines[0].Effort != "medium" {
		t.Fatalf("cost lines = %#v, want the turn's line recording medium", log.lines)
	}
	recorded, err := options.Store.Load(runstate.ConversationIdentity{Agent: options.Agent, Role: options.Role})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if recorded.ProviderEffort != "medium" {
		t.Fatalf("recorded effort = %q, want medium", recorded.ProviderEffort)
	}
}

// A turn that crosses onto another provider keeps the agent's level where that
// provider accepts it, and is asked with none where it accepts none -- and the
// conversation's record then says none was asked, rather than the level the
// agent configured.
func TestACrossedTurnKeepsTheEffortOrRecordsThatItAskedForNone(t *testing.T) {
	t.Parallel()

	for _, adapter := range []domain.Backend{domain.BackendClaudeCode, domain.BackendCodex} {
		resetsAt := time.Date(2026, 9, 7, 11, 0, 0, 0, time.UTC)
		held := &speakingBackend{results: []backendapi.RunResult{
			{IsError: true, StopReason: "usage_limit", UsageLimit: &backendapi.UsageLimit{Kind: "five_hour", ResetsAt: resetsAt}},
		}}
		crossed := &speakingBackend{results: []backendapi.RunResult{
			{SessionID: "second-session-1", FinalText: "The second one first."},
		}}
		options := crossingOptions(t, held, crossed)
		options.Providers = effortProviderRegistry(t, adapter)
		options.UsageLimits = newTestUsageLimits(t)
		options.Effort = "high"

		if _, err := openTestSession(t, options).Send(context.Background(), "what now?"); err != nil {
			t.Fatalf("%s: Send() error = %v, want the turn served on the other provider", adapter, err)
		}
		want := "high"
		if adapter == domain.BackendCodex {
			want = ""
		}
		if len(held.requests) != 1 || held.requests[0].Effort != "high" {
			t.Fatalf("%s: held requests = %#v, want the refused attempt at high", adapter, held.requests)
		}
		if len(crossed.requests) != 1 || crossed.requests[0].Effort != want {
			t.Fatalf("%s: crossed requests = %#v, want one at %q", adapter, crossed.requests, want)
		}
		recorded, err := options.Store.Load(runstate.ConversationIdentity{Agent: options.Agent, Role: options.Role})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if recorded.ProviderEffort != want {
			t.Fatalf("%s: recorded effort = %q, want %q", adapter, recorded.ProviderEffort, want)
		}
	}
}

// effortProviderRegistry is the second provider of crossingOptions, launched by
// the named adapter, whose levels are the ones it inherits.
func effortProviderRegistry(t *testing.T, adapter domain.Backend) *backendapi.Registry {
	t.Helper()

	registry, err := backendapi.NewRegistry(map[domain.Backend]backendapi.ProviderPlugin{
		"second-provider": {
			Adapter: adapter,
			Roles: []domain.AgentRole{
				domain.RoleProductManager, domain.RoleArchitect, domain.RoleDevelopmentManager,
				domain.RoleDeveloper, domain.RoleReviewer,
			},
			Postures: []backendapi.Posture{backendapi.PostureReadOnly, backendapi.PostureWorktreeWrite},
			Dialect: backendapi.DialectSpec{Rules: []backendapi.DialectRule{
				{Answer: backendapi.AnswerRefused, Terminal: truth(true), Failed: truth(true)},
			}},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}
