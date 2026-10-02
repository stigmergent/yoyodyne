package ownership

import (
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestEveryPassedOverClassHasARegistryAnswer(t *testing.T) {
	want := map[runstate.PassedOverClass]Mover{
		runstate.PassedOverCarriedInConversation: Architect,
		runstate.PassedOverParked:                ProductManager,
		runstate.PassedOverHeldForAPerson:        DevelopmentManager,
		runstate.PassedOverAwaitingDecision:      DevelopmentManager,
		runstate.PassedOverAwaitingCarryOut:      Harness,
		runstate.PassedOverUnreadableGate:        ProductManager,
		runstate.PassedOverWaitingOnAPerson:      ProductManager,
		runstate.PassedOverValidHumanGate:        Operator,
		runstate.PassedOverWaitingOnOtherWork:    Nobody,
		runstate.PassedOverAlreadyTried:          Nobody,
		runstate.PassedOverAlreadyInFlight:       Nobody,
		runstate.PassedOverCoveredByChildren:     Nobody,
		runstate.PassedOverPausedByDirective:     ProductManager,
		runstate.PassedOverSequencedBehindWork:   Nobody,
		runstate.PassedOverPrerequisiteUnmet:     DevelopmentManager,
		runstate.PassedOverLeftForAnotherSlot:    Nobody,
		runstate.PassedOverWaitingOnUsageWindow:  Nobody,
	}
	for _, class := range runstate.PassedOverClasses() {
		entry := Entry{Kind: KindPassedOver, PassedOver: class, Role: domain.RoleArchitect}
		answer, classified := registry[KindPassedOver](entry)
		owner, covered := want[class]
		if !covered || !classified || answer.Owner != owner || answer.Remedy == "" {
			t.Errorf("%s: classified = %v, answer = %+v, want owner %s", class, classified, answer, owner)
		}
		if owner.IsOperator() && answer.Reason != ReasonHumanGate {
			t.Errorf("%s: operator reason = %q, want the declared human gate", class, answer.Reason)
		}
	}
}
