# yoyodyne-edi: why the re-arms on the supervisor's periodic pass and the maintenance-duties item were never carried out

On 2026-09-28 the development manager recorded a one-time re-arm of the merge on
two stopped runs. Neither re-arm happened, and nothing she was shown said why.
About twelve hours later, with the watch pulling work the whole time, she
reported that neither item carried a carry-out, a refusal, or an attempt.

The two items had different causes:

- **The supervisor's periodic pass (yoyodyne-ifd.413): the re-arm was never
  attempted.** The watch passed over it at every pull without a word. No record
  anywhere shows an attempt, because none was made.
- **The maintenance-duties item (yoyodyne-ifd.434.10): the re-arm was attempted
  fifty-five times and refused every time.** Each refusal was written onto the
  item's triage record. The docket she reads then dropped all of them.

Neither is the defect behind yoyodyne-ifd.428.52 (a decision about a run the
docket never held, refused out of her sight). That fix does not reach either
of these, so both are fixed here.

All times below are UTC, with Pacific time beside the ones that matter. They
come from the product's state directory: the runs under `runs/`, the item
records under `triage/`, `docket.jsonl`, `docket-closed.jsonl`, `watch.jsonl`,
`sweeps/sweeps.jsonl`, and the development manager's conversation
`chat-3f488b20…`.

## The supervisor's periodic pass (yoyodyne-ifd.413), run-43a30916, pull request 571

| When | What |
| --- | --- |
| 2026-09-20 01:44 | The run's change is approved. Promotion is refused: local main and the forge's main had diverged. The run ends `failed` in `integrating`, with **no promotion recorded** (`integration` is empty) and **no merge method** on its pull request. |
| 2026-09-20 03:49 | Its publication is docketed as `publication:run-43a30916…#571`. |
| 2026-09-20 21:53 | She decides to wait, and that closes the publication entry. |
| 2026-09-28 15:43 (08:43 PDT) | She records a re-arm (turn 227). The item's triage record spends the publication's re-arm and closes both entries. |
| from then on | No pull attempts it. The item's triage record has no carry-out finding for the run, the run's record is unchanged since 2026-09-21, and neither the watch log nor any sweep mentions it. |

**Why nothing attempted it.** The watch looks for re-arms to carry out in
`CarryOut.readRearms` (`internal/orchestrator/carryrearm.go`). That function
skipped every run that `rearmable` rejected, before it even read the decision.
`rearmable` requires the run to have recorded a promotion. This run never
promoted, so the decision was never offered to anything that could refuse it,
and no record was written. The closed docket entry was not the cause: closed
entries are read, and the maintenance-duties item below was offered through one.

**Whether the head could be brought up to date.** A re-arm cannot do that. It
makes the merge request the reviewer's verdict authorized, pinned to the
promoted commit, and this run promoted no commit. Had it been attempted, the
re-arm action would have refused with "run … recorded no promotion, so pull
request 571 carries nothing this harness integrated". The branch
`yoyodyne/yoyodyne-ifd-413/43a30916` still exists at `90002017`. It is 604 commits
behind main and conflicts with it (`git merge-tree` exits 1). The re-arm she
recorded assumed the harness would bring the head up to date and re-check it
before merging. No re-arm path does that. What carries this change forward is
the fallback her own decision named: a fresh run that lifts the branch onto
current main. That is a re-run, and it is hers to record.

## The maintenance-duties item (yoyodyne-ifd.434.10), run-6fdcae7a, pull request 863

| When | What |
| --- | --- |
| 2026-09-28 04:15 | The forge's adoption check is red on main itself, so the sweep withdraws the queued merge and hands it back. The run ends `succeeded` with a publication failure recorded. |
| 2026-09-28 17:44 (10:44 PDT) | She records a re-arm (turn 232). It closes both entries. |
| 2026-09-28 17:44 → 2026-09-29 08:15 | The watch attempts it at every pull its pacing allows. Each time the re-arm action refuses: "the merge of pull request 863 is held by something only a person can satisfy: the pull request conflicts with the base branch (DIRTY)". Each refusal is written onto the item's triage record, which by 08:15 (01:15 PDT) counts **55 attempts**. |
| same span | No development manager sweep mentions the item or pull request 863. Her conversation has no word about it after 18:00. |

**Why she was shown none of it.** When the docket is built, `listableDocket`
(`internal/orchestrator/triage.go`) keeps a settled entry only where the
harness carries out the decision that settled it, and `harnessCarriesOut`
named only a repair and a re-run. Its comment said a re-arm "is a merge request
the operator still repeats by hand". That had stopped being true once the
watch began carrying re-arms out (yoyodyne-ifd.429.31, yoyodyne-ifd.428.46).
So every re-arm-settled entry was dropped before its refusal was joined onto
it, and a refusal the design means to put at the top of her docket never
reached it. The harness did write something, but only where nobody reads.

**Whether the head could be brought up to date.** Not by a re-arm. The branch
`yoyodyne/yoyodyne-ifd-434-10/6fdcae7a` is at `69815201`, 91 commits behind main,
and conflicts with it. Main's red adoption check has since been fixed. The
fallback her decision named was "if the forge drops it again, the second drop is
an escalation naming the check". The forge dropped nothing a second time: the
harness refused, and a conflicting head is not a state a further re-arm clears.
What applies is a re-run, which she records, or the escalation she named.

## What changed

- **A re-arm the harness cannot make is refused, not skipped.** The watch now
  offers a re-arm decision about any unmerged, unqueued publication of a run
  that has ended, not only one whose record describes a merge. So the re-arm
  action refuses it with its own reason. That refusal is typed
  (`UnrearmablePublicationError`), and what the item says will clear it names
  the re-run as the decision that applies. It no longer says a later attempt
  might clear it.
- **Re-arm refusals reach the docket.** `harnessCarriesOut` includes a re-arm.
  An entry her re-arm settled therefore comes back onto the docket, ahead of the
  rest, once the harness has tried to carry it out and been refused, which is
  what already happens with a refused repair or re-run.
- **A re-arm that is made takes its old refusal back off.** A refusal left
  standing over a merge the forge now holds would read on the docket as a
  decision that is not happening. The re-run and repair actions already clear
  theirs.

`TestARearmOfAPublicationItsRunNeverPromotedIsRefusedAloudAtTheNextPull`
replays the supervisor's-pass case: a publication entry an earlier wait closed,
a run with no promotion, and a re-arm recorded against it. It asserts the next
pull refuses the re-arm onto the item, naming the missing promotion and the
re-run, and that the entry is urgent on the docket.
`TestARefusedRearmReachesTheDocketAndLeavesItOnceTheMergeIsMade` replays the
maintenance-duties case. Each test fails without its half of the change.

## What this leaves for the two items

Nothing here moves either item. Once this build is running, the next pull
refuses the supervisor's-pass re-arm onto its record and puts both items'
refusals in front of the development manager. For each, the decision that
applies is the re-run her own recorded fallback describes. That is hers to
record, and it spends the item's re-run budget like any decision. Neither item's
tracker notes carry this account yet. A developer run cannot write to the
tracker, so it is in this document and in the summary of the run that wrote it.
