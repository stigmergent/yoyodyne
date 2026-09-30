package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// unansweredListing is what a listing says once bd has been killed at its bound
// on every attempt the tracker client makes, in the client's own words.
// sessionMoment is when the passes here begin.
var sessionMoment = time.Date(2026, 9, 30, 4, 52, 8, 0, time.UTC)

const unansweredListing = "bd list did not answer within its 30s bound on any of 3 attempts over 1m40s: bd list failed with status timed_out and exit code -1: "

// passingTasks is a recurring schedule whose every firing begins a pass of the
// development manager's sweep, announced exactly as the real trigger announces
// one, and takes a turn.
type passingTasks struct {
	mu      sync.Mutex
	firings int
}

func (p *passingTasks) Fire(ctx context.Context) (RecurringSweep, error) {
	p.mu.Lock()
	p.firings++
	p.mu.Unlock()
	announcePass(ctx, runstate.WatchPass{
		Task: "development-manager-sweep", Role: domain.RoleDevelopmentManager,
		Trigger: runstate.PassTriggerSchedule, At: sessionMoment,
	})
	return RecurringSweep{Fired: []Fired{{Task: "development-manager-sweep", Role: domain.RoleDevelopmentManager, Turns: 1}}}, nil
}

func (p *passingTasks) fired() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.firings
}

// The widened half of yoyodyne-ifd.433.20, replayed: a tracker whose listing
// times out on every attempt, under a watching session with a recurring task
// due at every poll. The session carries on — it fires the pass at every poll
// whatever the listing does, since the pass comes before the queue is read —
// and it never goes silent: each pass it begins is a line naming the pass and
// since when, and each poll whose listing failed is a line saying the store is
// being read again and what it said. Neither is left for a reader to infer from
// nothing being written, which is what the log did for twelve hours on
// 2026-09-29.
func TestAWatchThroughAListingThatTimesOutOnEveryAttemptCarriesOnAndSaysSo(t *testing.T) {
	t.Parallel()

	harness := newScheduleHarness(readyItems("yoyodyne-one")...)
	harness.failList = func(*scheduleHarness, int) error { return errors.New(unansweredListing) }
	tasks := &passingTasks{}
	harness.recurring = tasks
	harness.onSleep = func(_ *scheduleHarness, sleeps int) bool { return sleeps < 3 }
	sessions := &recordedSessions{}

	schedule, err := Scheduler{Open: harness.open, Watching: true, Sleep: harness.sleep, Sessions: sessions, Now: harness.clock}.Schedule(context.Background())
	if err != nil {
		t.Fatalf("Schedule() error = %v, want the session to ride the listing through", err)
	}
	if schedule.Stopped != ScheduleCancelled {
		t.Fatalf("stopped = %q, want the session ended by its operator rather than by the listing", schedule.Stopped)
	}
	if fired := tasks.fired(); fired < 3 {
		t.Fatalf("the recurring task fired %d time(s) across 3 polls, want a pass at every poll whatever the listing did", fired)
	}
	if len(schedule.Fired) < 3 {
		t.Fatalf("schedule fired %d pass(es), want each on the schedule", len(schedule.Fired))
	}

	var passes, retries int
	for _, transition := range sessions.recorded() {
		switch {
		case transition.pass != nil:
			passes++
			if transition.state != runstate.WatchWatching || transition.pass.Task != "development-manager-sweep" ||
				!strings.Contains(transition.reason, "taking the recurring pass of development-manager-sweep since") ||
				!strings.Contains(transition.reason, "pulls nothing more until this pass ends") {
				t.Fatalf("pass note = %#v, want a watching note naming the pass, since when, and that nothing is pulled meanwhile", transition)
			}
		case transition.unreadable:
			retries++
			if !strings.Contains(transition.reason, "did not answer within its 30s bound on any of 3 attempts") {
				t.Fatalf("retry line = %q, want what the listing said", transition.reason)
			}
		}
	}
	if passes < 3 {
		t.Fatalf("recorded %d pass note(s), want one for each pass begun: %#v", passes, sessions.recorded())
	}
	if retries == 0 {
		t.Fatalf("no line said the store was being read again: %#v", sessions.recorded())
	}
}
