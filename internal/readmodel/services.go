package readmodel

// The parts of the product, as the supervisor last saw them.
//
// The supervisor starts every enabled part and keeps it running, and a part
// that keeps failing is left down and reported as degraded — through the
// standing surfaces, the design says, rather than through a restart loop. This
// is where that reaches them: the supervisor's own record, read beside its
// lease, carried whole for the surfaces that read the model, and said on the
// attention line for each child the supervisor has given up on, because a
// part of the product that is down and is not coming back is waiting on a
// person whatever else the machine is doing.

import (
	"fmt"
	"strings"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/supervise"
)

// Supervision is the product's supervisor as the read model asks about it:
// whether one is running, and what it last recorded about the children. It is
// satisfied by *runstate.SupervisionStore.
type Supervision interface {
	Running() (bool, error)
	Load() (runstate.Supervision, bool, error)
}

// Services is the product's parts and its supervisor, as one reading.
type Services struct {
	// SupervisorRunning is the lease's answer, now, rather than anything the
	// record says about itself.
	SupervisorRunning bool `json:"supervisor_running"`
	// Recorded reports whether a supervisor has ever written the record; the
	// record below is meaningful only where it has.
	Recorded bool `json:"recorded"`
	// Record is what the supervisor last knew, in the supervisor's own type so
	// every surface reads one vocabulary of child states.
	Record runstate.Supervision `json:"record"`
}

// readServices reads the supervisor's lease and record. A reading with no
// supervision wired says nothing rather than reporting every part off, for
// the reason every other optional source here does.
func readServices(sources Sources) (*Services, string) {
	if sources.Supervision == nil {
		return nil, ""
	}
	services := &Services{}
	var problem string
	running, err := sources.Supervision.Running()
	if err != nil {
		problem = joinProblems(problem, fmt.Sprintf("whether the product's supervisor is running could not be read: %v", err))
	}
	services.SupervisorRunning = running
	recorded, found, err := sources.Supervision.Load()
	if err != nil {
		problem = joinProblems(problem, fmt.Sprintf("the supervisor's record of the product's parts could not be read: %v", err))
		return services, problem
	}
	services.Recorded = found
	services.Record = recorded
	return services, problem
}

// Attention is each child the supervisor has left down, as something waiting
// on a person: the supervisor's own record of it, carried whole, from which
// the line says the reason it was left down and what brings it back.
func (s *Services) Attention() []Attention {
	if s == nil || !s.Recorded {
		return nil
	}
	degraded := s.Record.Degraded()
	attention := make([]Attention, 0, len(degraded))
	for _, child := range degraded {
		attention = append(attention, degradedServiceAttention(child))
	}
	return attention
}

// RenderServices is the product's parts under the four lines, one line each,
// in the words `yoyo start` prints them in: how each stands, which build it is
// on and since when, and where a deploy is moving it. It is not a fifth line —
// a part left down is already on the attention line — and it is empty for a
// product no supervisor has ever run for, which is a product that does not use
// one rather than one whose parts are all off.
func (s Standing) RenderServices() string {
	services := s.Services
	if services == nil || (!services.Recorded && !services.SupervisorRunning) {
		if s.ServicesProblem == "" {
			return ""
		}
		return "Services:\n" + partialRead + s.ServicesProblem + "\n"
	}
	var rendered strings.Builder
	record := services.Record
	switch {
	case services.SupervisorRunning && services.Recorded:
		fmt.Fprintf(&rendered, "Services (supervisor running as pid %d", record.PID)
	case services.SupervisorRunning:
		rendered.WriteString("Services (supervisor running, and it has not recorded the parts yet")
	default:
		fmt.Fprintf(&rendered, "Services (no supervisor is running; `yoyo start` starts one; as it last recorded them at %s", localMoment(record.ObservedAt))
	}
	if record.Deployed != "" {
		fmt.Fprintf(&rendered, "; the binary on disk is build %s", shortRevision(record.Deployed))
	}
	rendered.WriteString("):\n")
	if services.Recorded {
		for _, child := range record.Children {
			fmt.Fprintf(&rendered, "  %s\n", supervise.DescribeChild(child))
		}
	}
	if s.ServicesProblem != "" {
		rendered.WriteString(partialRead + s.ServicesProblem + "\n")
	}
	return rendered.String()
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
