package readmodel

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/ownership"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// Every entry kind the read model names has a rule in the ownership registry:
// a kind added here without one fails.
func TestEveryAttentionKindHasARuleInTheRegistry(t *testing.T) {
	t.Parallel()
	for _, kind := range AttentionKinds() {
		if !ownership.Covers(kind) {
			t.Errorf("the read model names the kind %q and the ownership registry holds no rule for it", kind)
		}
	}
}

// Every entry the read model builds carries the registry's answer — owner,
// reason, remedy, and capability — and its sentence says that answer and
// nothing a surface worked out.
func TestEveryEntryCarriesTheRegistrysAnswer(t *testing.T) {
	t.Parallel()
	for kind, fixture := range attentionOfEveryKind(t) {
		entry := fixture.entry
		answer := ownership.Resolve(entry.ownershipEntry())
		if entry.Mover != answer.Owner || entry.OwnerReason != answer.Reason || entry.Remedy != answer.Remedy || entry.Capability != answer.Capability {
			t.Errorf("%s: entry carries %s (%q) %q %q, the registry says %+v", kind, entry.Mover, entry.OwnerReason, entry.Remedy, entry.Capability, answer)
		}
		if entry.Remedy == "" {
			t.Errorf("%s: no remedy", kind)
		}
		if entry.Whose() != answer.Whose() {
			t.Errorf("%s: whose = %q, want the registry's %q", kind, entry.Whose(), answer.Whose())
		}
	}
}

// A finding raised for the operator is his only where the account that raised
// it names a reason on the closed list; otherwise it is the Lead Product
// Manager's, and the sentence says so.
func TestAFindingIsTheOperatorsOnlyForANamedReason(t *testing.T) {
	t.Parallel()
	named := operatorActionAttention(OperatorAction{Key: "report:r-1", Finding: ownership.FindingHandling, Subject: "r-1",
		Needs: "beyond-grant: add the PreToolUse hook to .claude/settings.json", Ends: reportFindingEnds})
	if !named.Mover.IsOperator() || named.OwnerReason != ownership.ReasonBeyondGrant ||
		!strings.HasSuffix(named.Whose(), "(his because it is a file beyond every grant)") {
		t.Errorf("named = %s (%q): %q, want the operator's for the reason it names", named.Mover, named.OwnerReason, named.Whose())
	}
	unnamed := operatorActionAttention(OperatorAction{Key: "report:r-2", Finding: ownership.FindingHandling, Subject: "r-2",
		Needs: "the operator should look at this", Ends: reportFindingEnds})
	if unnamed.Mover != MoverProductManager || unnamed.OwnerReason != "" {
		t.Errorf("unnamed = %s (%q), want the Lead Product Manager's", unnamed.Mover, unnamed.OwnerReason)
	}
}

// An entry whose record the rule for its kind cannot classify is the Lead
// Product Manager's to classify, never the operator's.
func TestAnUnclassifiableEntryIsTheLeadProductManagersToClassify(t *testing.T) {
	t.Parallel()
	for _, entry := range []Attention{
		resolved(Attention{Kind: AttentionPublication, ID: "run-1"}),
		resolved(Attention{Kind: AttentionOperatorAction, ID: "report:r-1"}),
		resolved(Attention{Kind: AttentionHold, ID: HoldIntake}),
		resolved(Attention{Kind: AttentionOutage, ID: "no-such-cause", Outage: &runstate.ProviderOutage{}}),
	} {
		if entry.Mover != MoverProductManager || entry.Remedy != "classify this entry" {
			t.Errorf("%s %s: %s %q, want the Lead Product Manager's and \"classify this entry\"", entry.Kind, entry.ID, entry.Mover, entry.Remedy)
		}
	}
}
