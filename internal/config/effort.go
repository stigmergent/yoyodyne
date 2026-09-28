package config

// Naming how hard an agent's provider is asked to think, beside the model it is
// asked to think with.
//
// Before this key existed no agent set a level, the harness passed none, and
// every role ran at whatever its provider resolved for itself -- an environment
// variable, a level saved in the machine's settings, or the model's own default,
// which differs by model. None of that is written anywhere the harness reads, so
// nobody could say what level a role had run at. An agent now names one, the
// level is validated here against what the agent's provider accepts, and every
// invocation of the agent passes it and records it.
//
// The level is the agent's and not the model's. A failover alternate, a version
// fallback, a model execution.developer_models maps an item to, and a model a
// recurring task names are all served at the agent's level, because each of
// them moves which model answers and none of them is a decision about how hard
// it is asked to think. The one exception is a failover crossing onto a provider
// that would not accept the level: that turn is asked with none, and its record
// says so, rather than the crossing failing on a flag the provider refuses --
// which would cost the turn the failover exists to save.
//
// The key is optional, so a file written before it existed loads unchanged and
// runs exactly as it did: no level passed, the provider's own resolution. The
// shipped template states one for every agent, and `yoyo config drift` reports
// it to such a project as available.

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/backend"
	"github.com/mason-bryant/yoyodyne/internal/domain"
)

// effortProblems reports an effort level one agent's provider, or the provider
// its failover alternate crosses onto, would not accept. A provider the project
// does not name is reported elsewhere, and nothing is said about its levels.
func effortProblems(providers *backend.Registry, name string, agent AgentConfig) []string {
	level := strings.TrimSpace(agent.Effort)
	if level == "" {
		return nil
	}
	var problems []string
	if descriptor, known := providers.Lookup(agent.Backend); known && !descriptor.AcceptsEffort(level) {
		problems = append(problems, effortRefusal(name, level, agent.Backend, descriptor))
	}
	return problems
}

func effortRefusal(name, level string, provider domain.Backend, descriptor backend.Descriptor) string {
	if len(descriptor.EffortLevels) == 0 {
		return fmt.Sprintf("agent %q names effort %q, and provider %q accepts no effort level from this harness; leave effort out for this agent",
			name, level, provider)
	}
	return fmt.Sprintf("agent %q names effort %q, which provider %q does not accept; effort is one of %s",
		name, level, provider, descriptor.DescribeEffortLevels())
}

// AgentEffort is the effort level one configured agent's invocations ask for,
// and empty for an agent that names none.
func (c Config) AgentEffort(name string) string {
	return strings.TrimSpace(c.Agents[strings.TrimSpace(name)].Effort)
}
