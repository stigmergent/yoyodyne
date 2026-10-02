package ownership

import (
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestUnclaimedRunNotificationKeepsCarryOutOwnership(t *testing.T) {
	ended := time.Unix(10, 0)
	run := runstate.State{RunID: "run-1", WorkItemID: "item-1", Status: runstate.StatusFailed, CompletedAt: &ended, Failure: "claim refused"}
	if !run.DiedBeforeClaiming() {
		t.Fatal("fixture did not fail before claiming")
	}
	entry := Entry{Kind: KindNotification, Notification: NotificationRunEnded, Stopped: &run}
	if got := Resolve(entry); got.Owner != DevelopmentManager || got.Capability != "yoyo triage" {
		t.Fatalf("undecided dispatch resolved as %+v", got)
	}
	entry.AwaitingCarryOut = true
	if got := Resolve(entry); got.Owner != Harness {
		t.Fatalf("decided dispatch resolved as %+v", got)
	}
}

func TestEveryNotificationHasAnAnswerOrAnExplicitMissingRecord(t *testing.T) {
	// These kinds need facts beyond the milestone name. A new kind that lacks
	// both a progress answer and a rule cannot silently take the fallback.
	missing := map[NotificationKind]bool{
		NotificationWorkHandedOff: true, NotificationWorkPickedUp: true,
		NotificationRunParked: true, NotificationProposalRaised: true,
		NotificationExchangeTurn: true, NotificationIntakeHeld: true,
		NotificationIntakeEscalated: true, NotificationOperatorAction: true,
		NotificationWatchIdle: true, NotificationWatchBraked: true,
		NotificationLineWaiting: true, NotificationResidentStale: true,
		NotificationStallNoticed: true, NotificationProviderOutage: true,
		NotificationRecurringTaskFailing: true,
	}
	for _, kind := range NotificationKinds() {
		entry := Entry{Kind: KindNotification, Notification: kind}
		_, classified := notificationRule(entry)
		if classified == missing[kind] {
			t.Errorf("%s: classified = %t, requires missing record = %t", kind, classified, missing[kind])
		}
		if missing[kind] && Resolve(entry) != Unclassified() {
			t.Errorf("%s with no record did not request classification", kind)
		}
	}
	if got := Resolve(Entry{Kind: KindNotification, Notification: "unknown.milestone"}); got != Unclassified() {
		t.Fatalf("unknown milestone resolved as %+v", got)
	}
}
