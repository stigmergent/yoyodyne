package readmodel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/backlog"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/humangate"
	"github.com/mason-bryant/yoyodyne/internal/ownership"
)

// The not-startable line counts its work by what it waits on, names the next
// step and whose it is for each group, and says in one sentence whether any of
// it is the operator's — the six groups the operator's dashboard of 2026-09-27
// held, beside the ready work waiting for a slot, which is counted apart.
func TestTheNotStartableLineCountsItsWorkByWhatItWaitsOn(t *testing.T) {
	t.Parallel()
	entries := []struct {
		entry backlog.Entry
		kind  backlog.HoldKind
	}{
		{backlog.Entry{ID: "item-decision", Awaiting: "run run-a stopped on it"}, backlog.HeldForAPerson},
		{backlog.Entry{ID: "item-carry-out", Awaiting: "run run-b stopped on it; the decision is recorded", AwaitingCarryOut: true}, backlog.HeldForAPerson},
		{backlog.Entry{ID: "item-waiting", WaitingOn: []string{"item-other"}}, backlog.HeldWaitingOn},
		{backlog.Entry{ID: "item-conversation", Executor: domain.ConversationWith(domain.RoleArchitect)}, backlog.HeldByConversation},
		{backlog.Entry{ID: "item-parked", Parking: "off the critical path"}, backlog.HeldParked},
		{backlog.Entry{ID: "item-covered"}, backlog.HeldCovered},
		{backlog.Entry{ID: "item-parked-too", Parking: "off the critical path"}, backlog.HeldParked},
	}
	groups := newWaitGroups(Stall{}, switches{})
	refused := make([]Refused, 0, len(entries))
	for _, tried := range entries {
		if tried.entry.HoldKind() != tried.kind && tried.kind != backlog.HeldCovered {
			t.Fatalf("entry %s holds as %q, want %q", tried.entry.ID, tried.entry.HoldKind(), tried.kind)
		}
		groups.add(tried.entry, tried.kind)
		refused = append(refused, Refused{WorkItemID: tried.entry.ID, Reason: "its refusal", Kind: tried.kind})
	}
	listed := groups.list()
	standing := Standing{
		Admitted:                len(entries) + 45,
		AwaitingDecision:        1,
		AwaitingCarryOut:        1,
		NotStartable:            refused,
		NotStartableGroups:      listed,
		NotStartableForOperator: ForOperator(listed),
		WaitingForSlot:          &SlotWait{Ready: 45, Slots: 3, InFlight: 3},
	}

	rendered := standing.Render()
	want := []string{
		"Not startable (7 of 52 admitted items; 1 awaits the development manager's decision, 1 awaits the harness carrying out a decision already recorded):\n",
		"  - 45 ready, waiting for a developer slot; 3 slots, all taken — not counted as not startable: the harness starts the next one as a run in flight finishes, and nothing is asked of anybody\n",
		"  - 1 waits on the development manager's decision about a stopped run — next: she decides what becomes of each stopped run: a repair, a re-run, a wait, a re-scope, or an escalation; whose: the development manager's\n",
		"  - 1 waits on the harness carrying out a decision already recorded — next: the harness acts on the recorded decision — a repair, a re-run, or a re-armed merge — at its next pull; whose: the harness's\n",
		"  - 1 waits on other items — next: the harness pulls each once the work it waits on lands; whose: the harness's\n",
		"  - 1 is done in the architect's conversation, not by a run — next: the role does the work in its conversation, and the harness closes each item when the role's revision lands; whose: the architect's\n",
		"  - 2 are parked by the Lead Product Manager — next: she releases each once what it was parked for is settled; whose: the Lead Product Manager's\n",
		"  - 1 is covered by other work: their own unfinished children — next: the harness runs the children; nothing pulls the covering item itself; whose: the harness's\n",
		"  - nothing here is the operator's: the ownership registry assigns each waiting group to a role, the harness, or nobody\n",
	}
	at := -1
	for _, line := range want {
		index := strings.Index(rendered, line)
		if index < 0 {
			t.Fatalf("rendered lacks %q:\n%s", line, rendered)
		}
		if index < at {
			t.Fatalf("%q is out of order:\n%s", line, rendered)
		}
		at = index
	}
	if !strings.Contains(rendered, "  item-decision — its refusal\n") {
		t.Fatalf("the items themselves are no longer listed:\n%s", rendered)
	}

	// The hourly message carries the heads, and the counts by who moves them are
	// what it has to say: every tally line survives the brief rendering, and the
	// items under them do not.
	brief := standing.RenderBrief()
	for _, line := range want[1:] {
		if !strings.Contains(brief, line) {
			t.Fatalf("brief rendering lacks %q:\n%s", line, brief)
		}
	}
	if strings.Contains(brief, "item-decision") {
		t.Fatalf("brief rendering lists items:\n%s", brief)
	}

	// --json carries the groups, each with its kind, count, next step, and mover.
	encoded, err := json.Marshal(standing)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Groups []struct {
			Kind     string   `json:"kind"`
			Awaiting string   `json:"awaiting"`
			Count    int      `json:"count"`
			Next     string   `json:"next"`
			Mover    string   `json:"mover"`
			Items    []string `json:"-"`
		} `json:"not_startable_groups"`
		ForOperator string `json:"not_startable_for_operator"`
		Slot        struct {
			Ready int `json:"ready"`
			Slots int `json:"slots"`
		} `json:"waiting_for_slot"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	movers := map[string]string{}
	for _, group := range decoded.Groups {
		if group.Next == "" {
			t.Fatalf("group %+v names no next step", group)
		}
		movers[group.Kind+"/"+group.Awaiting] = group.Mover
	}
	wantMovers := map[string]string{
		"held/decision":  "development-manager",
		"held/carry-out": "harness",
		"waiting/":       "harness",
		"conversation/":  "architect",
		"parked/":        "product-manager",
		"covered/":       "harness",
	}
	for key, mover := range wantMovers {
		if movers[key] != mover {
			t.Fatalf("groups by mover = %v, want %s moved by %s", movers, key, mover)
		}
	}
	if len(decoded.Groups) != len(wantMovers) || decoded.ForOperator == "" || decoded.Slot.Ready != 45 || decoded.Slot.Slots != 3 {
		t.Fatalf("json = %s", encoded)
	}
}

// Where something on the line is the operator's, the sentence says what and
// how many rather than that nothing is — and what the registry gives a role is
// not counted as his.
func TestTheNotStartableLineSaysWhatIsTheOperators(t *testing.T) {
	t.Parallel()
	groups := newWaitGroups(Stall{Reason: ReasonDivergedTarget, Says: "the target branch main diverged from the forge's", Clears: "follow the recovery"}, switches{})
	groups.add(backlog.Entry{ID: "item-a"}, backlog.HeldByStall)
	groups.add(backlog.Entry{ID: "item-b"}, backlog.HeldByDirective)
	listed := groups.list()
	said := ForOperator(listed)
	if !strings.HasPrefix(said, "1 item here is the operator's — ") ||
		strings.Contains(said, "directive") ||
		!strings.Contains(said, "diverged from the forge") {
		t.Fatalf("for the operator = %q", said)
	}
	if listed[0].Mover != MoverProductManager {
		t.Fatalf("directive group = %+v, want the Lead Product Manager's", listed[0])
	}
	if listed[1].Next != "follow the recovery" || !listed[1].Mover.IsOperator() {
		t.Fatalf("stalled group = %+v, want the command that clears it and the operator named", listed[1])
	}
}

// Whose move a stalled group is agrees with what the stall reason itself
// says: both are the registry's answer, so the reason's sentence opens with
// the group's mover.
func TestAStalledGroupsMoverAgreesWithItsReason(t *testing.T) {
	t.Parallel()
	for _, reason := range Reasons() {
		if reason == ReasonIntakeHold {
			continue
		}
		mover := newWaitGroups(Stall{Reason: reason}, switches{}).shape(backlog.Entry{}, backlog.HeldByStall).Mover
		if whose := reason.Whose(); !strings.HasPrefix(whose, mover.Possessive()) {
			t.Errorf("%s says %q, and its group names %s", reason, whose, mover)
		}
	}
}

// A step only a person can take is a group of its own, ranked with the other
// things a person moves and named as the operator's, because recording the act
// is his and nothing any role or run does passes it. Counting it among the held
// work would send whoever reads the line to the development manager for it.
func TestAGatedItemIsTheOperatorsOwnGroup(t *testing.T) {
	t.Parallel()
	gated := backlog.Entry{ID: "yoyodyne-ifd.209.7", HumanGates: humangate.Read(humangate.DeclareMarker + " soak-reviewed — the operator has judged the parity soak")}
	if kind := gated.HoldKind(); kind != backlog.HeldForAGate {
		t.Fatalf("kind = %q, want %q", kind, backlog.HeldForAGate)
	}
	groups := newWaitGroups(Stall{}, switches{})
	groups.add(backlog.Entry{ID: "item-waiting", WaitingOn: []string{"item-other"}}, backlog.HeldWaitingOn)
	groups.add(gated, gated.HoldKind())
	listed := groups.list()
	if len(listed) != 2 || listed[0].Kind != backlog.HeldForAGate || listed[0].Mover != ownership.Operator {
		t.Fatalf("groups = %+v, want the gated group first and the operator's", listed)
	}
	if !strings.Contains(listed[0].Says(), "yoyo gate record") {
		t.Fatalf("says = %q, want the act that passes it named", listed[0].Says())
	}
	if sentence := ForOperator(listed); !strings.Contains(sentence, "1 item here is the operator's") || !strings.Contains(sentence, "a step only a person can take") {
		t.Fatalf("for the operator = %q", sentence)
	}
}

// Malformed declarations are corrections, not acts the operator can record.
// A mixed item carries both obligations, without changing the item count.
func TestGateDeclarationsAgreeAcrossIndividualEntriesAndGroups(t *testing.T) {
	t.Parallel()
	valid := humangate.DeclareMarker + " sign-off — review the release"
	another := humangate.DeclareMarker + " release-signed — sign the release"
	unreadable := humangate.DeclareMarker
	mixed := humangate.Read(valid, another, unreadable)
	for _, fixture := range []struct {
		name              string
		reading           humangate.Reading
		acts, corrections int
	}{
		{"valid only", humangate.Read(valid), 1, 0},
		{"unreadable only", humangate.Read(unreadable), 0, 1},
		{"conflicting statements only", humangate.Read(valid, humangate.DeclareMarker+" sign-off — a different act", valid), 0, 1},
		{"valid gate beside conflicting statements", humangate.Read(valid, humangate.DeclareMarker+" sign-off — a different act", another), 1, 1},
		{"mixed declarations", mixed, 2, 1},
		{"mixed after every act was recorded", mixed.Pending([]string{"sign-off", "release-signed"}), 0, 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			entry := backlog.Entry{ID: "gated-item", Title: "release", HumanGates: fixture.reading}
			if kind := entry.HoldKind(); kind != backlog.HeldForAGate {
				t.Fatalf("HoldKind() = %q, want the human-gate hold", kind)
			}
			attention := Gated(backlog.Queue{Entries: []backlog.Entry{entry}})
			acts, corrections := 0, 0
			for _, waiting := range attention {
				if waiting.Mover.IsOperator() {
					acts++
					if waiting.OwnerReason != ownership.ReasonHumanGate || waiting.HumanGate.Gate == "" || waiting.Capability != "yoyo gate record" {
						t.Fatalf("operator entry = %+v, want a recordable act with its reason", waiting)
					}
				} else {
					corrections++
					if waiting.Mover != MoverProductManager || waiting.OwnerReason != "" || waiting.HumanGate.Unreadable == "" || !strings.Contains(waiting.Remedy, "correct the declaration") {
						t.Fatalf("correction entry = %+v, want the Lead Product Manager's", waiting)
					}
				}
			}
			if acts != fixture.acts || corrections != fixture.corrections {
				t.Fatalf("attention has %d acts and %d corrections, want %d and %d", acts, corrections, fixture.acts, fixture.corrections)
			}
			groups := newWaitGroups(Stall{}, switches{})
			groups.add(entry, entry.HoldKind())
			listed := groups.list()
			owners := map[Mover]WaitGroup{}
			for _, group := range listed {
				if group.Count != 1 || len(group.Items) != 1 || group.Items[0].WorkItemID != entry.ID {
					t.Fatalf("group = %+v, want the item counted once per obligation", group)
				}
				owners[group.Mover] = group
			}
			if fixture.acts > 0 {
				group, found := owners[ownership.Operator]
				if !found || group.OwnerReason != ownership.ReasonHumanGate || group.Capability != "yoyo gate record" || !strings.Contains(group.Next, "records each act") || !strings.Contains(group.WaitsOn, "step only a person") {
					t.Fatalf("operator group = %+v, want only recordable acts", group)
				}
				delete(owners, ownership.Operator)
			}
			if fixture.corrections > 0 {
				group, found := owners[MoverProductManager]
				want := ownership.Resolve(ownership.Entry{Kind: ownership.KindHumanGate, GateUnreadable: true})
				if !found || group.OwnerReason != want.Reason || group.Next != want.Remedy || group.Capability != want.Capability || !strings.Contains(group.WaitsOn, "correction of an unreadable") {
					t.Fatalf("correction group = %+v, want the same answer as the individual declaration %+v", group, want)
				}
				delete(owners, MoverProductManager)
			}
			if len(owners) > 0 {
				t.Fatalf("unexpected owners: %+v", owners)
			}
			forOperator := ForOperator(listed)
			if fixture.acts > 0 {
				if !strings.HasPrefix(forOperator, "1 item here is the operator's") || strings.Contains(forOperator, "unreadable") {
					t.Fatalf("ForOperator() = %q, want one item with a recordable act", forOperator)
				}
			} else if !strings.HasPrefix(forOperator, "nothing here is the operator's") {
				t.Fatalf("ForOperator() = %q, want no operator obligation", forOperator)
			}
			standing := Standing{Admitted: 1, NeedsHuman: attention,
				NotStartable:       []Refused{{WorkItemID: entry.ID, Kind: backlog.HeldForAGate}},
				NotStartableGroups: listed, NotStartableForOperator: forOperator}
			for _, rendered := range []string{standing.Render(), standing.RenderBrief()} {
				if !strings.Contains(rendered, "Not startable (1 of 1 admitted item") || !strings.Contains(rendered, forOperator) {
					t.Fatalf("rendered does not count the refused item once or project ForOperator:\n%s", rendered)
				}
				for _, group := range listed {
					if !strings.Contains(rendered, group.Says()) {
						t.Fatalf("rendered does not project group %q:\n%s", group.Says(), rendered)
					}
				}
			}
			encoded, err := json.Marshal(standing)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Standing
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded.NotStartable) != 1 || len(decoded.NotStartableGroups) != len(listed) || ForOperator(decoded.NotStartableGroups) != forOperator {
				t.Fatalf("JSON lost the obligations or changed the item count: %s", encoded)
			}
		})
	}
}
