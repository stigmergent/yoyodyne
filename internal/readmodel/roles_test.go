package readmodel

import (
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestRoleDefinitionsUseTheLatestActivationAndSortByName(t *testing.T) {
	definitions := map[string]config.RoleDefinition{
		"zebra": {Name: "zebra", Digest: "old"},
		"alpha": {Name: "alpha", Digest: "current"},
		"never": {Name: "never", Digest: "unactivated"},
	}
	history := []runstate.RoleActivation{
		{Name: "zebra", Digest: "new"},
		{Name: "alpha", Digest: "current"},
		{Name: "zebra", Digest: "old"},
	}
	roles := RoleDefinitions(definitions, history)
	if len(roles) != 3 || roles[0].Definition.Name != "alpha" || roles[1].Definition.Name != "never" || roles[2].Definition.Name != "zebra" {
		t.Fatalf("definition order = %#v", roles)
	}
	if !roles[0].Activated || roles[0].AmendedSince || roles[1].Activated || roles[1].AmendedSince || roles[1].Activation != nil {
		t.Fatalf("current and unactivated definitions = %#v", roles)
	}
	if roles[2].Activated || !roles[2].AmendedSince || roles[2].Activation.Digest != "new" {
		t.Fatalf("an older matching digest authorized the current file: %#v", roles[2])
	}
}
