package notify

import (
	"strings"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/ownership"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/report"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A legacy or independently assembled event can lack Mover. Its renderer must
// still agree with the registry about ownership, including the closed reason
// on a pause the operator placed and the fallback where the cause is absent.
func TestWaitingNotificationsWithoutMoverProjectRegistryOwnership(t *testing.T) {
	for _, test := range []struct {
		kind  Kind
		entry ownership.Entry
		owner ownership.Mover
	}{
		{KindWatchStopped, ownership.Entry{Kind: ownership.KindStall, StallReason: ownership.StallNoWatchSession}, ownership.Harness},
		{KindWatchReadRetrying, ownership.Entry{Kind: ownership.KindStall, StallReason: ownership.StallStoreUnreadable}, ownership.Harness},
		{KindWatchRedeploying, ownership.Entry{Kind: ownership.KindStall, StallReason: ownership.StallRedeploying}, ownership.Nobody},
		{KindCapacityHold, ownership.Entry{Kind: ownership.KindHold, Hold: ownership.HoldCapacity}, ownership.Harness},
		{KindHoldPlaced, ownership.Entry{Kind: ownership.KindHold, Hold: ownership.HoldOperator}, ownership.Operator},
		{KindProviderWindow, ownership.Entry{Kind: ownership.KindPassedOver, UsageWindow: true}, ownership.Nobody},
		{KindProviderOutage, ownership.Entry{Kind: ownership.KindOutage}, ownership.ProductManager},
		{KindRecurringTaskFailing, ownership.Entry{Kind: ownership.KindFailingTask}, ownership.ProductManager},
		{KindOperatorAction, ownership.Entry{Kind: ownership.KindOperatorAction}, ownership.ProductManager},
		{KindLineWaiting, ownership.Entry{Kind: ownership.KindStall}, ownership.ProductManager},
		{KindStallNoticed, ownership.Entry{Kind: ownership.KindStall}, ownership.ProductManager},
		{KindWatchBraked, ownership.Entry{Kind: ownership.KindHold, Hold: ownership.HoldIntake}, ownership.ProductManager},
		{KindIntakeHeld, ownership.Entry{Kind: ownership.KindHold, Hold: ownership.HoldIntake}, ownership.ProductManager},
		{KindIntakeEscalated, ownership.Entry{Kind: ownership.KindHold, Hold: ownership.HoldIntake}, ownership.ProductManager},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			answer := ownership.Resolve(test.entry)
			if answer.Owner != test.owner {
				t.Fatalf("registry owner = %q, want %q", answer.Owner, test.owner)
			}
			for _, speaker := range speakers() {
				event := fullyRecorded(test.kind)
				// Severity and the event's name cannot grant ownership to the operator.
				event.Severity = report.SeverityCritical
				message, err := Render(Product(), speaker, event)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(message.Body, nextMoveLead+ended(answer.Whose())) {
					t.Fatalf("%s rendered %q, want registry answer %q", speaker.Key(), message.Body, answer.Whose())
				}
				if test.kind == KindOperatorAction {
					account := strings.Split(message.Body, nextMoveLead)[0]
					for _, assumed := range []string{"only you", "on you", "only the operator", "operator's hand", "your hand", "operator's own hand"} {
						if strings.Contains(account, assumed) {
							t.Fatalf("%s assigns the finding in its voice: %q", speaker.Key(), account)
						}
					}
				}
			}
		})
	}
}

func TestCarriedNotificationWithoutNamedRoleNeedsClassification(t *testing.T) {
	for _, executor := range []domain.WorkItemExecutor{domain.WorkItemExecutorConversation, "conversation:unknown-role"} {
		event := fullyRecorded(KindItemAdmitted)
		event.Detail.Executor = string(executor)
		message, err := Render(Product(), Harness(), event)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(message.Body, nextMoveLead+ended(ownership.Unclassified().Whose())) {
			t.Fatalf("carried work with %q rendered as %q", executor, message.Body)
		}
	}
}

func TestSuppliedNotificationOwnershipIsProjectedUnchanged(t *testing.T) {
	answer := readmodel.NotificationOwnership(KindProviderOutage, ownership.Entry{OutageCause: domain.ProviderUnauthenticated})
	if !answer.Owner.IsOperator() || answer.Reason != ownership.ReasonCredential {
		t.Fatalf("credential outage resolved as %+v", answer)
	}
	event := fullyRecorded(KindProviderOutage)
	event.Detail.Mover = answer.Whose()
	message, err := Render(Product(), Harness(), event)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(message.Body, nextMoveLead+ended(answer.Whose())) {
		t.Fatalf("resolved outage rendered as %q", message.Body)
	}
}

func TestRecordedIntakeHoldKeepsItsRegistryOwner(t *testing.T) {
	for _, holder := range []runstate.IntakeHolder{runstate.IntakeHolderOperator, runstate.IntakeHolderBrake} {
		hold := runstate.IntakeHold{HeldBy: holder, HeldAt: moment}
		notice := FromIntakeHold(hold)
		answer := readmodel.IntakeHoldWhose(hold)
		if notice.Event.Detail.Mover != answer {
			t.Fatalf("%s hold carried %q, want %q", holder, notice.Event.Detail.Mover, answer)
		}
		message, err := Render(notice.Topic, notice.Speaker, notice.Event)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(message.Body, nextMoveLead+ended(answer)) {
			t.Fatalf("%s hold rendered as %q", holder, message.Body)
		}
	}
}
