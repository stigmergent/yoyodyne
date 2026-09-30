package readmodel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/config"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type fakeSupervision struct {
	running     bool
	recorded    runstate.Supervision
	found       bool
	failRunning error
	failLoad    error
}

func (f fakeSupervision) Running() (bool, error) { return f.running, f.failRunning }

func (f fakeSupervision) Load() (runstate.Supervision, bool, error) {
	return f.recorded, f.found, f.failLoad
}

// A part the supervisor has left down is down and not coming back on its own,
// so it is on the attention line with the supervisor's own reason, and the
// record is carried whole beside the lines for the surfaces that read it.
func TestADegradedServiceWaitsOnTheOperatorWithTheSupervisorsReason(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	sources.Supervision = fakeSupervision{
		running: true,
		found:   true,
		recorded: runstate.Supervision{
			PID: 4242,
			Children: []runstate.SupervisedChild{
				{Service: config.ServiceSlack, State: runstate.ChildRunning, PID: 77},
				{Service: config.ServiceScheduler, State: runstate.ChildDegraded, Reason: "died 6 times within 2m0s of being started, most recently at 2026-09-19T12:03:00Z, so it is left down"},
			},
		},
	}
	standing := ReadStanding(context.Background(), sources)
	if standing.Services == nil || !standing.Services.SupervisorRunning || !standing.Services.Recorded || standing.Services.Record.PID != 4242 {
		t.Fatalf("Services = %+v, want the supervisor running and its record carried", standing.Services)
	}
	if len(standing.NeedsHuman) != 1 {
		t.Fatalf("NeedsHuman = %+v, want the one degraded child", standing.NeedsHuman)
	}
	if got := standing.NeedsHuman[0]; !strings.Contains(got.What(), "scheduler service is degraded") || !strings.Contains(got.What(), "died 6 times") || !strings.HasPrefix(got.Whose(), "the harness's") {
		t.Errorf("attention = %+v, want the child, the reason, and whose move it is", got)
	}
	rendered := standing.Render()
	if !strings.Contains(rendered, "Waiting on the harness (1):\n  the scheduler service is degraded: died 6 times") {
		t.Errorf("rendered:\n%s\nwant the degraded child on the attention line", rendered)
	}
}

// A running part is not attention, a product nothing has supervised says so
// rather than reporting parts down, and a reading with nothing wired says
// nothing at all.
func TestARunningServiceAndAnUnsupervisedProductWaitOnNobody(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	sources.Supervision = fakeSupervision{running: true, found: true, recorded: runstate.Supervision{
		PID:      1,
		Children: []runstate.SupervisedChild{{Service: config.ServiceSlack, State: runstate.ChildRunning}},
	}}
	if standing := ReadStanding(context.Background(), sources); len(standing.NeedsHuman) != 0 {
		t.Errorf("a running service was reported as waiting on somebody: %+v", standing.NeedsHuman)
	}

	sources.Supervision = fakeSupervision{}
	standing := ReadStanding(context.Background(), sources)
	if standing.Services == nil || standing.Services.SupervisorRunning || standing.Services.Recorded || len(standing.NeedsHuman) != 0 {
		t.Errorf("an unsupervised product = %+v, %+v, want no supervisor, no record, and nothing waiting", standing.Services, standing.NeedsHuman)
	}

	sources.Supervision = nil
	if standing := ReadStanding(context.Background(), sources); standing.Services != nil {
		t.Errorf("Services = %+v with nothing wired, want nil", standing.Services)
	}
}

// A record that cannot be read is said on the line it would have been said on
// rather than read as every part running.
func TestAnUnreadableSupervisionRecordIsSaidRatherThanAssumedHealthy(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	sources.Supervision = fakeSupervision{failLoad: errors.New("decode supervision record: unexpected EOF")}
	standing := ReadStanding(context.Background(), sources)
	if !strings.Contains(standing.ServicesProblem, "unexpected EOF") || !strings.Contains(standing.NeedsHumanProblem, "unexpected EOF") {
		t.Errorf("problems = %q / %q, want the unreadable record named on both", standing.ServicesProblem, standing.NeedsHumanProblem)
	}
}

// The services line says which build each part is on and when it last moved,
// beside what the binary on disk is, and where a deploy is moving a part.
func TestTheServicesLineSaysWhichBuildEachPartIsOnAndWhenItMoved(t *testing.T) {
	t.Parallel()

	moved := time.Date(2026, 9, 28, 16, 40, 0, 0, time.UTC)
	sources := quietSources()
	sources.Supervision = fakeSupervision{
		running: true,
		found:   true,
		recorded: runstate.Supervision{
			PID:      4242,
			Deployed: "3d3d367a1b2c4d5e",
			Children: []runstate.SupervisedChild{
				{Service: config.ServiceSlack, State: runstate.ChildRunning, PID: 77, Build: "3d3d367a1b2c4d5e", BuildSince: moved, Restarts: 1},
				{Service: config.ServiceScheduler, State: runstate.ChildRunning, PID: 78, Build: "1a2b3c4d5e6f7a8b", BuildSince: moved.Add(-time.Hour),
					Redeploy: "on build 1a2b3c4d5e6f, behind the deployed 3d3d367a1b2c; the watch restarts itself into it between runs"},
			},
		},
	}
	rendered := ReadStanding(context.Background(), sources).RenderServices()
	for _, want := range []string{
		"Services (supervisor running as pid 4242; the binary on disk is build 3d3d367a1b2c):\n",
		"  slack: running as pid 77, on build 3d3d367a1b2c since " + moved.Local().Format("2006-01-02 15:04 MST") + " (restarted into a deployed build once)\n",
		"  scheduler: running as pid 78, on build 1a2b3c4d5e6f since ",
		"; on build 1a2b3c4d5e6f, behind the deployed 3d3d367a1b2c; the watch restarts itself into it between runs\n",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered:\n%s\nwant it to contain %q", rendered, want)
		}
	}
}

// A product no supervisor has run for prints no services line at all.
func TestAnUnsupervisedProductPrintsNoServicesLine(t *testing.T) {
	t.Parallel()

	sources := quietSources()
	sources.Supervision = fakeSupervision{}
	if rendered := ReadStanding(context.Background(), sources).RenderServices(); rendered != "" {
		t.Errorf("rendered %q, want nothing for a product no supervisor has run for", rendered)
	}
}
