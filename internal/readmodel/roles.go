package readmodel

import (
	"sort"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// RoleDefinitionStatus compares a definition with its latest activation. An
// amended file has an activation on record but is not activated as it stands.
type RoleDefinitionStatus struct {
	Definition   config.RoleDefinition    `json:"definition"`
	Activation   *runstate.RoleActivation `json:"activation,omitempty"`
	Activated    bool                     `json:"activated"`
	AmendedSince bool                     `json:"amended_since"`
}

// RoleDefinitions is the shared derivation every surface uses. History is in
// the store's order, newest first; an older matching digest cannot authorize a
// file that differs from the person's latest decision.
func RoleDefinitions(definitions map[string]config.RoleDefinition, history []runstate.RoleActivation) []RoleDefinitionStatus {
	latest := make(map[string]runstate.RoleActivation)
	for _, activation := range history {
		if _, seen := latest[activation.Name]; !seen {
			latest[activation.Name] = activation
		}
	}
	roles := make([]RoleDefinitionStatus, 0, len(definitions))
	for name, definition := range definitions {
		status := RoleDefinitionStatus{Definition: definition}
		if activation, found := latest[name]; found {
			status.Activation = &activation
			status.Activated = activation.Digest == definition.Digest
			status.AmendedSince = !status.Activated
		}
		roles = append(roles, status)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Definition.Name < roles[j].Definition.Name })
	return roles
}
