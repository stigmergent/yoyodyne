package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mason-bryant/yoyodyne/internal/runstate"
	"github.com/mason-bryant/yoyodyne/internal/triage"
)

// A decision is carried out or refused with the reason, whatever became of the
// docket entry for its run. yoyodyne-ifd.428.52 is the case where it was
// neither in any sense the development manager could read: yoyodyne-ifd.187's
// re-run of run-04e578ce, a run the docket never held, was refused onto the
// item's record thirty-nine times with nothing said about what would clear it,
// and no entry she was shown carried the refusal at all.

// undocketedRunID is a run of the docketed item that ended without being
// docketed: a re-run the harness cancelled on its way out.
const undocketedRunID = "run-33333333333333333333333333333333"

// recordUndocketedRun records a run of the item that ended cancelled and was
// never docketed as a stoppage, which is what run-04e578ce was.
func recordUndocketedRun(t *testing.T, harness *rerunHarness) {
	t.Helper()
	cancelled := stoppedState()
	cancelled.RunID = undocketedRunID
	cancelled.Status = runstate.StatusCancelled
	cancelled.Blocker = ""
	cancelled.CheckFailure = nil
	cancelled.ReviewFindingDetails = nil
	cancelled.ReviewFindings = 0
	cancelled.ReviewSummary = ""
	if err := harness.runs.Create(cancelled); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

// joined is the docket as the development manager is shown it: the entries
// with the item's triage record joined onto them.
func joined(t *testing.T, harness *rerunHarness) []triage.Entry {
	t.Helper()
	entries, err := harness.docket.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	docketer := Docketer{Decisions: harness.runs.Triage(), Reruns: harness.reruns}
	if problems := docketer.joinDecisions(entries, nil); len(problems) != 0 {
		t.Fatalf("joinDecisions() problems = %v", problems)
	}
	return entries
}

// The shape the item names: an entry an earlier repair decision closed, and a
// re-run recorded about the same stoppage afterwards. The closure does not keep
// the re-run from the pass; it is attempted at the next pull as any other.
func TestARerunRecordedAfterARepairClosedTheEntryIsAttemptedAtTheNextPull(t *testing.T) {
	t.Parallel()

	harness := newDocketedHarness(t, stoppedState())
	if _, err := harness.runs.Triage().GrantRepair(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRepair, docketedRunID), 1, docketedNow, laterCaps); err != nil {
		t.Fatalf("GrantRepair() error = %v", err)
	}
	harness.docket.close(triage.Key(triage.ClassStoppedRun, docketedRunID), runstate.TriageDecisionRepair, docketedNow)
	if _, err := harness.runs.Triage().RecordRerun(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRerun, docketedRunID), docketedNow.Add(time.Hour), laterCaps); err != nil {
		t.Fatalf("RecordRerun() error = %v", err)
	}

	carrying := harness.carryOutAt(2 * time.Hour)
	task := theOneOutstanding(t, carrying)
	if task.RunID != docketedRunID || task.Decision != runstate.TriageDecisionRerun {
		t.Fatalf("task = %#v, want the re-run recorded over the closed entry offered", task)
	}
	carried, _, err := carrying.Carry(context.Background(), task)
	if err != nil {
		t.Fatalf("Carry() error = %v", err)
	}
	if !carried.Carried || len(harness.started) != 1 {
		t.Fatalf("carried = %#v, started = %#v, want the re-run fired", carried, harness.started)
	}
}

// The 187 case: a re-run recorded against a run the docket never held, where
// the item's one docketed stoppage has already been re-run. It is attempted,
// refused, and the refusal on the item says why and that an escalation is what
// applies — and the docket entry she reads for the item carries it.
func TestARerunOfAnUndocketedRunIsRefusedNamingWhyAndTheDecisionThatApplies(t *testing.T) {
	t.Parallel()

	harness := newRerunHarness(t, stoppedState())
	first := harness.carryOut()
	if _, _, err := first.Carry(context.Background(), theOneOutstanding(t, first)); err != nil {
		t.Fatalf("Carry() of the first decision error = %v", err)
	}
	recordUndocketedRun(t, harness)
	if _, err := harness.runs.Triage().RecordRerun(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRerun, undocketedRunID), docketedNow.Add(time.Hour), laterCaps); err != nil {
		t.Fatalf("RecordRerun() error = %v", err)
	}

	later := harness.carryOutAt(2 * time.Hour)
	task := theOneOutstanding(t, later)
	if task.RunID != undocketedRunID {
		t.Fatalf("task = %#v, want the decision about the undocketed run attempted", task)
	}
	carried, _, err := later.Carry(context.Background(), task)
	if err != nil {
		t.Fatalf("Carry() error = %v, want the refusal reported as a gate", err)
	}
	if carried.Carried || len(harness.started) != 1 {
		t.Fatalf("carried = %#v, started = %#v, want nothing fired", carried, harness.started)
	}
	counters, err := harness.runs.Triage().Counters(docketedItem)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	recorded, found := counters.CarryOutOf(undocketedRunID)
	if !found || recorded.Unattempted || recorded.Attempts != 1 {
		t.Fatalf("finding = %#v (found %t), want the refusal of an attempt on the item", recorded, found)
	}
	for _, want := range []string{"holds no stoppage of it", "ended cancelled"} {
		if !strings.Contains(recorded.Refusal, want) {
			t.Fatalf("refusal = %q, want it to say %q", recorded.Refusal, want)
		}
	}
	for _, want := range []string{docketedRunID, "whose one re-run was claimed", "an escalation", "`yoyo run " + docketedItem + "`"} {
		if !strings.Contains(recorded.Clears, want) {
			t.Fatalf("clears = %q, want it to name %q", recorded.Clears, want)
		}
	}
	if strings.Contains(recorded.Clears, "what the refusal itself names") {
		t.Fatalf("clears = %q, want the decision that applies rather than a pointer back at the refusal", recorded.Clears)
	}

	entries := joined(t, harness)
	if len(entries) != 1 {
		t.Fatalf("docket = %#v, want the item's one stoppage", entries)
	}
	shown := entries[0].CarryOut
	if shown == nil || shown.RunID != undocketedRunID || shown.Refusal != recorded.Refusal {
		t.Fatalf("carry-out = %#v, want the item's entry to carry the refusal about the undocketed run", shown)
	}
	rendered := entries[0].Render()
	for _, want := range []string{"is about run " + undocketedRunID + ", which this docket holds no entry for", "an escalation"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered = %q, want it to say %q", rendered, want)
		}
	}
}

// Where the item has a docketed stoppage that can still take the re-run, the
// refusal names that stoppage as where the decision belongs.
func TestARerunOfAnUndocketedRunNamesTheDocketedStoppageThatCouldTakeIt(t *testing.T) {
	t.Parallel()

	harness := newDocketedHarness(t, stoppedState())
	recordUndocketedRun(t, harness)
	if _, err := harness.runs.Triage().RecordRerun(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRerun, undocketedRunID), docketedNow, rerunCaps); err != nil {
		t.Fatalf("RecordRerun() error = %v", err)
	}
	carrying := harness.carryOut()
	carried, _, err := carrying.Carry(context.Background(), theOneOutstanding(t, carrying))
	if err != nil {
		t.Fatalf("Carry() error = %v", err)
	}
	if carried.Carried {
		t.Fatalf("carried = %#v, want the decision refused", carried)
	}
	counters, err := harness.runs.Triage().Counters(docketedItem)
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	recorded, _ := counters.CarryOutOf(undocketedRunID)
	for _, want := range []string{"recorded instead against a stoppage the docket holds", docketedRunID} {
		if !strings.Contains(recorded.Clears, want) {
			t.Fatalf("clears = %q, want it to name %q", recorded.Clears, want)
		}
	}
	if strings.Contains(recorded.Clears, "escalation") {
		t.Fatalf("clears = %q, want the open stoppage named rather than an escalation", recorded.Clears)
	}
}

// The unattempted record covers the same shape: a decision about an undocketed
// run that no pass attempts is written onto the item, and the item's docket
// entry carries it, naming the run it is about.
func TestAnUnattemptedDecisionAboutAnUndocketedRunIsShownOnTheItemsEntry(t *testing.T) {
	t.Parallel()

	harness := newDocketedHarness(t, stoppedState())
	recordUndocketedRun(t, harness)
	if _, err := harness.runs.Triage().RecordRerun(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRerun, undocketedRunID), docketedNow, rerunCaps); err != nil {
		t.Fatalf("RecordRerun() error = %v", err)
	}
	going := stoppedState()
	going.RunID = "run-11111111111111111111111111111111"
	going.Status = runstate.StatusRunning
	going.Phase = runstate.PhaseDeveloping
	going.CompletedAt = nil
	going.Blocker = ""
	going.CheckFailure = nil
	going.ReviewFindingDetails = nil
	going.ReviewFindings = 0
	going.ReviewSummary = ""
	if err := harness.runs.Create(going); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	written, err := harness.carryOutAt(2*time.Minute).RecordUnattempted(context.Background(), time.Minute, nil)
	if err != nil {
		t.Fatalf("RecordUnattempted() error = %v", err)
	}
	if len(written) != 1 || written[0].RunID != undocketedRunID {
		t.Fatalf("written = %#v, want the decision about the undocketed run written down as unattempted", written)
	}
	entries := joined(t, harness)
	if len(entries) != 1 || entries[0].CarryOut == nil || !entries[0].CarryOut.Unattempted || entries[0].CarryOut.RunID != undocketedRunID {
		t.Fatalf("docket = %#v, want the item's entry to carry the unattempted decision", entries)
	}
	if rendered := entries[0].Render(); !strings.Contains(rendered, "No pass has attempted") || !strings.Contains(rendered, undocketedRunID) {
		t.Fatalf("rendered = %q, want the entry to say the decision about the undocketed run was not attempted", rendered)
	}
}

// A finding about a decision she has since decided past is not shown on the
// item's entry: only the item's latest decision is.
func TestAFindingAboutAnUndocketedRunIsNotShownOnceSheDecidesPastIt(t *testing.T) {
	t.Parallel()

	harness := newDocketedHarness(t, stoppedState())
	recordUndocketedRun(t, harness)
	if _, err := harness.runs.Triage().RecordRerun(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionRerun, undocketedRunID), docketedNow, rerunCaps); err != nil {
		t.Fatalf("RecordRerun() error = %v", err)
	}
	carrying := harness.carryOut()
	if _, _, err := carrying.Carry(context.Background(), theOneOutstanding(t, carrying)); err != nil {
		t.Fatalf("Carry() error = %v", err)
	}
	if _, err := harness.runs.Triage().RecordDecision(context.Background(), docketedItem,
		triageDecided(runstate.TriageDecisionEscalate, docketedRunID), docketedNow.Add(time.Hour)); err != nil {
		t.Fatalf("RecordDecision() error = %v", err)
	}
	if entries := joined(t, harness); len(entries) != 1 || entries[0].CarryOut != nil {
		t.Fatalf("docket = %#v, want no finding about the decision she moved past", entries)
	}
}
