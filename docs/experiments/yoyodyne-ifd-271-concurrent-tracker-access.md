# What concurrent runs do to the tracker, exercised against a real store

Work item: yoyodyne-ifd.271, from the developer report on run-3e6cc3ad.

**Status: nothing was refused and nothing was lost, and there is one open
finding.** Concurrent `bd` invocations against one embedded Dolt store were
exercised live, first on 2026-09-03 and again on 2026-09-29. No invocation
failed and no write was lost. So the question the item asked, whether
invocations contend, has the answer no, and
`internal/beads/client.go` was left as it is, with no lock and no retry. The
second exercise also found that, now and then, one `bd` invocation stalls for
minutes. That happens under concurrent load, including when an exclusive lock
lets only one invocation run at a time. It is recorded below as its own
finding, not as contention.

## The question

Raising `execution.max_concurrent_developers` above one makes several runs invoke
`bd` at the same time — `Show`, `Claim`, `RecordOutcome`, `RecordCost`,
`Complete` — beside the scheduler's `List` and `Ready`. The tracker is an
embedded Dolt database behind a file lock, and the adapter neither serializes its
invocations nor retries a contended one: a contended invocation has nowhere to go
but back to its caller as a failed run, on a boundary where the work is already
done. Two failure modes were open, and one of them is silent:

- **Loud.** A second opener is refused, and a run that finished its work is
  recorded as failed at the write that would have recorded it.
- **Silent.** Two overlapping writes to one item are read-modify-write inside bd
  — a note appended to the notes already there, a metadata key set beside the
  keys already there — so one of them is lost with no error anywhere. A lost
  `--append-notes` takes a goal attribution with it, which is the loss
  [the goal witness](../work.md) exists to make recoverable.

Nothing settled either way, because every concurrency check in the package drove
an in-process fake: all of them pass identically against a bd that refuses a
second opener.

## What was exercised

Against `bd version 1.1.2`, on macOS, over a scratch store in a temporary
directory. The two dates are two machines' worth of load rather than one
reading, and the figures are wall clock.

On 2026-09-03:

| Exercise | Concurrency | Result |
|---|---|---|
| `bd create` alongside `bd list` | 8 writers, 6 readers | 14/14 succeeded; 8 distinct items, none lost |
| `bd update --set-metadata=kN=vN` on **one** item | 6 writers | 6/6 keys present afterwards |
| `bd update --append-notes` on **one** item | 6 writers | 6/6 lines present afterwards |
| The capacity-2 run shape through `beads.Client` | 2 runs + scheduler reads | every invocation succeeded; every item closed with its notes and its price |

On 2026-09-29, one store, 99 creates in all, every one of which succeeded and
every one of which was present afterwards:

| Exercise | Result |
|---|---|
| One invocation against an idle store | 0.6s to 1.75s; `list` 0.7s to 1.45s |
| 6 creates one after another | 10.2s, about 1.7s each |
| 60 creates one after another | about 55s in all, none over 2s |
| 6 creates at once, 4 batches | 3 batches finished in 9.7s to 10.5s; in the fourth one create took 170s |
| 6 creates at once, once more | 1,094s for the batch |
| 6 creates at once, each behind an exclusive file lock, 5 batches | 4 batches finished in 5.5s to 8.3s; in the other one create took about 2,420s, and the two queued behind it waited with it |

**How the timing rows fit together.** The row in the first version of this
document read "6 creates sequential, then 6 concurrent: 3.05s then 3.23s", beside
a stated cost of about 0.95s per invocation. Those two figures do not agree:
six sequential creates at 0.95s is 5.7s, not 3.05s. The first run's working notes
are gone, so which of the two figures was wrong cannot now be decided, and the
row has been replaced by the ones measured again above. What they show is the
same shape. A batch that does not stall costs about what the same creates cost
one after another, about 1.5s to 1.7s each, and the creates finish one at a time
about that far apart. So bd takes concurrent invocations one at a time, and a
caller waits its turn rather than being refused.

## The stall

Three of the nine concurrent batches had an invocation that took between
170 seconds and 40 minutes. Each of them still succeeded, and nothing it wrote
was lost. What is known about it:

- **It is not two invocations colliding.** In the stalled batch without a lock,
  the other five creates had finished within 4.5s, so the slow one spent more
  than two and a half minutes with nothing running beside it. In the batch
  behind an exclusive lock, only one `bd` process ran at a time, and one of them
  took about 40 minutes by itself. So a lock in the adapter would not remove
  the stall. It would only queue every other invocation behind it.
- **It was not seen without concurrency.** None of 60 creates made one after
  another stalled. Nine batches and sixty creates are too few to say it never
  happens there, only that it is much rarer.
- **Its cause is not known.** The store's directory holds nothing but bd's own
  lock file, so whatever bd waits on is not visible from outside it.
  Finding the cause is work for bd's internals, and nothing in this change
  touches them.

**What it would cost a run.** The adapter gives each invocation 30 seconds
(`defaultTimeout` in `internal/beads/client.go`), and the client a run uses sets
no other bound. A stall like these would therefore be ended at 30 seconds and
reported to the run as a tracker that did not answer. The harness already
counts that as a stop caused by something outside the work rather than a
verdict on the change, and a watching session retries a tracker read that fails
before it gives up. What nothing does is retry a write. Retrying a write after
it was ended is not safe in general either: an append ended after it landed
would be written twice. That is the case a follow-up has to decide.

## What was not exercised

`yoyo work` was not run at capacity two. It would have spent provider
invocations, cut worktrees, and written to this project's own tracker, none of
which is the question: what capacity two does to the tracker is the invocation
pattern it produces, and that is what ran. The substitution is worth knowing
about, because it means nothing here says anything about two concurrent runs
contending for anything other than the tracker — the check suite and the machine
are covered by [how long a check may take](../configuration.md#how-long-a-check-may-take)
instead, which is a measured cost rather than an open question. Capacity has
since been raised past one in this project's own configuration without waiting
for this exercise, so what it measured is also what that configuration now
depends on.

Neither does anything here say bd will go on serializing. It is somebody else's
software, and the observation is about the version above.

## What keeps it true

`TestConcurrentRunConformance` and `TestConcurrentWriteConformance` in
`internal/beads/conformance_test.go`, beside the other checks that pin real-bd
behavior. The first runs the capacity-2 invocation pattern against a real store —
two runs making the whole sequence a run makes, while the scheduler reads beside
them — and fails naming what a contended invocation would cost. The second makes
four overlapping appends to one item and fails if any of them is missing, which
is the silent case caught loudly.

They run in `make test` like everything else, and skip where bd is not
installed, so a green suite alone does not say they ran. On 2026-09-29 they were
run by name against bd 1.1.2 and both ran rather than skipped:

```
=== RUN   TestConcurrentRunConformance
=== RUN   TestConcurrentWriteConformance
--- PASS: TestConcurrentWriteConformance (71.97s)
--- PASS: TestConcurrentRunConformance (89.64s)
ok  	github.com/mason-bryant/yoyodyne/internal/beads	92.999s
```

They give each invocation the package's ten-minute conformance bound rather than
the adapter's 30 seconds, because they run beside the whole suite on a loaded
machine. So they fail if bd refuses or loses a concurrent write, and not on the
stall above.
