package supervise

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

func TestOSPowerEventsExcludeRequestsAndAcknowledgments(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		line         string
		found, awake bool
	}{
		{"2026-09-29 21:15:00 -0700 Sleep Entering Sleep state due to 'Clamshell Sleep'", true, false},
		{"2026-09-30 08:52:00 -0700 Wake Wake from Deep Idle due to UserActivity", true, true},
		{"2026-09-30 07:10:00 -0700 DarkWake DarkWake from Deep Idle due to Maintenance", true, true},
		{"2026-09-30 08:52:00 -0700 Wake DarkWake to FullWake from Deep Idle due to UserActivity", true, true},
		{"2026-09-30 07:10:01 -0700 Sleep Entering DarkWake state due to Notification", false, false},
		{"2026-09-29 21:15:01 -0700 Wake Requests wakeAt=2026-09-30 08:52:00", false, false},
		{"2026-09-30 08:52:00 -0700 Kernel Client Acks Delays to Wake notifications", false, false},
	} {
		event, found, err := parsePowerEvent(test.line)
		if err != nil || found != test.found || (found && event.Awake != test.awake) {
			t.Errorf("parse %q = %+v, %v, %v", test.line, event, found, err)
		}
		if found {
			_, offset := event.At.Zone()
			if offset != -7*60*60 {
				t.Errorf("lost OS offset: %s", event.At)
			}
		}
	}
	if _, _, err := parsePowerEvent("bad-date 08:52:00 -0700 Wake Wake from Deep Idle"); err == nil {
		t.Fatal("malformed OS event timestamp was accepted")
	}
}

func TestSupervisorBackfillsPowerHistoryAndKeepsItAcrossRestart(t *testing.T) {
	t.Parallel()
	store, err := runstate.NewSupervisionStore(t.TempDir(), "example")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 16, 52, 0, 0, time.UTC)
	events := []runstate.PowerEvent{{At: now.Add(-11 * time.Hour), Source: "pmset: Sleep"}, {At: now.Add(-time.Minute), Awake: true, Source: "pmset: Wake"}}
	child := &fakeChild{name: config.ServiceScheduler, running: true}
	reads := 0
	power := func(context.Context) ([]runstate.PowerEvent, error) { reads++; return events, nil }
	first := Supervisor{Records: store, Children: []Child{child}, PowerHistory: power}
	first.observeMachine(context.Background(), now)
	first.observeMachine(context.Background(), now.Add(30*time.Second))
	if reads != 1 {
		t.Fatalf("read OS history %d times inside one minute", reads)
	}
	restarted := Supervisor{Records: store, Children: []Child{child}, PowerHistory: power}
	restarted.observeMachine(context.Background(), now.Add(time.Minute))
	history, err := store.MachineHistory()
	if err != nil || len(history) != 2 || len(history[0].Power) != 2 || len(history[1].Power) != 0 || !history[1].Watching {
		t.Fatalf("history: %+v, %v", history, err)
	}
	restarted.PowerHistory = func(context.Context) ([]runstate.PowerEvent, error) { return nil, errors.New("OS history refused") }
	restarted.observeMachine(context.Background(), now.Add(2*time.Minute))
	history, err = store.MachineHistory()
	if err != nil || history[2].PowerProblem != "OS history refused" {
		t.Fatalf("unavailable history: %+v, %v", history, err)
	}
}
