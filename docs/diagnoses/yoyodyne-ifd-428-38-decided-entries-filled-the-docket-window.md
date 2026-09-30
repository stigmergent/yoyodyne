# yoyodyne-ifd.428.38: decided entries filled the docket window

The finding, as raised: on 2026-09-26 the development manager's docket page
held four entries. All four had decisions already recorded and were waiting on a
gate. At the same time 29 stoppages were waiting for her decision, among them
yoyodyne-ifd.430.13.4, .383, and .362, and none of them reached any page she was
shown. She decided 430.13.4 only after the operator's assistant gave her its run
id by hand. The re-runs recorded for yoyodyne-ifd.437.1 and .429.1 had each been
refused over 140 times, waiting on .383 and .362, which were the same stoppages
nobody had decided. The window built by yoyodyne-ifd.428.29 was meant to list
live entries oldest first, critical ones ahead, bounded at 25. The item asked
which part was wrong: the live filter, the walk position, or the ordering.

## The cause: the ordering and the byte bound, together

The live filter was right. The fault was two things in
`internal/triage/window.go` and `renderTriageDocket` in
`internal/contextbundle/product.go`, and each needed the other.

**The ordering put decided entries first.** An entry counted as critical if a
decision of hers had been stopped by a gate that will not clear on its own
(`Entry.Critical`: the carry-out was stopped and not marked as waiting). A
re-run refused because its item waits on unfinished work is exactly that. So
the re-runs of 437.1 and 429.1 were critical, and critical entries went into
`Window.Urgent` and were listed before the walk. The walk is where the undecided
stoppages were. Nothing in the order told apart an entry that needs a decision
from one whose decision is already made and waiting on the harness.

**The byte bound, not the count bound, decided how many entries were listed.**
The window is held to 25 entries and to 48 KiB (`MaxTriageDocketBytes`). An
entry is rendered with all its evidence: the blocker, the findings, the failing
check's output, the carry-out finding, the decisions recorded, and every earlier
docketing of the same run rendered whole beneath it. The stoppages behind this
item were each docketed twice, once as a stopped run and once as an unmerged
publication, so each one rendered both accounts. In the docket log, the stopped-run
entries for these items are 7.4 to 11 KiB of JSON each, and the publication
entries 3.8 to 5.4 KiB. Four of them, with the rest of their evidence, fill
48 KiB.

The renderer spent the bytes first come, first served, and stopped at the first
entry that did not fit whole (`if !fits(standing) { break }`). With the four
decided entries first, the first undecided entry never fit. Nothing from the
walk was listed. So the walk position never moved, and every later window,
whether from her conversation or from a scheduled pass, listed the same four.
The count of what remained was right: 29 further entries. The window had also
told her those came first next time, and they did not.

The same limit was measured on today's docket, 2026-09-30, by rendering the
product's own `docket.jsonl` through the unchanged code with the tracker's
statuses. The docket held 17 live stoppages and the window listed 5. Each entry
rendered at 6 to 12 KiB, so the 25-entry bound was never reached. None of those
17 was a decided entry with a stopped carry-out in that reading, because the
carry-out findings are joined from the triage records when the docket is built
and the measurement read the log alone. So it shows the byte limit on its own,
not the ordering.

## What changed

- **Undecided before decided.** Each live stoppage now records whether a
  decision still stands over all of its entries (`Stoppage.Decided`). `Walk`
  puts those in a third part, `Window.Decided`, listed after both the critical
  undecided entries and the walk. Within it, a carry-out stopped by a gate that
  will not clear on its own comes before one waiting on a gate that will. A
  lapsed decision is a question again and stays with the undecided. The walk
  position only moves over the walk, so a decided entry never moves it.
- **The byte bound is shared.** The renderer takes the first 25 entries of that
  order and works out the most bytes any one may take so that all of them fit
  (`docketEntryShare`). Entries within that share are listed whole, and the
  bytes they leave go to the longer ones. An entry past its share is cut at a
  line, keeps its heading, and ends with a line saying it was cut and how long
  it is (`cutDocketEntry`). So the window lists 25 entries whenever the docket
  holds more than 25.
- **The count says what remains.** The line counting what was not listed also
  says how many of those nobody has decided and how many are her decisions
  waiting on the harness.

`TestTheDocketWindowListsEveryUndecidedStoppageBeforeAnyDecidedOne` builds this
item's docket: four decided and gated entries, each carrying a failing check as
long as the ones that filled the window and a second docketing folded beneath
it, stopped before 29 undecided ones. It asserts that the window lists 25
undecided entries oldest first, says 8 remain (4 undecided and 4 decided), and
that the next window opens with the four undecided entries this one left out.
Against the code before this change it lists 17 entries, and a decided one comes
first. `TestTheDocketWindowSharesItsBytesSoEveryEntryIsListed` covers the other
case: where the undecided entries leave room, the decided ones follow, and long
entries are cut rather than crowding out the entries after them. Before this
change, that docket listed one entry.

## What this does not change

A decided entry still stops being shown while 25 or more undecided stoppages are
live. That follows from the order this item asks for. The development manager
reaches those entries once the undecided ones are decided. A cut entry shows
less of its evidence than it did. The cut is declared, and a window listing
fewer entries shows more of each.
