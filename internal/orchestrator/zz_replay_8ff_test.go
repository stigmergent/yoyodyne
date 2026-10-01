package orchestrator

import (
	"context"
	"os"
	"testing"

	"github.com/mason-bryant/yoyodyne/internal/publish"
	"github.com/mason-bryant/yoyodyne/internal/readmodel"
	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// Temporary replay of the real records against the fixed carry-out; removed
// before hand-over.
func TestReplay8ff(t *testing.T) {
	root := os.Getenv("REPLAY_8FF_ROOT")
	if root == "" {
		t.Skip("no replay root")
	}
	runs, err := runstate.NewStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	docket, err := runstate.NewDocketStore(root, "yoyodyne")
	if err != nil {
		t.Fatal(err)
	}
	forge := &forgeStub{observed: publish.PullRequest{Number: 863, State: "OPEN"}, status: "DIRTY", result: publish.MergeResult{Queued: true}}
	watch := CarryOut{
		Docket:    docket,
		Decisions: runs.Triage(),
		Reruns:    runs.Reruns(),
		Runs:      runs,
		Rearmer: Rearmer{
			Docket:    docket,
			Runs:      &leasedRuns{Store: runs},
			Forge:     forge,
			Worktrees: &remoteTargetStub{},
			Decisions: runs.Triage(),
		},
	}
	entries, _ := docket.List()
	t.Logf("docket entries: %d", len(entries))
	carried, err := watch.CarryRearms(context.Background(), false)
	t.Logf("err = %v", err)
	for _, c := range carried {
		t.Logf("carried %s %s carried=%v gate=%s problem=%.300s record=%s", c.WorkItemID, c.RunID, c.Carried, c.Gate, c.Problem, c.RecordProblem)
	}
	for _, item := range []string{"yoyodyne-ifd.413", "yoyodyne-ifd.434.10"} {
		counters, _ := runs.Triage().Counters(item)
		t.Logf("%s carry-outs: %+v", item, counters.CarryOuts)
	}
	build, err := docketerOverStore(docket, runs, rearmConfig()).Build()
	t.Logf("docket build err=%v live=%d", err, len(build.Entries))
	for _, entry := range build.Entries {
		for _, candidate := range append([]triage.Entry{entry}, entry.Earlier...) {
			if candidate.RunID == "run-43a30916a7efed1223d7f91cc5e66fe0" || candidate.RunID == "run-6fdcae7ac8a9970d771d115d026c003f" {
				t.Logf("entry %s critical=%v stopped=%v awaiting=%v\n%s", candidate.Key, candidate.Critical(), candidate.CarryOutStopped(), candidate.Counters.AwaitingCarryOut(), candidate.Render())
			}
		}
	}
	holds, err := readmodel.HeldForAPerson(context.Background(), runs, runs.Triage(), nil)
	t.Logf("holds err=%v", err)
	for _, item := range []string{"yoyodyne-ifd.413", "yoyodyne-ifd.434.10"} {
		reason, held := holds.Reason(item)
		t.Logf("held line %s (%v): %s", item, held, reason)
	}
}
