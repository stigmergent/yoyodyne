# Diagnosis: persona changes delivered to the template and not to the copy the roles read

Work item: yoyodyne-ifd.430.26. This records which role-behaviour changes had
reached the copy the roles read when the run started, and how each came to be
carried across. It is the list the item's first done-condition asks for.

Read against base commit `cc9c93b2` and the tracker export copied into the run's
worktree on 2026-09-27. A persona is "live" when `.yoyodyne/personas/<role>.md`
at that commit carried its wording; the template is
`internal/config/builtin/v1/personas/`. A role contract is Go compiled into the
harness, so it reaches the roles with the build they run. There is no template
copy of a contract to fall behind.

## Items closed on 2026-09-27 whose deliverable was role behaviour

| Item | Closed (UTC) | Contract (Go) | Persona: live copy carried it at `cc9c93b2`? | Now |
| --- | --- | --- | --- | --- |
| No role routes an approval to the operator (yoyodyne-ifd.430.21) | 18:17 | carried, in `terms.DecideAndReport` | **no**: the template only, all six personas | carried by the operator's hand change |
| An item named to a person is named by what it is (yoyodyne-ifd.430.22) | 08:51 | carried, in `terms.ItemNaming` | **no**: the template only, all six personas | carried by the operator's hand change |
| The development manager stops a run in flight whose work she decides is superseded (yoyodyne-ifd.428.42) | 10:36 | carried, in `internal/chat/role.go` | no persona change | nothing to carry |
| A Lead Product Manager decision about in-flight work reaches the development manager (yoyodyne-ifd.428.40) | 15:14 | carried, in `internal/chat/role.go` | no persona change | nothing to carry |
| The development manager's hourly sweep (yoyodyne-ifd.283) | 10:08 | Go, and its prompt is in `.yoyodyne/config.yaml`, which has no template copy | no persona change | nothing to carry |

Closed the same day and not role behaviour, so not listed above: the
titles-beside-numbers surfaces (yoyodyne-ifd.432.18), the program manager's
report card without a file path (yoyodyne-ifd.430.13.14), and a program manager
instance's missed pass (yoyodyne-ifd.430.13.15). Each changed code that renders
or records, and none changed a persona or a contract.

## Older template-only wording the same levelling carried

The operator's hand change also carried wording from items that closed before
2026-09-27. Each had reached the template and never the live copy:

| Item | Closed (UTC) | Wording | Live copy carried it at `cc9c93b2`? |
| --- | --- | --- | --- |
| Every coined term registered with a plain-word definition (yoyodyne-ifd.216) | 2026-08-31 | reviewer: the coined-terms check | no |
| The out-of-scope disposition, distinct from minor (yoyodyne-ifd.359) | 2026-09-25 | reviewer: minor is a severity, out-of-scope a disposition | no |
| The template ships a program manager persona (yoyodyne-ifd.430.13.9) | 2026-09-26 | program manager: the opening paragraph, the lane wording, and the lane report as its own section | no: the live copy was added by hand on 2026-09-25 (`50a7b681`) and never followed the template |

The Lead Product Manager's persona also carried one more 430.21 change: where a
directive conflicts with the goals, the question is the operator's only if every
resolution moves what the goals admit. That change is included in the 430.21 row
above.

## How the live copies were brought level

While this run was in review, the operator brought all six live copies level by
hand (`3372693d`, 2026-09-27 11:29 PDT) and moved their versions to v2. That
change carried everything in the two tables above, and it also rewrote parts of
each persona in ordinary words. This run's own levelling of the same files was
dropped in favour of it.

Two things were still missing after it, and this run handles both:

- **The specification home (yoyodyne-ifd.433.13, closed 2026-09-28 02:19 UTC).**
  It put a passage into all six templates saying every document in the product's
  specification home is authoritative product intent. It reached no live copy,
  which is the same gap again, one day later. This run carries it into all six:
  the program manager's version joins the plain-words reading bullet the
  operator wrote, and the other five are the template's own words.
- **The plain-words rewordings.** Fifteen template passages across the
  architect, development manager, Lead Product Manager, and program manager
  personas now read differently in the live copy, meaning the same in plainer
  words. The templates follow under yoyodyne-ifd.430.23, which is still open.
  Until then each one is declared in the test's `templateOnlyPassages` with that
  reason. A declaration that stops matching anything fails the test, so each
  one is removed as its template passage catches up.

## What the live copies keep that the template does not

These stand in the live copies and not in the template, and are kept:

- every persona: the operator's "Writing for a person" section, which the
  templates take under yoyodyne-ifd.430.23;
- program manager: the lane report's summary opening with Completed, Handed off,
  and Blocked on a human;

- development manager: "Remember what you report" (added to the live persona alone on
  2026-09-25, `46a0dcd4`);
- program manager: the pointer to `docs/designs/program-manager.md`, the design
  this repository's instance works under.

`TestThisRepositorysPersonasCarryEveryTemplatePassage` in
`internal/config/livepersona_repository_test.go` now fails whenever the template
carries a passage the live copy does not, unless the passage is declared there
with a reason. A copy may still say more than the template.
