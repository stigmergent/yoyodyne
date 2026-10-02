package ownership

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/amendment"
	"github.com/mason-bryant/yoyodyne/internal/artifact"
	"github.com/mason-bryant/yoyodyne/internal/backlog"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// The registry covers every entry kind: a kind with no rule fails here.
func TestRegistryHoldsARuleForEveryKind(t *testing.T) {
	for _, kind := range Kinds() {
		if !Covers(kind) {
			t.Errorf("the registry holds no rule for the kind %q", kind)
		}
	}
	if len(registry) != len(Kinds()) {
		t.Errorf("the registry holds %d rules for %d kinds; a rule for a kind outside the vocabulary is a kind nobody can show", len(registry), len(Kinds()))
	}
}

// fixtures is, for every kind, records that drive its rule through each of the
// owners it can return — the operator among them where the kind can be his.
func fixtures() map[Kind][]Entry {
	goals := amendment.Proposal{ID: "amendment-1", Artifact: "v1-goals", Kind: artifact.KindGoals, Owner: domain.RoleProductManager}
	design := amendment.Proposal{ID: "amendment-2", Artifact: "a-design", Kind: artifact.KindDesign, Owner: domain.RoleArchitect}
	operatorIntake := runstate.IntakeHold{HeldBy: runstate.IntakeHolderOperator, HeldAt: time.Unix(1, 0)}
	brake := func(decision runstate.IntakeBrakeDecision) *runstate.IntakeHold {
		return &runstate.IntakeHold{HeldBy: runstate.IntakeHolderBrake, Brake: &runstate.IntakeBrake{Decision: decision}}
	}
	queued := &runstate.PullRequest{Number: 1, MergeQueued: true}
	red := &runstate.PullRequest{Number: 2, TargetRed: &runstate.TargetRed{}}
	open := &runstate.PullRequest{Number: 3}
	stopped := runstate.State{RunID: "run-1", WorkItemID: "item-1", Blocker: "checks failed"}
	return map[Kind][]Entry{
		KindPassedOver: passedOverFixtures(),
		KindAmendment: {
			{Kind: KindAmendment, Amendment: &goals},
			{Kind: KindAmendment, Amendment: &design},
			{Kind: KindAmendment},
		},
		KindCarriedItem: {
			{Kind: KindCarriedItem, Role: domain.RoleArchitect},
			{Kind: KindCarriedItem},
		},
		KindReports:           {{Kind: KindReports}},
		KindFactoryStall:      {{Kind: KindFactoryStall}},
		KindTrackerUnanswered: {{Kind: KindTrackerUnanswered}},
		KindAmendmentQueue:    {{Kind: KindAmendmentQueue}},
		KindOwedStep:          {{Kind: KindOwedStep}},
		KindDirective:         {{Kind: KindDirective}},
		KindDegradedService:   {{Kind: KindDegradedService}},
		KindProductDecision:   {{Kind: KindProductDecision}},
		KindPublication: {
			{Kind: KindPublication, Publication: &Publication{}},
			{Kind: KindPublication, Publication: &Publication{PullRequest: queued}},
			{Kind: KindPublication, Publication: &Publication{PullRequest: red}},
			{Kind: KindPublication, Publication: &Publication{PullRequest: open, MergeDropped: true}},
			{Kind: KindPublication, Publication: &Publication{PullRequest: open, Unarmed: true}},
			{Kind: KindPublication, Publication: &Publication{PullRequest: open}},
			{Kind: KindPublication},
		},
		KindFailingTask: {
			{Kind: KindFailingTask, FailingCause: runstate.PreTurnConversationUnopened},
			{Kind: KindFailingTask, FailingCause: runstate.PreTurnMessageRefused},
			{Kind: KindFailingTask},
		},
		KindHold: {
			{Kind: KindHold, Hold: HoldOperator},
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: &operatorIntake},
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: brake("")},
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: brake(runstate.BrakeDecisionRelease)},
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: brake(runstate.BrakeDecisionEscalate)},
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: brake(runstate.BrakeDecisionProbe)},
			{Kind: KindHold, Hold: HoldIntake},
			{Kind: KindHold, Hold: HoldCapacity},
			{Kind: KindHold},
			// A brake hold written before the brake kept its own record is the
			// brake's, not one the operator placed.
			{Kind: KindHold, Hold: HoldIntake, IntakeHold: &runstate.IntakeHold{HeldBy: runstate.IntakeHolderBrake}},
		},
		KindOutage: {
			{Kind: KindOutage, OutageCause: domain.ProviderUnauthenticated},
			{Kind: KindOutage, OutageCause: domain.ProviderUnreachable},
			{Kind: KindOutage},
		},
		KindStall:    stallFixtures(),
		KindHeldWork: heldFixtures(),
		KindOperatorAction: {
			{Kind: KindOperatorAction, Finding: FindingHandling, Account: "credential: renew the forge token"},
			{Kind: KindOperatorAction, Finding: FindingHandling, Account: "the operator should look at this"},
			{Kind: KindOperatorAction, Finding: FindingCriticalReport, Account: "beyond-grant: a critical report cannot claim it"},
			{Kind: KindOperatorAction, Finding: FindingEscalation, Account: "diverged-history: the target diverged from the forge"},
			{Kind: KindOperatorAction, Finding: FindingEscalation, Account: "the run keeps failing"},
			{Kind: KindOperatorAction, Finding: FindingAmendmentBatch, Role: domain.RoleArchitect},
			{Kind: KindOperatorAction, Finding: FindingAmendmentBatch},
			{Kind: KindOperatorAction},
		},
		KindHumanGate: {
			{Kind: KindHumanGate, Gate: "sign-off", WorkItemID: "item-1"},
			{Kind: KindHumanGate, GateUnreadable: true},
			{Kind: KindHumanGate},
		},
		KindUntracedPass: {
			{Kind: KindUntracedPass, Role: domain.RoleProgramManager},
			{Kind: KindUntracedPass},
		},
		KindStoppage: {
			{Kind: KindStoppage, Stopped: &stopped},
			{Kind: KindStoppage, Stopped: &stopped, AwaitingCarryOut: true},
			{Kind: KindStoppage},
		},
	}
}

func passedOverFixtures() []Entry {
	entries := []Entry{{Kind: KindPassedOver, Unreadable: true}, {Kind: KindPassedOver, UsageWindow: true}}
	for _, class := range runstate.PassedOverClasses() {
		entries = append(entries, Entry{Kind: KindPassedOver, PassedOver: class, Role: domain.RoleArchitect})
	}
	return entries
}

func stallFixtures() []Entry {
	var entries []Entry
	for _, reason := range StallReasons() {
		entries = append(entries, Entry{Kind: KindStall, StallReason: reason})
	}
	return append(entries,
		Entry{Kind: KindStall, StallReason: StallProviderAway, OutageCause: domain.ProviderUnauthenticated},
		Entry{Kind: KindStall},
	)
}

func heldFixtures() []Entry {
	entries := []Entry{
		{Kind: KindHeldWork, Held: HeldAwaitingDecision},
		{Kind: KindHeldWork, Held: HeldAwaitingCarryOut},
		{Kind: KindHeldWork},
	}
	for _, group := range []backlog.HoldKind{backlog.HeldForAPerson, backlog.HeldByDirective, backlog.HeldForAGate, backlog.HeldWaitingOn, backlog.HeldParked, backlog.HeldCovered, backlog.HeldUnread} {
		entries = append(entries, Entry{Kind: KindHeldWork, HeldIn: group, Held: HeldAwaitingDecision})
	}
	return append(entries,
		Entry{Kind: KindHeldWork, HeldIn: backlog.HeldByConversation, Role: domain.RoleArchitect},
		Entry{Kind: KindHeldWork, HeldIn: backlog.HeldByStall, StallReason: StallSessionIdle},
		Entry{Kind: KindHeldWork, HeldIn: backlog.HeldByStall, StallReason: StallDivergedTarget},
	)
}

// For every kind, fixture records driven through the rule never return the
// operator without a reason from the closed list, and never return a reason
// for anybody else. A kind with no fixture fails, so a kind added without one
// cannot pass unexamined.
func TestTheOperatorIsReturnedOnlyWithAReasonFromTheClosedList(t *testing.T) {
	all := fixtures()
	for _, kind := range Kinds() {
		entries := all[kind]
		if len(entries) == 0 {
			t.Errorf("no fixture drives the rule for %q", kind)
			continue
		}
		for index, entry := range entries {
			// The rule itself, not Resolve: Resolve's own refusal would hide a
			// rule that names him without a reason.
			resolved, classified := registry[kind](entry)
			if !classified {
				continue
			}
			switch {
			case resolved.Owner.IsOperator() && !resolved.Reason.Valid():
				t.Errorf("%s fixture %d: the rule returns the operator with the reason %q, which is not on the closed list %v", kind, index, resolved.Reason, Reasons())
			case !resolved.Owner.IsOperator() && resolved.Reason != "":
				t.Errorf("%s fixture %d: the rule returns %s with the reason %q, which only the operator carries", kind, index, resolved.Owner, resolved.Reason)
			case !resolved.Owner.Valid():
				t.Errorf("%s fixture %d: the rule returns the owner %q, which is not a mover", kind, index, resolved.Owner)
			case strings.TrimSpace(resolved.Remedy) == "":
				t.Errorf("%s fixture %d: the rule returns %s with no remedy", kind, index, resolved.Owner)
			}
		}
	}
}

// The operator is what the design's table says he is and nothing more: these
// fixtures are his, each for its reason, and every other fixture is not.
func TestTheOperatorsEntriesAreTheOnesTheDesignNames(t *testing.T) {
	type key struct {
		kind  Kind
		index int
	}
	his := map[key]Reason{
		{KindAmendment, 0}:      ReasonFundamentalIntent,
		{KindHold, 0}:           ReasonOwnHold,
		{KindHold, 1}:           ReasonOwnHold,
		{KindOutage, 0}:         ReasonCredential,
		{KindOperatorAction, 0}: ReasonCredential,
		{KindOperatorAction, 3}: ReasonDivergedHistory,
		{KindHumanGate, 0}:      ReasonHumanGate,
		{KindHeldWork, 5}:       ReasonHumanGate,
		{KindHeldWork, 12}:      ReasonDivergedHistory,
	}
	stall := stallFixtures()
	for index, entry := range passedOverFixtures() {
		if entry.PassedOver == runstate.PassedOverWaitingOnAPerson {
			his[key{KindPassedOver, index}] = ReasonHumanGate
		}
	}
	for index, entry := range stall {
		switch {
		case entry.StallReason == StallOperatorHold:
			his[key{KindStall, index}] = ReasonOwnHold
		case entry.StallReason == StallDivergedTarget:
			his[key{KindStall, index}] = ReasonDivergedHistory
		case entry.OutageCause == domain.ProviderUnauthenticated:
			his[key{KindStall, index}] = ReasonCredential
		}
	}
	for kind, entries := range fixtures() {
		for index, entry := range entries {
			resolved := Resolve(entry)
			want, isHis := his[key{kind, index}]
			if resolved.Owner.IsOperator() != isHis || resolved.Reason != want {
				t.Errorf("%s fixture %d resolves to %s (%q), want the operator's = %v (%q)", kind, index, resolved.Owner, resolved.Reason, isHis, want)
			}
		}
	}
}

// An entry no rule can classify is the Lead Product Manager's, to classify —
// never the operator's.
func TestAnEntryNoRuleCanClassifyIsTheLeadProductManagersToClassify(t *testing.T) {
	unclassifiable := []Entry{
		{Kind: KindPassedOver},
		{Kind: KindPassedOver, PassedOver: "no-such-class"},
		{Kind: KindPassedOver, PassedOver: runstate.PassedOverCarriedInConversation},
		{Kind: "no-such-kind"},
		{},
		{Kind: KindAmendment},
		{Kind: KindPublication},
		{Kind: KindHold},
		{Kind: KindOutage},
		{Kind: KindStall},
		{Kind: KindHumanGate},
		{Kind: KindOperatorAction},
		{Kind: KindCarriedItem},
		{Kind: KindUntracedPass},
		{Kind: KindStoppage},
		{Kind: KindFailingTask},
		{Kind: KindHeldWork},
		{Kind: KindHeldWork, HeldIn: backlog.HoldKind("no-such-group")},
	}
	for _, entry := range unclassifiable {
		resolved := Resolve(entry)
		if resolved.Owner != ProductManager || resolved.Remedy != "classify this entry" || resolved.Reason != "" {
			t.Errorf("%+v resolves to %s (%q) with the remedy %q; want the Lead Product Manager, no reason, and \"classify this entry\"", entry, resolved.Owner, resolved.Reason, resolved.Remedy)
		}
	}
}

// Resolve refuses a rule that names the operator without a closed-list
// reason, so no path through the registry names him by default.
func TestResolveRefusesTheOperatorWithoutAReason(t *testing.T) {
	saved := registry[KindReports]
	t.Cleanup(func() { registry[KindReports] = saved })
	registry[KindReports] = func(Entry) (Resolution, bool) {
		return Resolution{Owner: Operator, Remedy: "read the pile"}, true
	}
	if resolved := Resolve(Entry{Kind: KindReports}); resolved.Owner.IsOperator() || resolved.Remedy != RemedyClassify {
		t.Errorf("a rule naming the operator without a reason resolved to %s (%q); want it refused as unclassified", resolved.Owner, resolved.Remedy)
	}
	registry[KindReports] = func(Entry) (Resolution, bool) {
		return Resolution{Owner: Operator, Reason: "because", Remedy: "read the pile"}, true
	}
	if resolved := Resolve(Entry{Kind: KindReports}); resolved.Owner.IsOperator() {
		t.Errorf("a rule naming the operator with a reason off the closed list resolved to him")
	}
	registry[KindReports] = func(Entry) (Resolution, bool) {
		return Resolution{Owner: ProductManager, Reason: "because", Remedy: "read the pile"}, true
	}
	if resolved := Resolve(Entry{Kind: KindReports}); resolved.Remedy != RemedyClassify || resolved.Reason != "" {
		t.Errorf("a rule assigning a reason to a role resolved to %+v; want it refused as unclassified", resolved)
	}
}

func TestNamedInReadsOnlyALeadingClosedListReason(t *testing.T) {
	for text, want := range map[string]Reason{
		"credential: the token expired":          ReasonCredential,
		"  Beyond-Grant : add the hook":          ReasonBeyondGrant,
		"the token expired: credential":          "",
		"it needs a credential, and a person":    "",
		"urgent: the operator has to look at it": "",
		"":                                       "",
	} {
		if got := NamedIn(text); got != want {
			t.Errorf("NamedIn(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestEveryReasonSaysWhy(t *testing.T) {
	for _, reason := range Reasons() {
		if reason.Says() == "" {
			t.Errorf("the reason %q says nothing", reason)
		}
	}
	whose := Resolve(Entry{Kind: KindHold, Hold: HoldOperator}).Whose()
	if !strings.HasPrefix(whose, "the operator's — ") || !strings.HasSuffix(whose, " (his because it is a hold he placed himself)") {
		t.Errorf("the operator's hold is said %q; want his possessive, the remedy, and why it is his", whose)
	}
}

func TestEveryStallReasonAnswers(t *testing.T) {
	for _, reason := range StallReasons() {
		if reason.Whose() == "" {
			t.Errorf("the stall reason %q says nobody's move", reason)
		}
	}
}

// The form a role's contract prints names every reason on the closed list, and
// an account written in it is read back as that reason.
func TestTheNamingFormNamesEveryReason(t *testing.T) {
	for _, reason := range Reasons() {
		if !strings.Contains(NamingForm, string(reason)) {
			t.Errorf("NamingForm does not name %q", reason)
		}
		if got := NamedIn(string(reason) + ": what the operator has to do"); got != reason {
			t.Errorf("an account opening %q is read as %q", reason, got)
		}
	}
	// The remedy an unnamed handling carries tells her the form.
	if remedy := Resolve(Entry{Kind: KindOperatorAction, Finding: FindingHandling, Account: "look at this"}).Remedy; !strings.Contains(remedy, NamingForm) {
		t.Errorf("the remedy for a handling naming no reason is %q; want it to show the form", remedy)
	}
}

// The reasons are the invariant's closed list, not a set a new rule extends.
func TestOperatorReasonsStayOnTheClosedList(t *testing.T) {
	want := []Reason{"fundamental-intent", "credential", "forge-setting", "beyond-grant", "human-gate", "own-hold", "diverged-history"}
	got := Reasons()
	if len(got) != len(want) {
		t.Fatalf("reasons = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reason %d = %q, want %q", i, got[i], want[i])
		}
	}
}
