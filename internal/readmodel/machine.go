package readmodel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

type MachineHistory interface {
	MachineHistory() ([]runstate.MachineObservation, error)
}

// GapCause is the observed account of one interval, including whether the
// harness's own serial pass held it. Empty Why means the cause is unknown.
type GapCause struct {
	Why     string
	Waiting bool
	// Checked means durable history was consulted. A caller must not then
	// assume this session's start proves no earlier session was running.
	Checked bool
}

// WatchAvailability is the shared reading used by missed passes, stopped
// responses and services. All causes are derived from recorded observations.
type WatchAvailability struct {
	Observed  bool          `json:"observed"`
	LastSleep time.Time     `json:"last_sleep,omitempty"`
	LastWake  time.Time     `json:"last_wake,omitempty"`
	LastGap   time.Duration `json:"last_gap,omitempty"`
	Problem   string        `json:"problem,omitempty"`
	sleeps    []watchGap
	down      []watchGap
	passes    []watchGap
}

type watchGap struct {
	from, to time.Time
	task     string
	known    bool
}

type passMoment struct {
	task string
	at   int64
}

func ReadWatchAvailability(sources Sources) WatchAvailability {
	var availability WatchAvailability
	now := time.Now()
	if sources.Now != nil {
		now = sources.Now()
	}
	if sources.Machine == nil {
		availability.Problem = "OS sleep history and scheduler observations are unavailable"
		return availability
	}
	observations, err := sources.Machine.MachineHistory()
	if err != nil {
		availability.Problem = err.Error()
	}
	availability.Observed = len(observations) > 0
	if !availability.Observed {
		availability.Problem = joinProblems(availability.Problem, "no supervisor observation of machine sleep or scheduler presence has been recorded")
	}
	var events []runstate.PowerEvent
	var down time.Time
	for _, observation := range observations {
		events = append(events, observation.Power...)
		if !observation.Watching && down.IsZero() {
			down = observation.At
		}
		if observation.Watching && !down.IsZero() {
			availability.down = append(availability.down, watchGap{from: down, to: observation.At})
			down = time.Time{}
		}
	}
	if fresh, ok := sources.Machine.(interface {
		PowerHistory() ([]runstate.PowerEvent, error)
	}); ok {
		power, err := fresh.PowerHistory()
		if err != nil {
			availability.Problem = joinProblems(availability.Problem, err.Error())
		}
		events = append(events, power...)
	}
	if len(observations) > 0 {
		availability.Problem = joinProblems(availability.Problem, observations[len(observations)-1].PowerProblem)
	}
	if !down.IsZero() {
		availability.down = append(availability.down, watchGap{from: down, to: now})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	var asleep time.Time
	seen := map[string]bool{}
	for _, event := range events {
		key := fmt.Sprintf("%s/%t", event.At.Format(time.RFC3339Nano), event.Awake)
		if seen[key] {
			continue
		}
		seen[key] = true
		if event.Awake {
			availability.LastWake = event.At
			if !asleep.IsZero() {
				availability.sleeps = append(availability.sleeps, watchGap{from: asleep, to: event.At})
				asleep = time.Time{}
			}
		} else {
			availability.LastSleep = event.At
			if asleep.IsZero() {
				asleep = event.At
			}
		}
	}
	if !asleep.IsZero() {
		availability.Problem = joinProblems(availability.Problem, "the OS recorded sleep at "+localMoment(asleep)+" without a following wake; its duration is unknown")
	}
	// A pass's ending comes from its own sweep, rather than assuming the next
	// watch note ended it. Notes can be absent while a session remains idle.
	if sources.Sessions != nil {
		transitions, err := sources.Sessions.List()
		if err != nil {
			availability.Problem = joinProblems(availability.Problem, err.Error())
		}
		transitions = append([]runstate.WatchTransition(nil), transitions...)
		sort.SliceStable(transitions, func(i, j int) bool { return transitions[i].At.Before(transitions[j].At) })
		// Graceful stops have exact durable boundaries even if the supervisor
		// was not running to sample the scheduler's lease.
		var stopped time.Time
		var stoppedSession string
		for _, transition := range transitions {
			if transition.Note() && (stopped.IsZero() || transition.SessionID == stoppedSession) {
				continue
			}
			if transition.State == runstate.WatchStopped {
				if stopped.IsZero() {
					stopped = transition.At
					stoppedSession = transition.SessionID
				}
			} else if !stopped.IsZero() {
				availability.down = append(availability.down, watchGap{from: stopped, to: transition.At})
				stopped = time.Time{}
			}
		}
		if !stopped.IsZero() {
			availability.down = append(availability.down, watchGap{from: stopped, to: now})
		}
		var alive, stops []time.Time
		var openings []runstate.WatchTransition
		opened := map[string]bool{}
		for _, transition := range transitions {
			if transition.State == runstate.WatchStopped {
				stops = append(stops, transition.At)
			}
			if transition.State != runstate.WatchStopped && (!transition.Note() || transition.RecurringPass != nil) {
				alive = append(alive, transition.At)
				if !opened[transition.SessionID] {
					openings = append(openings, transition)
					opened[transition.SessionID] = true
				}
			}
		}
		// A watch opening is finer evidence than the next supervisor sample.
		// This also prevents a stale down sample extending across a restart.
		for i := range availability.down {
			next := sort.Search(len(alive), func(j int) bool { return alive[j].After(availability.down[i].from) })
			if next < len(alive) && alive[next].Before(availability.down[i].to) {
				availability.down[i].to = alive[next]
			}
		}
		endings := map[passMoment]time.Time{}
		if sources.Sweeps != nil {
			sweeps, unreadable, err := sources.Sweeps.List()
			if err != nil || len(unreadable) > 0 {
				availability.Problem = joinProblems(availability.Problem, "pass endings could not be read whole")
			}
			for _, sweep := range sweeps {
				if !sweep.IsMiss() {
					endings[passMoment{sweep.Task, sweep.StartedAt.UnixNano()}] = sweep.EndedAt
				}
			}
		}
		for _, transition := range transitions {
			if pass := transition.RecurringPass; pass != nil {
				end, known := endings[passMoment{pass.Task, pass.At.UnixNano()}]
				if !known {
					end = now
				}
				// A session's stop or replacement bounds an unfinished pass.
				next := sort.Search(len(stops), func(i int) bool { return stops[i].After(pass.At) })
				if next < len(stops) && stops[next].Before(end) {
					end = stops[next]
				}
				next = sort.Search(len(openings), func(i int) bool { return openings[i].At.After(pass.At) })
				if next < len(openings) && openings[next].SessionID != transition.SessionID && openings[next].At.Before(end) {
					end = openings[next].At
				}
				availability.passes = append(availability.passes, watchGap{from: pass.At, to: end, task: pass.Task, known: known})
			}
		}
	}
	// Union the intervals so sleep while the scheduler was down is counted once.
	gaps := append(append([]watchGap{}, availability.sleeps...), availability.down...)
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].from.Before(gaps[j].from) })
	var last watchGap
	for _, gap := range gaps {
		if last.to.Before(gap.from) {
			last = gap
		} else if gap.to.After(last.to) {
			last.to = gap.to
		}
	}
	if !last.from.IsZero() {
		availability.LastGap = last.to.Sub(last.from)
	}
	return availability
}

// Explain names only causes observed during this gap. The previous pass's
// failure is deliberately absent: it is not evidence about this missed pass.
func (a WatchAvailability) Explain(from, to time.Time, task string) string {
	var reasons []string
	for _, gap := range a.sleeps {
		if overlaps(gap, from, to) {
			reasons = append(reasons, fmt.Sprintf("the machine was asleep from %s to %s (%s), according to the OS", localMoment(gap.from), localMoment(gap.to), gap.to.Sub(gap.from).Round(time.Second)))
		}
	}
	for _, gap := range a.down {
		if overlaps(gap, from, to) {
			reasons = append(reasons, fmt.Sprintf("the harness was not watching: the scheduler was observed down from %s to %s", localMoment(gap.from), localMoment(gap.to)))
		}
	}
	for _, gap := range a.passes {
		if task != "" && gap.task != task && overlaps(gap, from, to) {
			if gap.known {
				reasons = append(reasons, fmt.Sprintf("the pass was waiting its turn behind the recurring pass of %s, running from %s to %s", gap.task, localMoment(gap.from), localMoment(gap.to)))
			} else {
				reasons = append(reasons, fmt.Sprintf("the session last recorded taking the recurring pass of %s at %s; its ending is unrecorded, so whether it held this pass is uncertain", gap.task, localMoment(gap.from)))
			}
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, "; ")
}

// WaitingBehindPass distinguishes the harness's own blocked schedule from
// machine sleep and observed downtime, without a caller parsing the prose.
func (a WatchAvailability) WaitingBehindPass(from, to time.Time, task string) bool {
	for _, gap := range a.passes {
		if gap.known && gap.task != task && overlaps(gap, from, to) {
			return true
		}
	}
	return false
}

func overlaps(gap watchGap, from, to time.Time) bool {
	return gap.from.Before(to) && gap.to.After(from)
}
