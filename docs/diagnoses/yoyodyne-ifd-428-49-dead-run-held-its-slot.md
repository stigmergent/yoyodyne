# yoyodyne-ifd.428.49: a run parked on a dependency held developer slot 1 for twenty hours, and the stop written for it was never read

From about 6:05 PM Pacific on 2026-09-26 until the operator's assistant stepped
in by hand the next day, run-3b94404c on the deploy-restart item
(yoyodyne-ifd.428.34) was recorded as running and held developer slot 1. No
process was behind it and it wrote nothing. The development manager decided at
7:59 AM Pacific on 2026-09-27 that the run should stop, and the harness wrote
the stop request. Nothing ever read it.

The settlement for a run whose process vanished (yoyodyne-ifd.428.4) was meant to
end runs like this. It did not, because it only looked for one kind of park, and
this run was parked on a different one.

## What the records say

All times are UTC, from the product's state directory under `runs/`.

| When | What |
| --- | --- |
| 2026-09-27 00:21:56 | run-3b94404c is reserved and claims yoyodyne-ifd.428.34, pulled into developer slot 1. |
| 2026-09-27 01:04:28 | The last event in its log. |
| 2026-09-27 01:05:29 | The record's last write: `status: running`, `phase: checking`, `dependency_pause: {blockers: [yoyodyne-ifd.398]}`. No `provider_stop`. |
| 2026-09-27 14:59:57 | `run-3b94404c….stop.json` written, carrying the development manager's `stop` decision (superseded by yoyodyne-ifd.398). |

Shortly before, the development manager had linked the deploy-restart item (yoyodyne-ifd.428.34) to wait on the draining-watch item (yoyodyne-ifd.398).
The run read that link at the gate boundary before its checks, recorded the
dependency park, noted it on the item ("this item waits on work that is not
finished, so the run is waiting rather than failing"), and returned. Its
process exited, as a dependency park is designed to do.

## Why nothing settled it

**The sweep settled provider stops and nothing else.** `Reconciler.settle` in
`internal/orchestrator/reconcile.go` settled a run past the grace only when
`stoppedProviderIsResumable` was true: a run with a recorded provider stop, in
the developing or reviewing phase. run-3b94404c had no provider stop and was in
the checking phase. It fell through to the `pausedForDependency` reading, which
returned `resumable` with no time limit. Every sweep reported it that way.

**Nothing else continues a dependency park.** The item was claimed, so the
scheduler never chose it again. The claim audit (`internal/readmodel/claims.go`)
treats every recorded park as a wait that is still pending and leaves its claim
alone. The only thing that continues a dependency park is somebody typing
`yoyo run` once the dependency closes. The draining-watch item it waited on (yoyodyne-ifd.398) was itself stopped, so the dependency
was not going to close soon, and when it did, nothing would have noticed. The run
stayed in `Incomplete()`, which is what both the developer slots and the
in-flight guard read.

The same gap covered every park whose process returns and exits: a directive
pause, a tracker pause, an outage wait whose process died, and an operator-pause
park once the pause was lifted. The earlier settlement for a run whose process vanished (yoyodyne-ifd.428.4) closed it for provider stops only.

**Only a live process reads a stop request.** A stop is written beside the run
(`runstate.Store.RecordStop`) and read by `activeRun.stopRequested` at the run's
next provider-call boundary. That is in the run's own process. The sweep never
read stop requests at all. A run with no process reaches no boundary, so the
development manager's decision stood unread.

**Status reported it as running.** `readmodel.readRunning` listed every run in
flight with the phase its record held. It had no way to ask whether a process
was behind a run without taking the run's lease, and a reading must not take
it. So `yoyo status` and the dashboard showed run-3b94404c as `checking` in
slot 1 for the whole twenty hours.

## What changed

- **Every park nothing continues is settled after the grace.** `settle` asks
  `parkNothingServes` before any resumable reading. A run parked on a provider
  stop, a dependency, a directive, a tracker that would not answer, a provider
  nobody could reach, or an operator pause since lifted is settled as a vanished
  process once its record has not moved for thirty minutes and the sweep can
  take its lease. For an outage wait, the thirty minutes run from the probe it
  recorded. The settlement is the one the vanished-process work (yoyodyne-ifd.428.4) built: an environmental stop of cause
  `process-vanished`, the item blocked with the account, the change left where
  it is, and the stoppage docketed. Two parks are left out, because something
  else ends each one: a usage limit or overload, which the sweep continues
  itself once its deadline passes, and an operator pause that still stands.
- **The sweep honours a stop on a run with no process, at once.** Before
  anything else it decides about an in-flight run it holds the lease of, the
  sweep reads the stop request. If there is one, it ends the run as the run
  would have ended itself: `cancelled`, the change kept, the stop recorded on the
  item, and a stop the development manager decided docketed and closed by her
  decision. It does not wait for the grace. The result's action is `cancelled`.
- **A run's lease now says who holds it.** `runstate.Store.takeLease` writes a
  holder stamp (the process id) beside the lease, and releasing the lease removes
  it, as a conversation's hold already does. `Store.Presence` reads it without
  taking anything. A stamp naming a process that has exited means no process,
  immediately. No stamp means no process once neither the record nor its event
  log has been written to for thirty minutes.
- **`yoyo status` and the dashboard name such a run.** The running line's head
  counts runs with no process behind them, and each such run says so in place
  of its phase. The dashboard's card says the same and carries a dashed rule.
  `--json` carries the sentence as `no_process`.

`TestTheSweepHonoursAStopOnAPausedRunWhoseProcessWasKilled` in
`internal/orchestrator` parks a run on a dependency, has a separate process take
its lease, kills that process, writes a decided stop, and checks that the sweep
cancels the run at once, keeps its change, dockets the decided stop, and frees
the slot. `TestTheSweepSettlesADependencyPausedRunNothingContinued` checks that a
dependency park is settled after the grace and not before. (Superseded by
yoyodyne-ifd.428.51: a watching session's pull now continues a dependency park
once the work closes, so the sweep leaves it, and
`TestTheSweepLeavesADependencyPausedRunToThePull` checks that instead.)

## What this does not cover

- The operator's assistant's hand fix and findings were recorded on the work item
  as an intervention. This diagnosis was written from the state directory and
  the code. The item's own notes were not readable from the developer run, so
  anything the intervention found that the records above do not show is not
  reflected here.
- A run from a build older than the holder stamp writes no stamp. If such a run
  is still alive but writes nothing to its record or event log for thirty
  minutes (asleep in-process on a long wait, say), status will name it as having
  no process until it moves again. The sweep is not affected: it decides by
  taking the lease, as before.
- The sweep has to run for any of this to happen. Nothing `yoyo` installs runs
  `yoyo reconcile` on a schedule yet; that is the supervisor's periodic pass,
  yoyodyne-ifd.413.
