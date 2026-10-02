# The bootstrap repair did not re-enter its run

The repository-preservation work (yoyodyne-ifd.428.47) also asks whether the
decided repair of the thin bootstrap script (yoyodyne-ifd.125.2) failed for the
same reason as the ownership registry's first build (yoyodyne-ifd.432.25.1).
The bootstrap case has additional causes: its notes announce re-entry before
the run is made live, its checkout was subsequently swept, and maintenance of
an older run makes the hold name that older run. This change makes the
preservation refusal truthful, but does not execute or enable that bootstrap
repair while its checkout is absent.

## Records read

This account uses read-only observations made on 2026-10-01 at about 23:08 PDT.
All times below are local, in PDT. The product's durable records are under
`/Users/mbryant/Library/Application Support/Yoyodyne/state/products/yoyodyne/`:

- `runs/run-6a6a2467fd73a2ae9eee416d9566c78c.json` and its `.events.jsonl`;
- `runs/run-7277daa143ec57b90a4726b8beb98e58.json`;
- `triage/yoyodyne-ifd-125-2-dac5dd14cc4e9da1.json`;
- `docket.jsonl` and `docket-closed.jsonl`.

The run's copied `.beads/issues.jsonl` supplies the item's notes. Those notes
are supporting evidence, not proof that a provider invocation happened.
Local branch refs were read with `git show-ref --verify`, and each recorded
checkout path was checked for an existing directory. No run, decision, claim,
branch, or checkout was changed by this investigation.

## The decision and the run are different records

| Time (PDT) | Recorded fact |
| --- | --- |
| 2026-09-18 15:59 | The older run, `run-7277daa1…`, ended failed at integration after a tracker read timed out. Its change is commit `6967b826477d1c1d6f3c94941cd6fb3662acd000`, on pull request #522. |
| 2026-09-27 21:22 | The development manager decided a re-run of that stoppage. The docket closure records that re-run. |
| 2026-09-27 22:14–23:06 | The new run, `run-6a6a2467…`, started, passed checks and review, and ended succeeded while its protected-target publication waited on pull request #866. Its commit is `4f5ff06a823ebc6e405324d73014c83d4766341c`. |
| 2026-09-27 23:08 | The forge's build check failed and the merge was withdrawn. The run acquired a durable publication blocker. |
| 2026-09-28 04:41 | The two docket entries for the new run were closed with the earlier re-arm decision. They remain in the docket store; a closure does not erase the entry a repair looks up. |
| 2026-09-28 17:48 | The development manager replaced that run's re-arm with a bounded repair, in conversation `chat-3f488b20b95402f3f61e8e27fedb6608`, turn 258. The reason identifies the install test's failure on a writable default installation directory. The decision names the new run, not the old one. |
| 2026-09-29 15:55 | The new run's checkout was swept; its record sets `worktree_removed` and `worktree_swept_at`. |
| 2026-09-29 18:31 | The older run's record was updated after the new run's last update. Its pull request is recorded as superseded by the new run's commit. |
| 2026-10-01 21:34 | The item's current carry-out record counts 134 refusals of the new run's repair. The latest says its preserved change was retired and is gone. |

The new run still has its original completion time, status `succeeded`, phase
`cleaning_up`, and no `repair_continuations`. Its event stream ends at sequence
545, the review completion at 23:05 PDT on September 27. It contains no event
from the decided repair. A continuation would keep the original run identifier
and start time, so the absence of a newer start time alone would prove nothing;
the absent continuation and later provider events are the relevant evidence.

The copied export contains fourteen notes saying the harness “re-entered” this
run, more than the three the work item's report counted. One is immediately
followed by the tracker's refusal: `Error claiming yoyodyne-ifd.125.2: issue
not claimable: status blocked`. In
`internal/orchestrator/repaircontinue.go`, `supersedeOnItem` writes the re-entry
account before claiming the item, and `supersedeOnRun` saves the continuation
only after that claim succeeds. Thus a refused claim can leave this account
without ever making the run live. The retained records do not preserve each
earlier attempt's failure, so they cannot identify the exact failing step of
all fourteen attempts. They do establish that the notes do not evidence a
carried-out repair; the current run and its stream record none.

## Why the hold names the older run

The hold reads all qualifying stoppages, not just the run named by the latest
decision. `heldStopping` in `internal/readmodel/holds.go` selects through
`latestPerItem`, which compares `UpdatedAt`. Both runs qualify: the older run
died in its own process, and the newer one carries its publication blocker.
Both branches remain. The older record's September 29 update at 18:31 PDT is
later than the newer record's 15:55 PDT update. It therefore replaces the
newer publication's hold with an account of the older preserved stoppage.
Maintenance has made an older run look latest; the repair has not been
retargeted to that run. The decision and carry-out refusal still name
`run-6a6a2467…`.

The repository observations agree with neither an empty change nor a usable
repair checkout. Branch `yoyodyne/yoyodyne-ifd-125-2/6a6a2467` still points at
`4f5ff06a823ebc6e405324d73014c83d4766341c`, but checkout
`/Users/mbryant/Library/Application Support/Yoyodyne/state/worktrees/yoyodyne/yoyodyne/yoyodyne-ifd-125-2-6a6a2467`
is absent. The older branch `yoyodyne/yoyodyne-ifd-125-2/7277daa1` also exists,
at `6967b826477d1c1d6f3c94941cd6fb3662acd000`, with its checkout absent.

## What this change resolves and what remains

The shared repository lookup replaces the false claim that sweeping the
bootstrap checkout destroyed its whole change. A repair still requires a
verified branch and checkout, and now names both and reports the missing
checkout. It cannot continue this live case as observed. Recovery must restore
the recorded checkout through the harness's ownership checks, or the
development manager must decide a different route for the preserved branch.
This developer run performs neither action.

The ownership registry's recorded refusal is the other shape: the retained
carry-out says its run left no blocker and no change, despite its branch
surviving a check-stage timeout. Its run records the bound stopping `make race`,
not a failed check. This change admits that stopped check stage on repository
evidence and continues checks rather than inventing repair input. The added
end-to-end test also covers zero prior repair attempts, which requires pipeline
adoption to recognize the decided check-stage continuation itself. These are
code changes and test coverage, not a claim that either live repair executed.

Separate work for the Lead Product Manager to consider admitting is to record
successful re-entry only after the durable transition, retain the failures of
earlier attempts, preserve or restore checkouts required by outstanding repair
decisions, and select the hold's run by run chronology or the standing decision
rather than a maintenance update time. Those changes are outside this item's
shared preservation reading.
