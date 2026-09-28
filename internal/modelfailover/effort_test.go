package modelfailover

// A substitution moves the model and never how hard it is asked to think: an
// alternate on the turn's own provider is asked at the turn's own effort level,
// and so is a crossing onto a provider that accepts it. The one crossing that
// cannot keep it -- onto a provider that accepts no such level -- is asked with
// none, and says so in what it reports as served.

import (
	"context"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/backend"
)

func TestAnAlternateOnTheSameProviderKeepsTheEffort(t *testing.T) {
	t.Parallel()

	resetsAt := time.Date(2026, 9, 7, 11, 0, 0, 0, time.UTC)
	provider := &fakeProvider{results: []backend.RunResult{
		refused("five_hour", resetsAt),
		{SessionID: "session-1", FinalText: "decided"},
	}}
	_, served, err := Serve(context.Background(), provider, backend.RunRequest{Model: "fable", Effort: "high"}, Policy{
		Alternate: "opus",
		Windows:   newTestWindows(t),
		Now:       fixedNow,
		ProductID: "yoyodyne",
		Waiting:   "the development manager conversation",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if len(provider.requests) != 2 || provider.requests[0].Effort != "high" || provider.requests[1].Effort != "high" {
		t.Fatalf("requests = %#v, want both attempts at high", provider.requests)
	}
	if served.Model != "opus" || served.Effort != "high" {
		t.Fatalf("served = %#v, want the alternate reported at the turn's own level", served)
	}
}

func TestAVersionFallbackKeepsTheEffort(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{results: []backend.RunResult{
		{IsError: true, StopReason: "api_error", ModelUnavailable: &backend.ModelUnavailable{Detail: "404 model"}},
		{SessionID: "session-1", FinalText: "decided"},
	}}
	_, served, err := Serve(context.Background(), provider, backend.RunRequest{Model: "opus", Effort: "max"}, Policy{
		Version:   "claude-opus-5-20260401",
		Windows:   newTestWindows(t),
		Now:       fixedNow,
		ProductID: "yoyodyne",
		Waiting:   "the architect conversation",
	})
	if err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if len(provider.requests) != 2 || provider.requests[1].Model != "opus" || provider.requests[1].Effort != "max" {
		t.Fatalf("requests = %#v, want the alias asked at max", provider.requests)
	}
	if served.Effort != "max" {
		t.Fatalf("served effort = %q, want max", served.Effort)
	}
}

func TestACrossingKeepsTheEffortUnlessTheOtherProviderAcceptsNone(t *testing.T) {
	t.Parallel()

	for _, drops := range []bool{false, true} {
		resetsAt := time.Date(2026, 9, 7, 11, 0, 0, 0, time.UTC)
		refusing := &fakeProvider{results: []backend.RunResult{refused("five_hour", resetsAt)}}
		crossed := &fakeProvider{results: []backend.RunResult{{SessionID: "other-session-1", FinalText: "decided"}}}
		policy := crossingPolicy(t, newTestWindows(t), func(request backend.RunRequest) (backend.RunRequest, error) {
			return request, nil
		})
		policy.AlternateProvider = crossed
		policy.AlternateDropsEffort = drops
		_, served, err := Serve(context.Background(), refusing, backend.RunRequest{
			Model: "fable", Effort: "high", Prompt: "what now?", AccountAlias: "house",
		}, policy)
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
		if refusing.requests[0].Effort != "high" {
			t.Fatalf("the refused attempt asked at %q, want high", refusing.requests[0].Effort)
		}
		want := "high"
		if drops {
			want = ""
		}
		if len(crossed.requests) != 1 || crossed.requests[0].Effort != want {
			t.Fatalf("drops=%v: crossed requests = %#v, want one at %q", drops, crossed.requests, want)
		}
		if !served.CrossedProviders() || served.Effort != want {
			t.Fatalf("drops=%v: served = %#v, want the crossing reported at %q", drops, served, want)
		}
	}
}
