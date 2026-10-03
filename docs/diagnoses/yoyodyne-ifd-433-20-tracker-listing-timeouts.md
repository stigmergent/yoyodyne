# yoyodyne-ifd.433.20: what timed the tracker's listings out, and what the twelve-hour stall actually was

Two things were reported as one. The development manager reported on
2026-09-29 that `bd list` was timing out, so her docket could not drop entries
on closed items and she could not check any item before deciding. The next
morning the Lead Product Manager widened the item after the factory went quiet
from 21:51 PDT on 2026-09-29 to 09:48 PDT on 2026-09-30, on the reading that
every scheduled pass had failed on `read the closed work items: bd list failed
with status timed_out`.

The records say those are two different causes. The twelve-hour stall was the
machine asleep. The listing timeouts are real, happen while the machine is
awake, and come from how bd serializes access to its store under load.

All times below are Pacific (PDT) unless marked Z, which is UTC.

## The twelve-hour stall was the laptop asleep

The macOS power log (`pmset -g log`) on the operator's machine:

| When (PDT) | What |
| --- | --- |
| 2026-09-29 21:53:26 | `Entering Sleep state due to 'Clamshell Sleep'`, on battery at 29% |
| 21:53 to 08:51 | Only `Maintenance Sleep` and `DarkWake` entries: about forty wakes of roughly 45 seconds each, battery falling to 18% |
| 2026-09-30 08:51:44 | `Wake from Deep Idle … due to … lid … HID Activity`, and on AC power two seconds later |

The lid was closed at 21:53 and opened at 08:51. Between those, the harness
ran only in the maintenance wakes.

The sweep log (`sweeps/sweeps.jsonl` under the product's state) lines up with
that exactly:

| Pass | Started | Ended | What its record says |
| --- | --- | --- | --- |
| development manager | 04:52:08Z (21:52 PDT) | 08:22:47Z (01:22 PDT) | turn 1 failed: `API Error: Your computer went to sleep mid-response` |
| development manager, product manager, factory-flow program manager | due 04:59Z–05:37Z | recorded missed at 08:44:45Z (01:44:45 PDT) | missed: the watch was inside the pass above |
| architect | 09:47:01Z (02:47 PDT) | 14:07:56Z (07:07 PDT) | completed, two turns, four hours twenty minutes of wall time |
| development manager | 15:18:54Z (08:18 PDT) | 15:53:31Z | completed |

The development manager's pass started one minute before the lid closed and its
provider reported, in its own words, that the computer went to sleep. The
missed-pass records were written at 01:44:45 PDT, the second a maintenance wake
began. The watch log (`watch.jsonl`) has one line at 04:51:52Z, when the
session restarted into a deployed build, and nothing after it until 16:48:08Z,
when it filled a developer slot: a watch session fires recurring passes inside
its poll, one at a time, so a pass that spans a sleep holds the poll for as long
as the sleep lasts.

`bd list` timing out appears in exactly one pass record in that window, and it
is not a pass failing. The development manager's 21:52 pass carries it as a
second problem after its failure: the forge reading on her pass, run once the
provider gave up at 01:22 PDT in a 45-second maintenance wake, could not list
the closed items. The three missed-pass records then carry that pass's whole
problem as their cause — the watch was in that pass, so it was what kept them —
and that is where "every pass failed on `read the closed work items`" was read
from. No pass in the window failed on a listing.

So the stall was not the tracker. Nothing the harness does replaces a machine
kept awake, and [the operations guide](../operations.md#a-tracker-that-does-not-answer-a-listing)
now says so. What the harness could do better — a recurring pass should not
hold the watch's poll for hours — is named at the end as work to admit rather
than done here.

## The listing timeouts are contention on bd's store, worst under check load

Separately from the sleep, `bd` calls do time out while the machine is awake.
The conversations' event logs record 117 tracker calls that timed out between
2026-09-02 and 2026-09-30, `show`, `update`, `create`, `list`, and `dep` alike,
bunched into busy hours (four listings in one hour on 2026-09-29 at 15Z, five
in one on 2026-09-30 at 17Z). The one the development manager's pass met at
11:13 PDT on 2026-09-30 was on an awake machine on power: two runs were in
their checks at that moment — the ownership registry item's run
(yoyodyne-ifd.432.25.1), which its check stage's bound later stopped, and the
priority-0 selection item's run (yoyodyne-ifd.428.58), landing — so the race
suites were running.

What the store looks like, read without running bd (every bd invocation opens
the store read-write, so none was run against the live tracker):

- The Dolt store under `.beads/embeddeddolt` is 420 MB for 895 items, and every
  bd process takes the exclusive lock `embeddeddolt/.lock` and
  `.dolt/noms/LOCK` to open it — reads included. One bd runs at a time; the
  rest wait.
- Every write re-exports the whole tracker to `.beads/issues.jsonl`, which is
  10.7 MB, while holding that lock.
- 111 abandoned export temporaries (`.beads/.~issues.jsonl.<n>`) sit beside it,
  20 of them partly written, from 2026-09-14 onward. Each is a bd process killed
  mid-export. The half-written ones cluster on the same days as the recorded
  timeouts: 9.9 MB at 23:07 PDT on 2026-09-28, 6.9 MB at 08:13 PDT on
  2026-09-29 (the conversations recorded ten timed-out calls in the hour that
  followed). The harness's own thirty-second bound is what kills them, so a
  killed write also leaves the lock held for the next one to wait out.

Measured against bd itself, on a fresh store with three items: the test this
change adds (`TestAListingUnderACompetingWriterCompletesOrNamesTheFailure` in
`internal/beads`) lists the store in a loop for 45 seconds while a second
process creates items. It made 31 listings beside 8 writes — a write took about
seven seconds on a three-item store because it queued behind the listings —
and every listing answered. The same serialization on a 420 MB store with a
10.7 MB export, beside two race suites, is what puts a listing past thirty
seconds.

The re-run of the concurrent tracker access item (yoyodyne-ifd.271) measured bd
1.1.2 stalling one call for 170 seconds to 40 minutes in 3 of 9 six-way
batches, once with only one bd running; that is a stall inside bd rather than
the queue, and it is the case a retry cannot help and the status line now names.

So of the three causes the item listed — machine load, contention on the Dolt
store, the listing's size — it is the second, made worse by the first and by
the export's size, which grows with every item; and the stall was none of them.

## What changed

- A listing its bound killed is asked again, twice, after two and then eight
  seconds (`internal/beads`, `listingWaits`), and one that still fails says how
  many attempts it made and over how long.
- Every listing the harness's tracker client makes writes how it ended to
  `tracker-listings.json` under the product's state, and `yoyo status` carries
  a tracker that is not answering listings under `Waiting on the harness`, with
  since when, as an entry of kind `tracker-unanswered`.
- The forge reading on the development manager's pass compares every request
  with the forge even when the closed items could not be listed, and names what
  it could not judge. Before, a failed listing returned before any request was
  compared.
- Her docket, when it cannot read whether each entry's item is still open, says
  that is the harness's to retry and that she decides the entries as they
  stand.
- The watching session writes a line to `watch.jsonl` as each recurring pass
  it fires begins — which pass, what fired it, and since when — so a pass that
  holds the poll is said as that pass rather than left as silence. It is a note,
  as a dispatch's wait is, so the session's own state as every surface reads it
  is unchanged. With a listing that times out on every attempt, the session
  still fires its passes at every poll (they come before the queue is read),
  and says at each poll that the store is being read again and what the
  listing said.

The watch is made to say what it is doing rather than made to keep pulling
through a pass, deliberately. A wall-time bound on a pass does nothing for the
case that happened: a machine asleep runs nothing, the bound included, and on
waking the provider had already ended the turn itself. What would keep the watch
pulling is running a pass beside the poll rather than inside it, which changes
how the session hosts its one conversation turn per poll against its runs and
its drain, and is left below.

## What is left

- **A recurring pass still holds the watch's poll for as long as it runs.** The
  log now says so, but nothing is pulled meanwhile. Running a pass beside the
  poll rather than inside it is a design change to the watch and is not made
  here.
- **The missed-pass record names the previous pass's whole problem as its
  cause.** Read alone, it says the missed task failed on that problem, which is
  how the stall came to be read as the tracker.
- **The export grows with the tracker and is rewritten on every write.** At
  10.7 MB it is a large part of how long bd holds its lock; bd's export
  settings, or keeping the export off the write path, are the lever.

  Resolved by tracker export cleanup and contention reduction
  (yoyodyne-ifd.433.24): the harness sets `BD_EXPORT_AUTO=false` on its bd
  calls and requests explicit snapshots at reader boundaries. Calls outside
  the harness retain their own export settings; see
  [tracker export snapshots and abandoned temporary files](../operations.md#tracker-export-snapshots-and-abandoned-temporary-files).
- **Abandoned export temporaries accumulate** in the primary checkout's
  `.beads` and nothing removes them.

  Resolved by tracker export cleanup and contention reduction
  (yoyodyne-ifd.433.24): `yoyo reconcile`, including the supervisor's
  maintenance pass, removes export temporaries more than 24 hours old only
  after checking that no process holds them open, and records each removal.
  If that check cannot give a complete answer, it removes nothing; see
  [tracker export snapshots and abandoned temporary files](../operations.md#tracker-export-snapshots-and-abandoned-temporary-files).
