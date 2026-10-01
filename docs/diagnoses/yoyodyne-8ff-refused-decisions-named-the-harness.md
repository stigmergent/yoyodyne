# yoyodyne-8ff: why the refused re-arms still read as the harness's move

The fix for re-arms nobody carried out (yoyodyne-edi) merged at 02:25 PDT on
2026-09-29. Nine hours later the program manager read the supervisor's periodic
pass (yoyodyne-ifd.413) and the maintenance-duties item (yoyodyne-ifd.434.10).
Neither showed a refusal. Each held line still said the harness was carrying out
the development manager's re-arm. She reported seeing neither.

This record is built from the product's state directory: the runs under `runs/`,
the triage records under `triage/`, `docket.jsonl` and `docket-closed.jsonl`,
`watch.jsonl`, `sweeps/sweeps.jsonl`, and the conversations.

## The build reached the watch

The watch restarted into build `8498cc81` at 03:07 PDT on 2026-09-29, about
forty minutes after the fix merged. It did so on its own: a newer build had been
deployed over the one it was running, so it drained and restarted into it. Every
build the watch ran after that contains the fix.

## The carry-out did refuse both re-arms

- **The maintenance-duties item (yoyodyne-ifd.434.10), run-6fdcae7a.** Its
  triage record counted 55 refused attempts at 01:15 PDT on 2026-09-29. When
  the development manager replaced the re-arm with a re-run at 18:05 PDT, she
  cited 102 refused attempts. So the refusals carried on after the fix, at the
  paced interval.
- **The supervisor's periodic pass (yoyodyne-ifd.413), run-43a30916.** Its
  triage record keeps one finding per run. Recording the re-run at 12:37 PDT
  replaced that finding, so the record no longer shows the attempts. Replaying
  the fixed carry-out over the real records gives the answer. Those records are
  the run as it stood before its pull request was closed, the docket and closure
  logs, and the 2026-09-28 re-arm. On that replay the re-arm was refused with
  "recorded no promotion", naming the re-run. A decision recorded before the fix
  merged was not skipped.

## Where the refusal went unread

The refusal was written onto each item's triage record. Each item's docket entry
came back and was listed as urgent. Three things still kept it from moving
anybody:

1. **Every line that names a next mover still said the harness.** The rule that
   decides whether a decision is still the harness's to carry out
   (`triage.AwaitingCarryOut`) read only whether a decision stood. It never
   checked whether the harness had already tried it and been refused. So each
   item's held line said "what is outstanding is the harness carrying that
   decision out". The urgent docket entry's own next-mover line said "Next
   mover: the harness". The replay shows both.
2. **The program manager read the tracker item.** A triage refusal is written
   to the triage record and the docket, and never to the item's notes.
3. **The development manager was not reading her docket.** Her standing look
   failed to start eight times between 02:08 and 07:27 PDT, because another
   process held her conversation. At 08:15 PDT the tracker listing timed out.
   From 08:59 PDT she worked through refused re-arms of this kind, a few per
   pass. She replaced the re-arm on the periodic pass at 12:37 PDT, after the
   operator's assistant relayed it to her. She replaced the one on the
   maintenance-duties item at 18:05 PDT.

## The same defect after a repair (2026-09-30)

The periodic pass's repaired run (run-05654a9d, pull request 924) stopped at
14:14 PDT on 2026-09-30. Its change was approved, and the replay onto a moved
main conflicted. Its docket entry recorded the decision as a repair with the
grant still outstanding, because the item had two rounds committed and one
judged. The harness had handed the repair back to the run at 13:34 PDT. It had
then been approved in one round. The rule asked only the item's counters, which
say a grant is outstanding until all of its rounds are judged. The carry-out
itself counts the run's continuations, so it fired nothing. The held line and
the docket named the harness, and nothing was going to act.

## What changed

- A decision the harness tried to carry out and was refused is no longer
  counted as the harness's to carry out. The refusal has to be about that
  decision, made since it, and not waiting on the operator's pause or hold. The
  docket's next-mover line names the development manager. The held line says
  what was refused and what clears it, and names her too.
- A repair counts as carried out once its run records being handed back since
  the decision, the same way the carry-out counts it. A repaired run that stops
  again is then hers to decide about.

The docket and the held line both read these from the same rule, so they cannot
disagree.
