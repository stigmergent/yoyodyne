package execution

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// heldAfterReply stands in for the two clocks a final reply starts, told apart
// by the span each is armed with, so a test fires the notice and the bound when
// it chooses rather than out-waiting real timers.
type heldAfterReply struct {
	mutex  sync.Mutex
	clocks map[time.Duration]chan time.Time
	armed  chan time.Duration
}

func newHeldAfterReply() *heldAfterReply {
	return &heldAfterReply{clocks: make(map[time.Duration]chan time.Time), armed: make(chan time.Duration, 4)}
}

func (h *heldAfterReply) arm(span time.Duration) (<-chan time.Time, func()) {
	h.mutex.Lock()
	clock := make(chan time.Time, 1)
	h.clocks[span] = clock
	h.mutex.Unlock()
	h.armed <- span
	return clock, func() {}
}

func (h *heldAfterReply) fire(span time.Duration) {
	h.mutex.Lock()
	clock := h.clocks[span]
	h.mutex.Unlock()
	clock <- time.Now()
}

// awaitArmed waits for both clocks to be armed, which is the runner saying it
// has read the reply.
func (h *heldAfterReply) awaitArmed(t *testing.T) {
	t.Helper()
	for range 2 {
		select {
		case <-h.armed:
		case <-time.After(time.Minute):
			t.Fatal("the runner never armed the clocks a final reply starts")
		}
	}
}

// replied is a Replied func for a helper whose final reply is the line "final
// reply", read the way an adapter reads its stream: by the observer, before
// the runner asks.
type replied struct {
	mutex sync.Mutex
	seen  bool
}

func (r *replied) observe(output Output) {
	if output.Stream == StreamStdout && output.Text == "final reply" {
		r.mutex.Lock()
		r.seen = true
		r.mutex.Unlock()
	}
}

func (r *replied) done() bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.seen
}

// A session that has written its final reply and stays alive on background
// work is not a stall. The idle bound is tripped after the reply and must not
// stop anything; the process is said to be waiting on background processes once
// the notice fires, and ended at the bound with the ending recorded as the
// runner's rather than as a failure or a stall.
func TestOSProcessRunnerEndsWhatOutlivesAFinalReplyWithoutAStall(t *testing.T) {
	t.Parallel()

	idle := newHeldIdleBound()
	clocks := newHeldAfterReply()
	reply := &replied{}
	waiting := make(chan AfterReply, 1)
	command := helperCommand("reply-then-linger", "")
	command.Timeout = time.Hour
	command.IdleTimeout = time.Hour
	command.Replied = reply.done
	command.AfterReplyTimeout = 5 * time.Minute
	command.AfterReplyWaiting = func(account AfterReply) { waiting <- account }

	type outcome struct {
		result ProcessResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := (OSProcessRunner{idle: idle.arm, afterReply: clocks.arm}).Run(context.Background(), command, reply.observe)
		finished <- outcome{result: result, err: err}
	}()
	clocks.awaitArmed(t)
	// Silence after the reply is the turn being over, not a provider that
	// stopped answering: the idle bound firing now stops nothing.
	idle.trip()
	clocks.fire(afterReplyNotice)
	account := <-waiting
	if !account.Waiting() {
		t.Fatalf("the account given while waiting = %+v, want one with no outcome yet", account)
	}
	if said := account.Describe(account.RepliedAt.Add(2 * time.Minute)); said != "reply written, waiting for background processes: 2m of 5m" {
		t.Fatalf("Describe() while waiting = %q", said)
	}
	clocks.fire(5 * time.Minute)
	ran := <-finished
	if ran.err != nil {
		t.Fatalf("Run() error = %v", ran.err)
	}
	if ran.result.Status == ProcessStalled {
		t.Fatalf("Run() status = %q: a process that had written its final reply was recorded as a stall", ran.result.Status)
	}
	if ran.result.Status != ProcessSucceeded {
		t.Fatalf("Run() status = %q, want %q for a turn that ended with its reply", ran.result.Status, ProcessSucceeded)
	}
	got := ran.result.AfterReply
	if got == nil || got.Outcome != AfterReplyEnded || got.BoundSeconds != 300 {
		t.Fatalf("Run() AfterReply = %+v, want the background processes recorded as ended at the 5m bound", got)
	}
	if said := got.Describe(time.Now()); !strings.Contains(said, "were still running at the 5m bound and were ended") {
		t.Fatalf("Describe() once ended = %q", said)
	}
}

// Background work that finishes on its own inside the bound is waited out, and
// what it wrote after the reply is counted and still reaches the observer.
func TestOSProcessRunnerWaitsOutWhatOutlivesAFinalReply(t *testing.T) {
	t.Parallel()

	clocks := newHeldAfterReply()
	reply := &replied{}
	stdin, release := io.Pipe()
	defer release.Close()
	waiting := make(chan AfterReply, 1)
	command := helperCommand("reply-then-finish", "")
	command.Stdin = stdin
	command.Timeout = time.Hour
	command.IdleTimeout = time.Hour
	command.Replied = reply.done
	command.AfterReplyTimeout = 5 * time.Minute
	command.AfterReplyWaiting = func(account AfterReply) { waiting <- account }

	var observed []string
	var mutex sync.Mutex
	type outcome struct {
		result ProcessResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := (OSProcessRunner{afterReply: clocks.arm}).Run(context.Background(), command, func(output Output) {
			reply.observe(output)
			mutex.Lock()
			observed = append(observed, output.Text)
			mutex.Unlock()
		})
		finished <- outcome{result: result, err: err}
	}()
	clocks.awaitArmed(t)
	clocks.fire(afterReplyNotice)
	<-waiting
	if _, err := release.Write([]byte("go\n")); err != nil {
		t.Fatalf("release the helper: %v", err)
	}
	// The runner copies stdin into the process until it ends, and waits for
	// that copy before it returns.
	release.Close()
	ran := <-finished
	if ran.err != nil {
		t.Fatalf("Run() error = %v", ran.err)
	}
	if ran.result.Status != ProcessSucceeded {
		t.Fatalf("Run() status = %q, want %q", ran.result.Status, ProcessSucceeded)
	}
	got := ran.result.AfterReply
	if got == nil || got.Outcome != AfterReplyExited || got.Lines != 1 {
		t.Fatalf("Run() AfterReply = %+v, want the background work recorded as ending on its own after one line", got)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(observed) == 0 || observed[len(observed)-1] != "background work done" {
		t.Fatalf("observed %q, want what the background work said after the reply", observed)
	}
}

// Without a reply the stall bound is exactly what it was: a command that never
// said it replied and went silent is still stopped as stalled.
func TestOSProcessRunnerStillStallsAProcessThatNeverReplied(t *testing.T) {
	t.Parallel()

	idle := newHeldIdleBound()
	command := helperCommand("sleep", "")
	command.Timeout = time.Hour
	command.IdleTimeout = time.Hour
	command.Replied = func() bool { return false }
	command.AfterReplyTimeout = 5 * time.Minute
	finished := make(chan ProcessResult, 1)
	go func() {
		result, _ := (OSProcessRunner{idle: idle.arm}).Run(context.Background(), command, nil)
		finished <- result
	}()
	<-idle.armed
	idle.trip()
	result := <-finished
	if result.Status != ProcessStalled || result.AfterReply != nil {
		t.Fatalf("Run() = status %q, after reply %+v; want a stall and no account of a reply", result.Status, result.AfterReply)
	}
}
