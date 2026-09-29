package execution

import (
	"fmt"
	"time"
)

// AfterReplyOutcome is how a process that outlived its own final reply ended.
type AfterReplyOutcome string

const (
	// AfterReplyExited is a process that was still running after its final reply
	// and ended on its own inside the bound it was given.
	AfterReplyExited AfterReplyOutcome = "exited"
	// AfterReplyEnded is a process still running when that bound ran out, whose
	// tree the runner then ended itself.
	AfterReplyEnded AfterReplyOutcome = "ended"
)

// afterReplyNotice is how long a process may go on after its final reply before
// it counts as having outlived it. A provider's CLI writes its terminal and
// exits a moment later, and a notice given for every one of those would put
// "waiting for background processes" on every run for the milliseconds before
// the exit; what is worth saying is a process still there after this.
const afterReplyNotice = 10 * time.Second

// AfterReply is the account of a process that was still running after it had
// written its final reply: a provider session that finished its turn while work
// it started in the background — a test suite, a race run — kept the process, or
// its pipes, open.
//
// It exists because the runner used to read that state as silence. The reply
// was written, nothing further came, and the idle bound stopped the process as
// stalled — so a turn that had ended was reported as a provider that stopped
// answering, and spent the continuation a stall is owed. On 2026-09-28 run
// run-008b0e25 wrote its final reply at 11:58:33 PDT and was stopped as a stall
// five minutes later over the make test and make race it had backgrounded.
//
// A final reply ends the turn. What is left is waited out to Bound, with every
// line it writes still reaching the observer and so the invocation's record,
// and ended at the bound if it is still running; Outcome says which. It is
// absent from a result whose process ended within afterReplyNotice of its reply,
// which is every ordinary invocation.
type AfterReply struct {
	RepliedAt    time.Time `json:"replied_at"`
	BoundSeconds int64     `json:"bound_seconds,omitempty"`
	// Outcome is empty while the process is still being waited out, which is the
	// only time a record holds one without it.
	Outcome       AfterReplyOutcome `json:"outcome,omitempty"`
	WaitedSeconds int64             `json:"waited_seconds,omitempty"`
	// Lines is how many lines of output the process wrote after its reply. They
	// went where every other line went, so they are in the invocation's record
	// rather than here.
	Lines int `json:"lines,omitempty"`
}

// Waiting reports an account whose process is still being waited out.
func (a AfterReply) Waiting() bool {
	return a.Outcome == ""
}

// Bound is the wait the process was given, as a span.
func (a AfterReply) Bound() time.Duration {
	return time.Duration(a.BoundSeconds) * time.Second
}

// Describe says where the wait stands in the words every surface uses for it:
// while it lasts, what it has spent of its bound; once it is over, which way it
// ended.
func (a AfterReply) Describe(now time.Time) string {
	switch a.Outcome {
	case AfterReplyExited:
		return fmt.Sprintf("reply written at %s; the background processes it left ended on their own after %s%s",
			a.RepliedAt.UTC().Format(time.RFC3339), spanOf(time.Duration(a.WaitedSeconds)*time.Second), a.linesSaid())
	case AfterReplyEnded:
		return fmt.Sprintf("reply written at %s; the background processes it left were still running at the %s bound and were ended%s",
			a.RepliedAt.UTC().Format(time.RFC3339), spanOf(a.Bound()), a.linesSaid())
	}
	said := "reply written, waiting for background processes"
	spent := now.Sub(a.RepliedAt)
	if spent < 0 {
		spent = 0
	}
	if a.BoundSeconds > 0 {
		return said + fmt.Sprintf(": %s of %s", spanOf(spent), spanOf(a.Bound()))
	}
	return said + fmt.Sprintf(": %s", spanOf(spent))
}

func (a AfterReply) linesSaid() string {
	if a.Lines == 0 {
		return "; they wrote nothing after the reply"
	}
	return fmt.Sprintf("; the %d line(s) they wrote after the reply are in the invocation's record", a.Lines)
}

// spanOf says a span the way an operator reads one: whole minutes once it is
// minutes, and seconds under that.
func spanOf(span time.Duration) string {
	if span < time.Minute {
		return fmt.Sprintf("%ds", int(span.Seconds()))
	}
	return fmt.Sprintf("%dm", int(span.Minutes()))
}
