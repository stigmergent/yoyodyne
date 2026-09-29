# yoyodyne-ifd.428.52: the re-run recorded for 187 against run-04e578ce was attempted and refused thirty-nine times, and nothing the development manager read said so

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
| 2026-09-26 23:30 → 2026-09-28 01:40 | The sweep's second walk (added by yoyodyne-ifd.428.39) reaches the item's latest decision even though no entry leads to it, and offers it. `Rerunner.Rerun` looks the run up on the docket, finds no stoppage, and refuses: "no stopped run of run-04e578ce… is on the triage docket, so there is no stoppage to run again". `carryOutGate` has no case for that refusal, so it is recorded under "the harness's own records" and says the thing that clears it is "what the refusal itself names". It is paced and attempted again. By 2026-09-28 01:40 the item's record counts **39 attempts**. |
| 2026-09-28 03:25, 06:20 | She records a wait and then an escalation about run-af66cb33's publication entry, each saying that no refusal had been recorded or shown. |

## Why nothing she read carried the refusal

`Docketer.joinDecisions` (`internal/orchestrator/triage.go`) puts a carry-out
finding onto an entry through `docketedCarryOut`, which matches on the entry's
own run. That is deliberate: an item with several stoppages has a decision and
an attempt for each, and a finding shown against the wrong one is about a change
the reader cannot see. It leaves one case uncovered. A decision about a run with
no entry has a finding that no entry matches, so the finding is on the item's
record and on nothing she reads. The sweep that 428.39 extended reaches such a
decision; the docket that shows her what became of it did not.

The unattempted-decision record from 428.39 did not name it either, and was
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

- **The refusal names why and what decision would apply.** The re-run and repair
  actions now refuse a run the docket holds no stoppage of with a typed
  `NoDocketedStoppageError`. The carry-out writes that refusal onto the item
  saying how the run ended, according to its own record ("ended cancelled at …"),
  and that recording a decision about a run does not docket it. It also names
  the decision that would apply:
  - the same decision recorded against a docketed stoppage of the item that can
    still take it, with that stoppage named; or,
  - where every such stoppage's re-run is already claimed (as run-af66cb33's
    was), an escalation. A fresh run of the item is then the operator's to name
    with `yoyo run <item>`.
- **The item's docket entries carry it.** `joinDecisions` also shows, on every
  entry of the item, the finding about the item's latest decision where that
  decision names a run the docket holds no entry for. It is taken only while the
  decision is the latest, and only where the finding was written since that
  decision. The finding is labelled with the run it is about, so it is not read
  as a finding about the entry's own stoppage. The same join shows an
  unattempted record about such a run.
- **The advice that sent her there is corrected.** The refusal of a re-run about
  an already-re-run stoppage now says to record the decision against the item's
  latest run the docket holds a stoppage of. It also says that a run it holds
  none of has nothing for a re-run to start from.

## What this leaves for the item itself

yoyodyne-ifd.187 is not moved by this change. Its only docketed stoppage has
spent its re-run, so the decision that applies is the escalation. The
development manager recorded that escalation on 2026-09-28 at 06:20, and it now
sits with the operator. Once this change is deployed, the refusal on the item
names that escalation.
