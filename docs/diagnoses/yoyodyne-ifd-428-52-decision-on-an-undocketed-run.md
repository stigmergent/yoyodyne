# yoyodyne-ifd.428.52: the re-run recorded for the environmental-refusal item (yoyodyne-ifd.187) against run-04e578ce was attempted and refused thirty-nine times, and nothing the development manager read said so

On 2026-09-26 at 23:30 UTC the development manager recorded a re-run of
yoyodyne-ifd.187 (the environmental-refusal item) against run-04e578ce, the
item's latest run. On 2026-09-27 and again on 2026-09-28 she reported that it had
not fired in over thirty hours, that developer slots were free, and that no
refusal was recorded on the item. She took it for the shape that had defeated
two decisions on the README rewrite (yoyodyne-ifd.437.13): a decision about a
run whose docket entry an earlier repair decision had closed, which the
carry-out might have no stoppage to start from.

**That was half right.** The carry-out did have no stoppage to start from, but
not because an earlier decision had closed the run's entry: **run-04e578ce was
never on the docket at all.** The carry-out did attempt the decision, at every
paced interval, and its re-run action refused it every time. The refusal was
written onto the item's triage record, but under run-04e578ce, and a docket entry
only shows the finding about its own run. No entry stood for run-04e578ce, so no
entry she was shown carried the refusal. And the refusal itself said nothing
about what would clear it.

## What the records say

All times are UTC and come from the product's state directory: the item's triage
record at `triage/yoyodyne-ifd-187-620975541b80bf2e.json`, the docket in
`docket.jsonl` and `docket-closed.jsonl`, the re-run claims under `reruns/`, and
the run at `runs/run-04e578ce4c1ca86a0be4e6481570b85d.json`.

| When | What |
| --- | --- |
| 2026-08-30 07:08 | `stopped_run:run-af66cb33…` docketed. Later that day `publication:run-af66cb33…#268` is docketed too. These are the only two entries the docket holds for yoyodyne-ifd.187. |
| 2026-09-18 14:01 | A re-run of run-af66cb33's stoppage is claimed and starts run-04e578ce. That spends the stoppage's one re-run. |
| 2026-09-18 14:19 | run-04e578ce ends `cancelled` in `developing` ("summarize developer changes: list worktrees failed with exit code -1"). A cancelled run is not a stoppage the docket records, so it is never docketed. Its worktree and branch are swept on 2026-09-19, leaving `refs/yoyodyne/preserved-work/run-04e578ce…`. |
| 2026-09-19 onward | A second re-run is recorded about run-af66cb33 (on 2026-09-19), and a repair about run-04e578ce. That repair cannot fire because the run's worktree was retired. |
| 2026-09-26 | The re-run about run-af66cb33 is refused: that stoppage was already re-run. The refusal's advice was to record the decision "against the stoppage of the item's latest run instead". |
| 2026-09-26 23:30 | Following that advice, she crosses the re-run cap to 3 and records a re-run about run-04e578ce, the item's latest run, replacing the repair recorded about that run. |
| 2026-09-26 23:30 → 2026-09-28 01:40 | The sweep's second walk (added by the unattempted-decision record, yoyodyne-ifd.428.39) reaches the item's latest decision even though no entry leads to it, and offers it. `Rerunner.Rerun` looks the run up on the docket, finds no stoppage, and refuses: "no stopped run of run-04e578ce… is on the triage docket, so there is no stoppage to run again". `carryOutGate` has no case for that refusal, so it is recorded under "the harness's own records" and says the thing that clears it is "what the refusal itself names". It is paced and attempted again. By 2026-09-28 01:40 the item's record counts **39 attempts**. |
| 2026-09-28 03:25, 06:20 | She records a wait and then an escalation about run-af66cb33's publication entry, each saying that no refusal had been recorded or shown. |

## Why nothing she read carried the refusal

`Docketer.joinDecisions` (`internal/orchestrator/triage.go`) puts a carry-out
finding onto an entry through `docketedCarryOut`, which matches on the entry's
own run. That is deliberate: an item with several stoppages has a decision and
an attempt for each, and a finding shown against the wrong one is about a change
the reader cannot see. It leaves one case uncovered. A decision about a run with
no entry has a finding that no entry matches, so the finding is on the item's
record and on nothing she reads. The sweep the unattempted-decision record (yoyodyne-ifd.428.39) extended reaches such a
decision; the docket that shows her what became of it did not.

The unattempted-decision record (yoyodyne-ifd.428.39) did not name it either, and was
right not to. The decision was attempted, and an attempt that a gate refuses is
recorded as a refusal, not as unattempted. Had the pass held it back instead,
the unattempted record would have been written under the same run and hidden in
the same way.

## Why the closed-entry shape is not the cause

The item names a decision recorded against a run whose entry an earlier repair
decision closed. That shape is carried out. `DocketStore.List` returns a closed
entry with its closure attached. `docketedStoppage` finds it whether or not it is
closed, and the decision the action reads is the run's latest, from the item's
triage record. So a repair that closes an entry, followed by a re-run recorded
about the same run, is offered at the next pull and fired.
`TestARerunRecordedAfterARepairClosedTheEntryIsAttemptedAtTheNextPull` pins this,
and it passes with and without this change.

## What changed

- **A re-run of a run the docket never held is carried out.** What the
  development manager decided about run-04e578ce is plain: start the item again.
  The re-run action now does that for a run the docket holds no stoppage of,
  provided the harness holds the run's record, the run belongs to a work item,
  and it has ended. It claims the re-run under the stopped-run key the docket
  would have given the run, so the one-re-run-per-stoppage rule holds for it as
  for any other. The fresh run starts from the target branch, and every other
  gate a re-run asks still applies: the decision on the item's record, its
  re-run budget, the intake hold, the item being startable, no run of it in
  flight, and a free developer slot. The rule that a docketed stoppage must have
  left a blocker or a change behind is not asked of such a run. That rule is why
  the run was never docketed, so asking it could only refuse what she decided.
- **What is still refused says why and what decision would apply.** A repair of
  such a run is still refused, because a repair re-enters a docketed stoppage's
  worktree. So is a re-run of a run the harness holds no record of. Both actions
  refuse with a typed `NoDocketedStoppageError`. The carry-out writes the refusal
  onto the item saying how the run ended, according to its own record, and names
  the decision the harness would carry out instead. That is the re-run of the
  same run for a repair; otherwise, the same decision against a docketed
  stoppage of the item that can still take it, or a re-run of the item's latest
  recorded run. It names only decisions the development manager records and the
  harness fires, and routes nothing to anybody else.
- **The item's docket entries carry it.** `joinDecisions` also shows, on every
  entry of the item, the finding about the item's latest decision where that
  decision names a run the whole docket holds no entry for. That set of runs is
  read from the full docket, not only the entries being listed. The finding is
  taken only while the decision is the latest, and only where it was written
  since that decision. It is labelled with the run it is about, so it is not read
  as a finding about the entry's own stoppage. The same join shows an
  unattempted record about such a run.
- **The advice that sent her there is corrected.** The refusal of a re-run of an
  already-re-run stoppage still points at the item's latest run, and now says
  that a run the docket never held is re-run like any other.

## What this leaves for the item itself

Once this change is deployed, a re-run recorded about run-04e578ce is carried
out at the next pull that has a developer slot for it, provided it is still the
item's latest decision. It is not the latest now. At 06:20 on 2026-09-28 the
development manager recorded an escalation about run-af66cb33. So what moves the
environmental-refusal item (yoyodyne-ifd.187) is her recording the re-run about
run-04e578ce again. That spends a further re-run of the item against its cap,
and past the cap it needs a crossing. The harness then carries it out without
anybody typing a command.
