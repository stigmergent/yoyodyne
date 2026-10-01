package supervise

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// While the provider cannot be reached or is not logged in, a part behind the
// deployed build is left on its build and the record says why; once the
// provider answers, it is moved. A part that dies is still started again
// through the outage, because a part left dead is one nothing is left to
// notice the outage ending.
func TestTheProviderGuardHoldsDeployRestartsAndNotTheRestartOfADeadPart(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	clock := &clock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	binary := newBinary(t, "1111111111111111")
	sink := newDeployable(config.ServiceSlack, binary, nil)
	supervisor := deploySupervisor(t, store, clock, binary, sink)
	held := "the provider is not logged in"
	supervisor.RestartHold = func() string { return held }

	supervisor.Tick(context.Background())
	binary.deploy(t, "2222222222222222")
	clock.advance(DeployEvery)
	supervisor.Tick(context.Background())
	if sink.stopCount() != 0 {
		t.Fatalf("the sink was stopped %d time(s) during an outage, want it left on its build", sink.stopCount())
	}
	if got := child(t, loaded(t, store), config.ServiceSlack); !strings.Contains(got.Redeploy, "not restarted while the provider is not logged in") {
		t.Errorf("slack redeploy = %q, want the hold said", got.Redeploy)
	}

	sink.die()
	clock.advance(DeployEvery)
	supervisor.Tick(context.Background())
	clock.advance(2 * time.Second)
	supervisor.Tick(context.Background())
	if !sink.isRunning() || sink.startCount() != 2 {
		t.Fatalf("a sink that died during an outage is running %t after %d start(s), want it started again", sink.isRunning(), sink.startCount())
	}

	held = ""
	binary.deploy(t, "3333333333333333")
	clock.advance(DeployEvery)
	supervisor.Tick(context.Background())
	if got := child(t, loaded(t, store), config.ServiceSlack); got.Build != "3333333333333333" {
		t.Errorf("slack = %+v, want it moved onto the deployed build once the provider answers", got)
	}
}

// A requested restart is a death: the part is stopped, counted against the
// bound, and started again after its backoff. The scheduler is never stopped,
// a degraded part is left down, a part the supervisor does not start is named
// as such, and nothing is restarted while the provider guard holds.
func TestARequestedRestartIsTreatedAsADeath(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	clock := &clock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	binary := newBinary(t, "1111111111111111")
	sink := &fakeChild{name: config.ServiceSlack}
	watch := selfRestartingChild{newDeployable(config.ServiceScheduler, binary, nil)}
	supervisor := newSupervisor(t, store, clock, sink, watch)
	held := ""
	supervisor.RestartHold = func() string { return held }
	supervisor.Tick(context.Background())

	answer := supervisor.RestartOnRequest(context.Background(), config.ServiceSlack, clock.Now())
	if !strings.Contains(answer, "stopped pid 1001") || !strings.Contains(answer, "restart 1 of 5") {
		t.Errorf("answer = %q, want the stop and the bound named", answer)
	}
	got, _ := supervisor.ChildState(config.ServiceSlack)
	if sink.isRunning() || got.State != runstate.ChildDown || got.Failures != 1 || !strings.Contains(got.Reason, "at a program manager's request") {
		t.Fatalf("slack = %+v, running %t, want it stopped, down, and counted", got, sink.isRunning())
	}
	clock.advance(2 * time.Second)
	supervisor.Tick(context.Background())
	if !sink.isRunning() || sink.startCount() != 2 {
		t.Errorf("slack running %t after %d start(s), want it started again after its backoff", sink.isRunning(), sink.startCount())
	}

	if answer := supervisor.RestartOnRequest(context.Background(), config.ServiceScheduler, clock.Now()); !strings.Contains(answer, "left running") || !watch.isRunning() {
		t.Errorf("answer for the scheduler = %q, running %t, want it left running", answer, watch.isRunning())
	}
	if answer := supervisor.RestartOnRequest(context.Background(), config.ServiceDashboard, clock.Now()); !strings.Contains(answer, "not yet a child") {
		t.Errorf("answer for the dashboard = %q, want it named as not a child", answer)
	}

	held = "the provider cannot be reached"
	stops := sink.stops
	if answer := supervisor.RestartOnRequest(context.Background(), config.ServiceSlack, clock.Now()); !strings.Contains(answer, "nothing is restarted while the provider cannot be reached") || sink.stops != stops {
		t.Errorf("answer during an outage = %q, stops %d then %d, want nothing stopped", answer, stops, sink.stops)
	}
	held = ""

	for index := 0; index < MaxRapidFailures; index++ {
		supervisor.RestartOnRequest(context.Background(), config.ServiceSlack, clock.Now())
		clock.advance(MaxRestartBackoff)
		supervisor.Tick(context.Background())
	}
	if got, _ := supervisor.ChildState(config.ServiceSlack); got.State != runstate.ChildDegraded {
		t.Fatalf("slack = %+v after a stream of requests, want it degraded by the bound", got)
	}
	if answer := supervisor.RestartOnRequest(context.Background(), config.ServiceSlack, clock.Now()); !strings.Contains(answer, "degraded and left down") {
		t.Errorf("answer for a degraded part = %q", answer)
	}
}

// A pass that asks the supervisor to take a deployed build up makes Run let its
// lease go and return, naming the build, with its children left running; the
// guard refuses it.
func TestTakingADeployedBuildUpEndsRunWithTheChildrenRunning(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	clock := &clock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	sink := &fakeChild{name: config.ServiceSlack}
	supervisor := newSupervisor(t, store, clock, sink)
	held := "the provider is not logged in"
	supervisor.RestartHold = func() string { return held }
	asker := &takeUpAsker{supervisor: supervisor, into: "def5678"}
	supervisor.Residents = []Resident{asker}
	supervisor.Sleep = func(context.Context, time.Duration) bool { return true }

	if err := supervisor.TakeUp("def5678"); err == nil || !strings.Contains(err.Error(), "not taken up while the provider is not logged in") {
		t.Fatalf("TakeUp() during an outage = %v, want it refused", err)
	}
	if err := supervisor.TakeUp("abc1234"); err == nil {
		t.Fatalf("TakeUp() of the build already running was accepted")
	}
	held = ""
	err := supervisor.Run(context.Background())
	var takeUp *TakeUpError
	if !errors.As(err, &takeUp) || takeUp.Into != "def5678" {
		t.Fatalf("Run() = %v, want it to end to take up def5678", err)
	}
	if !sink.isRunning() {
		t.Errorf("the sink was stopped by the supervisor taking a build up")
	}
	if running, err := store.Running(); err != nil || running {
		t.Errorf("store.Running() = %t, %v after Run returned, want the lease let go", running, err)
	}
}

type takeUpAsker struct {
	supervisor *Supervisor
	into       string
	asked      bool
}

func (a *takeUpAsker) Name() string { return "take-up" }

func (a *takeUpAsker) Look(context.Context, time.Time) {
	if a.asked {
		return
	}
	a.asked = true
	_ = a.supervisor.TakeUp(a.into)
}

// The maintenance pass is recorded as the supervisor's own scheduled part,
// with what it says of itself, and described in the same words everywhere.
func TestAScheduledPartIsRecordedWithItsLine(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	clock := &clock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	supervisor := newSupervisor(t, store, clock, &fakeChild{name: config.ServiceSlack})
	supervisor.Off = nil
	supervisor.Scheduled = []Scheduled{scheduledPass{}}
	supervisor.Tick(context.Background())

	got := child(t, loaded(t, store), config.ServiceMaintenance)
	if got.State != runstate.ChildScheduled || got.Reason != "every 10m0s; next at 2026-09-29T12:10:00Z" {
		t.Fatalf("maintenance = %+v, want it scheduled with its line", got)
	}
	if said := DescribeChild(got); said != "maintenance: the supervisor's own pass, every 10m0s; next at 2026-09-29T12:10:00Z" {
		t.Errorf("DescribeChild = %q", said)
	}
	if hosted := supervisor.Hosted(config.ServiceMaintenance); !strings.Contains(hosted, "a pass the supervisor takes itself") {
		t.Errorf("Hosted(maintenance) = %q", hosted)
	}
}

type scheduledPass struct{}

func (scheduledPass) Service() config.ServiceName { return config.ServiceMaintenance }
func (scheduledPass) Describe() string            { return "every 10m0s; next at 2026-09-29T12:10:00Z" }
