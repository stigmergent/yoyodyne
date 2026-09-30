package chat

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/ownership"
)

// A handling or an escalation reaches the operator only where its reason opens
// with one of the ownership registry's reasons and a colon, so the two roles
// that write them are told that form in the contracts they read. Without it no
// handling or escalation carries a reason, and every finding only a person can
// act on comes back to the Lead Product Manager instead of reaching him.
func TestTheContractsCarryTheFormThatNamesTheOperatorsReason(t *testing.T) {
	t.Parallel()
	for name, contract := range map[string]string{
		"product manager":     productManagerContract,
		"development manager": developmentManagerContract,
	} {
		if !strings.Contains(contract, ownership.NamingForm) {
			t.Errorf("the %s's contract does not carry the form that names the operator's reason: %q", name, ownership.NamingForm)
		}
	}
	// And neither says an escalation or a critical report reaches him on its own.
	for name, contract := range map[string]string{
		"product manager":     productManagerContract,
		"development manager": developmentManagerContract,
	} {
		for _, stale := range []string{"reaches him the same way on its own", "keeps it for the operator", "escalates the hold to the operator itself"} {
			if strings.Contains(contract, stale) {
				t.Errorf("the %s's contract still says %q", name, stale)
			}
		}
	}
}
