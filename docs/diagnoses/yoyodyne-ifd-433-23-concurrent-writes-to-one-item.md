# yoyodyne-ifd.433.23: overlapping writes to one tracker item lose one, silently

The concurrent tracker access run (yoyodyne-ifd.271, run-5680e6a2) stopped on
2026-09-29 at 17:50 PDT with its own new check, `TestConcurrentWriteConformance`,
failing. The check made four overlapping `--append-notes` writes to one item
and found one missing afterwards. The run's program manager judged the check
was at fault. It was not. The check found a real loss, and the loss is `bd`'s.

All times are Pacific (PDT).

## The answer

Against `bd version 1.1.2 (20e493e56)` on macOS, overlapping writes to one item
lose one now and then, and every write involved exits 0. The adapter now queues
its writes to any one item (`Client.write` in `internal/beads/writelock.go`),
and the same check passes with the queue in place and fails without it.

## What was run

Each batch is a fresh scratch store (`git init`, `bd init`) with one item,
and nine `bd update` commands started at once against it: six
`--append-notes` with a distinct line each, one `--priority=1`, and two
`--set-metadata` with different keys. Afterwards the item was read with
`bd show --json`. This was run on 2026-09-30 from 22:44 to 23:00 PDT.

| What | Batches | Writes | Exited non-zero | Missing on read-back |
| --- | --- | --- | --- | --- |
| Note appends to one item | 15 | 90 | 0 | 5, in 3 batches |
| Metadata keys on one item | 15 | 30 | 0 | 1 |
| Priority change on one item | 15 | 15 | 0 | 0 |
| Any loss in a batch | 15 | 135 | 0 | 4 of 15 batches |

In one batch, writer 4's note and the `k1` metadata key went missing together.
In another, writers 2 and 4 both lost their notes.

Writes to **different** items did not lose anything. In 8 batches, six items
each had a note appended and a metadata key set at the same moment. All 48
writes were present afterwards, and none exited non-zero.

## Why it happens

An append in `bd` reads the notes already on the item and writes them back with
the new line on the end. A metadata key is handled the same way: the metadata is
read, the key is added, and the whole thing is written back. If two of these run
at once, both read the same starting notes, and whichever writes second puts
back notes that do not contain the first one's line. Each command's own answer
shows the item as that command wrote it, with its own line present. So the
adapter's read-back in `confirmWritten` passes for both commands, and nothing
anywhere says a line was lost. A priority change sets a single value and reads
nothing first, which is why it was never lost.

## Why yoyodyne-ifd.271 saw nothing on 2026-09-03

That run's exercise made six overlapping appends once and found all six. With
about one batch in four losing something, a single batch coming back clean is
expected. Its run on 2026-09-29 is the one that failed.

## The fix and how it is checked

`Client.write` takes an exclusive advisory lock on
`.beads/yoyodyne-writes/<item>.lock` for as long as `bd` runs, around every
command that changes an existing item: update, claim, close, adding or removing
a dependency, and each record and status write built on them. The lock is
shared by every process using the same store. The operating system drops it if
the process holding it dies, so a crashed writer leaves nothing held. The lock
file is opened through `repowrite.Root.OpenAppend`, so the write is confined to
the repository. `.beads/.gitignore` already ignores `*.lock`.

Writes to different items take different locks, because nothing was lost
between them. That also means a `bd` command that stalls (see below) only
holds up writes to its own item. The wait for a lock is limited to the
client's limit on one command. A write that waits that long fails with an
error naming the item, and is not made without the lock. Creating a new item
takes no lock, because there is nothing for it to overwrite.

Checks:

- `TestConcurrentWriteConformance` runs against a real `bd`. Ten clients write
  to one item at once: eight note appends, a price, and a landing. Then every
  write is read back. Without the lock, 1 of 6 runs failed with an append
  missing. With the lock, 7 of 7 passed. One run catches the loss only some of
  the time, so this test can only catch a regression some of the time.
- `TestWritesToOneItemQueue` needs no `bd` and gives the same answer every
  time. A write to an item whose lock is held waits until it gives up, and a
  write to a different item goes ahead.
- Both real-`bd` results say whether the check ran. A run logs
  `RAN against bd version …`. A skip says `SKIPPED, not passed` and gives the
  reason. With `YOYODYNE_REQUIRE_BD` set, a machine without `bd` fails every
  real-`bd` check instead of skipping it, so a green suite there means the
  checks ran.

## What this does not cover

- **Writes outside the harness.** A `bd update` typed in a terminal, or run by
  a script, does not take the lock and can still overlap a harness write. The
  product manager's conversation writes through the harness client, so it is
  covered.
- **The live losses reported on 2026-09-30.** On the re-arms investigation
  item (yoyodyne-8ff), the program manager reported that a note and a priority
  change were sent together and only the note survived. The reproduction never
  lost a priority change. A note that overwrote a priority is not the
  read-then-write-back pattern above, so that report is not explained here.
  The item's current priority is also the Lead Product Manager's deliberate
  change that evening, which hides whatever was written before it. This run
  did not reconstruct that loss, or the reason missing from a stop around
  21:14 UTC, from the live store.
- **The stall.** yoyodyne-ifd.271 also measured `bd` sometimes stalling one
  command for minutes under concurrent load: 3 of 10 batches of six creates at
  once had one command that took between 170 seconds and about 40 minutes, and
  one of those batches was already running behind an exclusive lock. A
  command that stalls while holding an item's lock makes the other writes to
  that item wait up to the client's limit, and then fail. That is a loud
  failure rather than a silent loss, and it affects one item rather than the
  whole tracker.
