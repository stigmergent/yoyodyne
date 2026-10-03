package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/sweep"
	"github.com/mason-bryant/yoyodyne/internal/terms"
)

func TestAPassRecordsItsLanguageFindingsAndTellsTheNextPass(t *testing.T) {
	t.Parallel()
	store := sweepStore(t)
	account := complete("the posture changed")
	account.Questions = []string{"change the cadence?"}
	account.Findings = []sweep.Finding{{Issue: "a pause", Detail: "the idle-bound ended the run", Disposition: sweep.DispositionLeft}}
	role := &wokenRole{answers: []scriptedTurn{
		{result: account, wording: []terms.Finding{
			{Term: "posture", Replacement: "tool access", Retired: true},
			{Term: "stall continuation", Replacement: "the development manager resumed the run", Retired: true},
		}},
		{result: complete("ordinary words")},
	}}
	clock := &movingRecurringClock{now: recurringNow}
	trigger := Trigger{Repository: "../..", Tasks: hourlyTask("look"), Claims: store, Reports: store, Roles: role, Clock: clock}
	if _, err := trigger.Fire(context.Background()); err != nil {
		t.Fatal(err)
	}
	recorded, unreadable, err := store.List()
	if err != nil || len(unreadable) != 0 || len(recorded) != 1 {
		t.Fatalf("List() = %+v, %+v, %v", recorded, unreadable, err)
	}
	if len(recorded[0].Wording) != 4 || recorded[0].Result.Summary != account.Summary {
		t.Fatalf("pass = %+v, want four unique corrections beside the unchanged account, including the turn's report", recorded[0])
	}
	clock.now = recurringNow.Add(time.Hour)
	if _, err := trigger.Fire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(role.messages) != 2 || !strings.Contains(role.messages[1], "Words to correct from your previous pass") || !strings.Contains(role.messages[1], `"posture" was replaced; write tool access`) {
		t.Fatalf("next pass's message = %q, want the correction from the recorded pass", role.messages)
	}
	recorded, _, err = store.List()
	if err != nil || len(recorded) != 2 || len(recorded[1].Wording) != 0 {
		t.Fatalf("next recorded pass = %+v, %v", recorded, err)
	}
}

func TestAFailedTurnWithWordsAlreadyShownStillGetsACorrectionReminder(t *testing.T) {
	t.Parallel()
	pass := runstate.Sweep{Task: "look", Turns: 0, Failed: true, Wording: []terms.Finding{{Term: "posture", Replacement: "tool access", Retired: true}}}
	if message := wordingMessage([]runstate.Sweep{pass}); !strings.Contains(message, `"posture"`) {
		t.Fatalf("the failed turn's words were forgotten: %q", message)
	}
}

func TestTheCorrectionReminderIsBoundedAndCountsWhatItDoesNotList(t *testing.T) {
	t.Parallel()
	pass := runstate.Sweep{Task: "look", Turns: 1}
	for index := 0; index < 20; index++ {
		pass.Wording = append(pass.Wording, terms.Finding{Term: "word", Replacement: strings.Repeat("long definition ", 1000)})
	}
	message := wordingMessage([]runstate.Sweep{pass})
	if len(message) > 4000 || !strings.Contains(message, "and 10 more") {
		t.Fatalf("reminder is %d bytes and does not account for the omitted corrections: %q", len(message), message)
	}
}

func TestAPassThatTookNoTurnDoesNotConsumeTheWordingReminder(t *testing.T) {
	t.Parallel()
	earlier := []runstate.Sweep{
		{Task: "look", Turns: 1, Wording: []terms.Finding{{Term: "posture", Replacement: "tool access", Retired: true}}},
		{Task: "look", Turns: 0, Problem: "the provider refused it"},
	}
	if message := wordingMessage(earlier); !strings.Contains(message, `"posture"`) {
		t.Fatalf("reminder was lost: %q", message)
	}
	earlier = append(earlier, runstate.Sweep{Task: "look", Turns: 1})
	if message := wordingMessage(earlier); message != "" {
		t.Fatalf("an old correction was repeated after the next pass: %q", message)
	}
}
