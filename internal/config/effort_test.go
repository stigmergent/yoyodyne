package config

import (
	"strings"
	"testing"
)

// The shipped template states an effort level beside the model for every agent,
// and states the provider's own default for the model it names rather than a
// level somebody preferred: Claude Code's documentation gives "medium" as the
// default of Opus 5.5, which is what "opus" serves.
func TestTheTemplatePinsEveryAgentToTheProvidersDefaultEffort(t *testing.T) {
	t.Parallel()

	for source, cfg := range map[string]Config{
		"the bundle":   loadProject(t, minimalProjectConfig, nil).Config,
		"the scaffold": loadScaffold(t, ScaffoldOptions{ProductID: "example", Repository: "."}).Config,
	} {
		if len(cfg.Agents) == 0 {
			t.Fatalf("%s configures no agents", source)
		}
		for name, agent := range cfg.Agents {
			if agent.Model != "opus" {
				t.Fatalf("%s: agent %q model = %q; the pinned default below is Opus 5.5's and has to be revisited for another model", source, name, agent.Model)
			}
			if got := cfg.AgentEffort(name); got != "medium" {
				t.Fatalf("%s: agent %q effort = %q, want Claude Code's default for Opus 5.5, medium", source, name, got)
			}
		}
	}
}

// A level the agent's provider does not accept is refused when the file loads,
// with a sentence naming the levels it does.
func TestAnEffortTheProviderDoesNotAcceptIsRefusedNamingTheLevels(t *testing.T) {
	t.Parallel()

	_, err := loadProjectError(t, minimalProjectConfig+`agents:
  architect:
    effort: extreme
`, nil)
	if err == nil {
		t.Fatal("LoadResolved() succeeded, want the level refused")
	}
	want := `agent "architect" names effort "extreme", which provider "claude-code" does not accept; effort is one of low, medium, high, xhigh, or max`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

// Every level Claude Code accepts loads, and a later layer stating the key empty
// removes an inherited level, which is how an agent goes back to the provider
// resolving its own.
func TestEveryAcceptedLevelLoadsAndEmptyRemovesAnInheritedOne(t *testing.T) {
	t.Parallel()

	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		cfg := loadProject(t, minimalProjectConfig+`agents:
  developer:
    effort: `+level+`
`, nil).Config
		if got := cfg.AgentEffort("developer"); got != level {
			t.Fatalf("effort = %q, want %q", got, level)
		}
	}
	resolved := loadProject(t, minimalProjectConfig+`agents:
  developer:
    effort: ""
`, nil)
	if got := resolved.Config.AgentEffort("developer"); got != "" {
		t.Fatalf("effort = %q, want the inherited level removed", got)
	}
	if origin := resolved.Origins["agents.developer.effort"]; origin == "" {
		t.Fatalf("the removal has no origin; origins = %v", resolved.Origins)
	}
}

// Codex is given no effort level by this harness, so a Codex agent naming one is
// refused, saying so, rather than launched with a level its provider may reject.
func TestACodexAgentNamingAnEffortIsRefused(t *testing.T) {
	t.Parallel()

	_, err := loadProjectError(t, minimalProjectConfig+`agents:
  developer:
    backend: codex
    model: gpt-5-codex
    effort: high
`, nil)
	if err == nil {
		t.Fatal("LoadResolved() succeeded, want the Codex agent's level refused")
	}
	want := `agent "developer" names effort "high", and provider "codex" accepts no effort level from this harness; leave effort out for this agent`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

// A failover that crosses onto a provider accepting no level does not stop the
// file loading: that turn is asked with none and says so, which is the
// failover's own business rather than a reason to refuse the configuration.
func TestAFailoverCrossingOntoAProviderWithoutEffortLoads(t *testing.T) {
	t.Parallel()

	cfg := loadProject(t, minimalProjectConfig+`accounts:
  default:
    provider: claude-code
  codex-account:
    provider: codex
agents:
  developer:
    model: fable
    effort: high
    failover:
      enabled: true
      model: gpt-5-codex
      provider: codex
      account: codex-account
`, nil).Config
	if got := cfg.AgentEffort("developer"); got != "high" {
		t.Fatalf("effort = %q, want the agent's own level kept", got)
	}
}
