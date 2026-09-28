package orchestrator

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/beads"
	"github.com/mason-bryant/yoyodyne/internal/domain"
	"github.com/mason-bryant/yoyodyne/internal/execution"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
)

// A re-run the development manager recorded, carried out against an item at
// status blocked whose claim bd refuses on the status even after the harness
// cleared it and read it back as open. On 2026-09-22 and 2026-09-23 that is
// what yoyodyne-ifd.432.10 and yoyodyne-ifd.117.3 met: the carry-out cleared the
// status, bd refused the claim with "issue not claimable: status blocked", and
// only a later pull claimed each item. The claim is retried within the
// read-back's bound, so the fresh run starts on the pull that cleared the
// status, and its record says the claim was refused and then taken.
//
// The claim is the real client's, over a bd scripted to refuse; everything else
// is the real pipeline the carry-out starts.
func TestARerunOfABlockedItemWhoseClaimIsRefusedStartsOnTheSamePull(t *testing.T) {
	t.Parallel()

	pipelined := newPipelinedRerun(t, nil)
	pipelined.tracker.Item.Status = "blocked"
	// The first refusal is the one the claim meets the stale status with; the
	// second is the claim bd refused after the cleared status read back as open.
	bd := &blockedClaimBD{id: docketedItem, refusals: 2}
	client := beads.Client{Runner: bd, Binary: "bd-test", Dir: t.TempDir()}
	pipelined.tracker.OnClaim = func() error {
		_, cleared, err := client.Claim(context.Background(), docketedItem)
		pipelined.tracker.StaleBlockClear = cleared
		return err
	}

	result, err := pipelined.rerunner.Rerun(context.Background(), RerunRequest{Run: priorRunID})
	if err != nil {
		t.Fatalf("Rerun() error = %v, want the fresh run started on this pull", err)
	}
	if !result.Started || result.Outcome.RunID == "" {
		t.Fatalf("result = %#v, want the fresh run started", result)
	}
	if result.Outcome.Status != runstate.StatusSucceeded {
		t.Fatalf("fresh run status = %q, want it to have run to the end: %#v", result.Outcome.Status, result.Outcome)
	}
	if claims := bd.claimCount(); claims != 3 {
		t.Fatalf("bd was asked to claim %d time(s), want the refused one, the one refused after the read-back, and the one taken", claims)
	}
	state, err := pipelined.runs.Load(result.Outcome.RunID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := &runstate.StaleBlockClear{Outcome: domain.StaleBlockClearConfirmedLate, Reads: 2, Status: "open", ClaimsRefused: 1}
	if !reflect.DeepEqual(state.StaleBlockClear, want) {
		t.Fatalf("run record's stale blocked status = %#v, want %#v", state.StaleBlockClear, want)
	}
}

// blockedClaimBD is bd over one item at status blocked whose claim it refuses
// on the status a set number of times, whatever the status reads back as.
type blockedClaimBD struct {
	mu       sync.Mutex
	id       string
	refusals int
	cleared  bool
	claims   int
}

func (b *blockedClaimBD) claimCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.claims
}

func (b *blockedClaimBD) Run(_ context.Context, command execution.Command, _ execution.OutputObserver) (execution.ProcessResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	args := command.Args
	switch {
	case len(args) >= 3 && args[0] == "update" && args[2] == "--claim":
		b.claims++
		if b.claims <= b.refusals {
			return execution.ProcessResult{
				Status:   execution.ProcessFailed,
				ExitCode: 1,
				Stderr:   fmt.Sprintf("Error claiming %s: issue not claimable: status blocked", b.id),
			}, nil
		}
		return b.answer("in_progress"), nil
	case len(args) >= 3 && args[0] == "update" && args[2] == "--status=open":
		b.cleared = true
		return b.answer("open"), nil
	case len(args) >= 1 && args[0] == "show":
		if b.cleared {
			return b.answer("open"), nil
		}
		return b.answer("blocked"), nil
	case len(args) >= 1 && args[0] == "list":
		return execution.ProcessResult{Status: execution.ProcessSucceeded, Stdout: "[]"}, nil
	}
	return execution.ProcessResult{}, fmt.Errorf("unexpected bd command %v", args)
}

func (b *blockedClaimBD) answer(status string) execution.ProcessResult {
	return execution.ProcessResult{
		Status: execution.ProcessSucceeded,
		Stdout: fmt.Sprintf(`[{"id":%q,"title":%q,"status":%q,"priority":1,"issue_type":"task"}]`, b.id, docketedTitle, status),
	}
}
