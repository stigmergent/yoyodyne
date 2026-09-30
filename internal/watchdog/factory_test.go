package watchdog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/report"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/sweep"
)

type factoryRuns []runstate.State

func (r factoryRuns) Recorded() ([]runstate.State, error) { return r, nil }

type factoryPasses struct{ passes []runstate.Sweep }

func (p *factoryPasses) List() ([]runstate.Sweep, []runstate.UnreadableSweep, error) {
	return p.passes, nil, nil
}

type factoryHold struct{ held bool }

func (h factoryHold) Held() (runstate.OperatorHold, bool, error) {
	return runstate.OperatorHold{}, h.held, nil
}

type factoryPile struct{ filed []report.Report }

func (p *factoryPile) Append(reported report.Report) error {
	p.filed = append(p.filed, reported)
	return nil
}

var factoryPulled = time.Date(2026, 9, 30, 4, 51, 0, 0, time.UTC)

// newFactoryWatch is a product whose last run started at factoryPulled and
// whose development manager's sweep has failed on the tracker every hour since.
func newFactoryWatch(t *testing.T) (*FactoryWatch, *factoryPasses, *factoryPile) {
	t.Helper()
	stalls, err := runstate.NewFactoryStallStore(t.TempDir(), "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	passes := &factoryPasses{}
	for hour := 1; hour <= 12; hour++ {
		passes.passes = append(passes.passes, runstate.Sweep{
			Task: "development-manager-sweep", Role: domain.RoleDevelopmentManager,
			StartedAt: factoryPulled.Add(time.Duration(hour) * time.Hour),
			Problem:   "list work items: bd list timed out after 30s",
		})
	}
	pile := &factoryPile{}
	return &FactoryWatch{
		Runs:        factoryRuns{{RunID: "run-last", StartedAt: factoryPulled}},
		Passes:      passes,
		Holds:       factoryHold{},
		Stalls:      stalls,
		Reports:     pile,
		Attribution: report.Attribution{ProductID: "yoyodyne", RepositoryID: "yoyodyne"},
		Limit:       2 * time.Hour,
	}, passes, pile
}

func TestAStallUnderTheLimitFilesNothing(t *testing.T) {
	t.Parallel()
	watch, _, pile := newFactoryWatch(t)
	reading, err := watch.Check(context.Background(), factoryPulled.Add(2*time.Hour-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if reading.Stall != nil || reading.Opened != nil || reading.Standing != nil || len(pile.filed) != 0 {
		t.Fatalf("Check() = %+v and filed %d, want nothing inside the limit", reading, len(pile.filed))
	}
}

// Past the limit the stall is filed as a critical report the harness itself
// files, naming how long, the last success, and each pass's failure — and
// every reading after it, for as long as it stands, files nothing more.
func TestAStallPastTheLimitIsFiledOnceAsACriticalReport(t *testing.T) {
	t.Parallel()
	watch, _, pile := newFactoryWatch(t)
	now := factoryPulled.Add(2*time.Hour + time.Minute)
	reading, err := watch.Check(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Opened == nil || reading.Standing == nil || reading.Stall == nil {
		t.Fatalf("Check() = %+v, want the stall opened", reading)
	}
	if len(pile.filed) != 1 {
		t.Fatalf("filed %d reports, want one", len(pile.filed))
	}
	filed := pile.filed[0]
	if filed.Role != report.HarnessReporter || filed.Severity != report.SeverityCritical {
		t.Fatalf("filed %+v, want a critical report in the harness's own voice", filed)
	}
	for _, want := range []string{"2h1m0s", "last pulled at", "development-manager-sweep", "bd list timed out"} {
		if !strings.Contains(filed.Message, want) {
			t.Errorf("message = %q, want it to carry %q", filed.Message, want)
		}
	}
	if !strings.HasPrefix(filed.RunID, "factory-stall@") {
		t.Errorf("run id = %q, want the stall named as what the report came from", filed.RunID)
	}

	// Ten hours more of readings a minute apart, the supervisor's own pace, say
	// nothing more.
	for check := 0; check < 600; check++ {
		now = now.Add(time.Minute)
		reading, err := watch.Check(context.Background(), now)
		if err != nil {
			t.Fatal(err)
		}
		if reading.Opened != nil || reading.Standing == nil {
			t.Fatalf("check %d = %+v, want the one stall standing and nothing opened", check, reading)
		}
	}
	if len(pile.filed) != 1 {
		t.Fatalf("filed %d reports across the stall, want the one", len(pile.filed))
	}
}

// The first successful pass closes the stall and files its recovery once, as
// a note; the next stall is a new one.
func TestARecoveryIsReportedOnceWhenAPassSucceedsAgain(t *testing.T) {
	t.Parallel()
	watch, passes, pile := newFactoryWatch(t)
	now := factoryPulled.Add(12*time.Hour + 30*time.Minute)
	if _, err := watch.Check(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	passes.passes = append(passes.passes, runstate.Sweep{
		Task: "development-manager-sweep", Role: domain.RoleDevelopmentManager,
		StartedAt: now.Add(time.Minute), Turns: 1, Result: &sweep.Result{Status: sweep.StatusComplete, Summary: "the tracker answers again"},
	})
	now = now.Add(2 * time.Minute)
	reading, err := watch.Check(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Closed == nil || reading.Standing != nil {
		t.Fatalf("Check() = %+v, want the stall closed", reading)
	}
	if !strings.Contains(reading.Closed.Cleared, "development-manager-sweep succeeded") {
		t.Fatalf("Cleared = %q, want the pass that ended it named", reading.Closed.Cleared)
	}
	if len(pile.filed) != 2 || pile.filed[1].Severity != report.SeverityNote || !strings.Contains(pile.filed[1].Message, "has cleared") {
		t.Fatalf("filed = %+v, want the stall and then its recovery as a note", pile.filed)
	}
	if _, err := watch.Check(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(pile.filed) != 2 {
		t.Fatalf("filed %d reports, want the recovery said once", len(pile.filed))
	}
	events, err := watch.Stalls.List()
	if err != nil || len(events) != 1 || events[0].Open() {
		t.Fatalf("List() = %+v, %v, want the one stall, closed", events, err)
	}
}

// The supervisor calls Look every few seconds; it reads once a minute.
func TestTheSupervisorsLookIsPaced(t *testing.T) {
	t.Parallel()
	watch, _, pile := newFactoryWatch(t)
	now := factoryPulled.Add(3 * time.Hour)
	watch.Look(context.Background(), now)
	if len(pile.filed) != 1 {
		t.Fatalf("filed %d, want the stall filed at the first look", len(pile.filed))
	}
	// A look inside the pace reads nothing.
	before := watch.lastLook
	watch.Look(context.Background(), now.Add(5*time.Second))
	if !watch.lastLook.Equal(before) {
		t.Fatalf("lastLook moved inside the pace")
	}
	watch.Look(context.Background(), now.Add(DefaultFactoryEvery))
	if !watch.lastLook.Equal(now.Add(DefaultFactoryEvery)) {
		t.Fatalf("lastLook = %s, want the look a minute on to read", watch.lastLook)
	}
}

// The operator pausing the harness during a stall is not a recovery, and
// lifting the pause is not a second stall: the one stall stands through both,
// with no recovery note and no second critical report.
func TestAPauseDuringAStallFilesNoRecoveryAndNoSecondStall(t *testing.T) {
	t.Parallel()
	watch, _, pile := newFactoryWatch(t)
	hold := &factoryHold{}
	watch.Holds = hold
	now := factoryPulled.Add(3 * time.Hour)
	opened, err := watch.Check(context.Background(), now)
	if err != nil || opened.Opened == nil {
		t.Fatalf("Check() = %+v, %v, want the stall opened", opened, err)
	}

	hold.held = true
	for check := 0; check < 30; check++ {
		now = now.Add(time.Minute)
		reading, err := watch.Check(context.Background(), now)
		if err != nil {
			t.Fatal(err)
		}
		if reading.Closed != nil || reading.Opened != nil || reading.Standing == nil || reading.Standing.EventID != opened.Opened.EventID {
			t.Fatalf("paused check %d = %+v, want the one stall left standing and untouched", check, reading)
		}
	}

	hold.held = false
	now = now.Add(time.Minute)
	reading, err := watch.Check(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Opened != nil || reading.Standing == nil || reading.Standing.EventID != opened.Opened.EventID {
		t.Fatalf("Check() after the pause = %+v, want the same stall standing", reading)
	}
	if len(pile.filed) != 1 || pile.filed[0].Severity != report.SeverityCritical {
		t.Fatalf("filed = %+v, want the one critical report and nothing else", pile.filed)
	}
	events, err := watch.Stalls.List()
	if err != nil || len(events) != 1 || !events[0].Open() {
		t.Fatalf("List() = %+v, %v, want the one stall, still open", events, err)
	}
}

// A limit raised past the stall stops the reading calling it one, and nothing
// has recovered either, so the record is left open and nothing is filed.
func TestALongerLimitIsNotARecovery(t *testing.T) {
	t.Parallel()
	watch, _, pile := newFactoryWatch(t)
	now := factoryPulled.Add(3 * time.Hour)
	if _, err := watch.Check(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	watch.Limit = 24 * time.Hour
	reading, err := watch.Check(context.Background(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if reading.Closed != nil || reading.Standing == nil || len(pile.filed) != 1 {
		t.Fatalf("Check() = %+v and filed %d, want the stall left standing and no recovery", reading, len(pile.filed))
	}
}
