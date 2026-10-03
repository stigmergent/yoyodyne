package readmodel

import (
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type machineHistory struct{ observations []runstate.MachineObservation }

func (m machineHistory) MachineHistory() ([]runstate.MachineObservation, error) {
	return m.observations, nil
}

func TestMissedPassCausesComeFromTheGapRatherThanThePreviousFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 16, 52, 0, 0, time.UTC)
	from := now.Add(-3 * time.Hour)
	for _, test := range []struct {
		name         string
		observations []runstate.MachineObservation
		passes       []runstate.WatchTransition
		sweeps       []runstate.Sweep
		want         string
	}{
		{"asleep", []runstate.MachineObservation{{At: from, Watching: true, Power: []runstate.PowerEvent{{At: from, Source: "pmset: Sleep"}, {At: now.Add(-time.Minute), Awake: true, Source: "pmset: Wake"}}}}, nil, nil, "the machine was asleep"},
		{"down", []runstate.MachineObservation{{At: from}, {At: now, Watching: true}}, nil, nil, "the harness was not watching"},
		{"waiting", []runstate.MachineObservation{{At: from, Watching: true}}, []runstate.WatchTransition{{At: from, SessionID: "one", RecurringPass: &runstate.WatchPass{Task: "another-pass", At: from}}}, []runstate.Sweep{{Task: "another-pass", StartedAt: from, EndedAt: now}}, "waiting its turn behind the recurring pass of another-pass"},
		{"unknown", []runstate.MachineObservation{{At: from, Watching: true}}, nil, []runstate.Sweep{{Task: "owed-pass", StartedAt: from.Add(-time.Hour), EndedAt: from, Problem: "previous pass failed"}}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			availability := ReadWatchAvailability(Sources{Machine: machineHistory{test.observations}, Sessions: fakeSessions{transitions: test.passes}, Sweeps: fakeSweeps{recorded: test.sweeps}, Now: func() time.Time { return now }})
			why := availability.Explain(from, now, "owed-pass")
			if test.want == "" && why != "" || test.want != "" && !strings.Contains(why, test.want) {
				t.Fatalf("cause = %q, want %q", why, test.want)
			}
			if strings.Contains(why, "previous pass failed") {
				t.Fatal("previous failure was blamed for this gap")
			}
			if test.name == "waiting" && !availability.WaitingBehindPass(from, now, "owed-pass") {
				t.Fatal("serial pass wait was not identified")
			}
		})
	}
}

func TestServicesSleepAndDowntimeAreCountedOnceAndUseLocalTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 16, 52, 0, 0, time.UTC)
	from := now.Add(-3 * time.Hour)
	availability := ReadWatchAvailability(Sources{Machine: machineHistory{[]runstate.MachineObservation{{At: from, Power: []runstate.PowerEvent{{At: from}, {At: now.Add(-time.Hour), Awake: true}}}, {At: now, Watching: true}}}, Now: func() time.Time { return now }})
	if availability.LastGap != 3*time.Hour {
		t.Fatalf("overlapping sleep and downtime = %s, want 3h", availability.LastGap)
	}
	rendered := (Standing{Services: &Services{Recorded: true, SupervisorRunning: true, Availability: &availability}}).RenderServices()
	for _, want := range []string{"last machine sleep: " + localMoment(from), "last machine wake: " + localMoment(now.Add(-time.Hour)), "without the harness watching: 3h0m0s"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("services = %s, want %s", rendered, want)
		}
	}
	unknown := ReadWatchAvailability(Sources{})
	if unknown.Problem == "" || unknown.Explain(from, now, "owed-pass") != "" {
		t.Fatalf("missing history invented a cause: %+v", unknown)
	}
}

func TestACompletedPassDoesNotAccountForALaterMiss(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 16, 52, 0, 0, time.UTC)
	start := now.Add(-4 * time.Hour)
	availability := ReadWatchAvailability(Sources{Machine: machineHistory{}, Sessions: fakeSessions{transitions: []runstate.WatchTransition{{At: start, RecurringPass: &runstate.WatchPass{Task: "earlier-pass", At: start}}}}, Sweeps: fakeSweeps{recorded: []runstate.Sweep{{Task: "earlier-pass", StartedAt: start, EndedAt: start.Add(time.Hour)}}}, Now: func() time.Time { return now }})
	if why := availability.Explain(now.Add(-time.Hour), now, "owed-pass"); why != "" {
		t.Fatalf("earlier pass blamed: %s", why)
	}
}

func TestMissingWakeAndPassEndingLeaveTheirDurationsUnknown(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 30, 16, 52, 0, 0, time.UTC)
	start := now.Add(-3 * time.Hour)
	availability := ReadWatchAvailability(Sources{
		Machine:  machineHistory{[]runstate.MachineObservation{{At: now, Watching: true, Power: []runstate.PowerEvent{{At: start, Source: "pmset: Sleep"}}}}},
		Sessions: fakeSessions{transitions: []runstate.WatchTransition{{At: start, RecurringPass: &runstate.WatchPass{Task: "unfinished-pass", At: start}}}},
		Now:      func() time.Time { return now },
	})
	if !availability.LastSleep.Equal(start) || availability.LastGap != 0 || !strings.Contains(availability.Problem, "duration is unknown") {
		t.Fatalf("missing wake invented a duration: %+v", availability)
	}
	why := availability.Explain(start, now, "owed-pass")
	if !strings.Contains(why, "ending is unrecorded") || strings.Contains(why, "was asleep from") || availability.WaitingBehindPass(start, now, "owed-pass") {
		t.Fatalf("missing pass ending invented a cause: %s", why)
	}
}
