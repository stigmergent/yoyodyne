# The harness's own vocabulary

Every term of art the harness uses where a person or a role reads it: where it
appears, how often, what it means in one plain sentence, and a proposed
decision — replace it with ordinary words, or register it in
[the register](terms.md) as a term that names something real. The operator
asked for it on 2026-09-27, after a program manager's report reached him saying
"environmental stop" and "idle bound" and nothing told him what either meant.
It is the inventory of the harness's coined vocabulary (yoyodyne-ifd.437.18).

**This document is generated. Edit the script, not this file.** Run
`go run ./scripts/vocabulary > docs/vocabulary-inventory.md` from the
repository root. The terms, what they mean, and what is proposed for each are
in `scripts/vocabulary/terms.go`; the counts are measured each time it
runs. These were measured on 2026-09-28.

## Who decides

Every decision below is a proposal. The Lead Product Manager records the
decision on each term on the inventory's work item (yoyodyne-ifd.437.18), and
the items admitted beside it carry the decisions out:

- replacing the coined vocabulary in printed strings, role contracts, shipped
  personas, and pass prompts (yoyodyne-ifd.437.19);
- replacing it in the shipped guides (yoyodyne-ifd.437.20);
- the architect replacing it in the governed documents (yoyodyne-ifd.437.21);
- a terms check that refuses a term that is neither registered nor replaced
  (yoyodyne-ifd.437.22).

Three terms are already being replaced on their own item — 'environmental
stop', 'idle bound', and 'stall continuation' (yoyodyne-ifd.437.17) — and
what roles write for a person is being held to the register on its own item
too (yoyodyne-ifd.437.16). When a decision differs from the proposal, change the
term's entry in the script and run it again, so this document says what was
decided rather than what was proposed.

A term proposed for replacement keeps its name wherever it is an identifier: a
type, a field in a stored record, a configuration key, a package. What changes
is the words a person or a role reads.

## What was counted

- **Printed strings:** every string literal holding a space in the Go source,
  outside tests and test data, counted by package. That is what the commands
  print and their help, `yoyo status`, the development manager's list of
  stopped runs, Slack messages, and the read model the dashboard shows — and the
  role contracts and pass prompts too, which are Go strings. The dashboard's own
  files under `internal/dashboard/assets` are read whole.
- **Personas:** the shipped personas and bundle under
  `internal/config/builtin`.
- **Guides:** the README and every Markdown file under `docs/` that is
  not a governed document, the register, a record under `docs/diagnoses`,
  `docs/experiments` or `docs/releases`, or this document.
- **Governed documents:** every Markdown file under `docs/product`,
  `docs/designs`, and `docs/decisions`, less its frontmatter.

A count is of occurrences, matched at the start of a word and ignoring case, so
a stem counts its inflections: `docket` counts `docketed`. A
term written in parts is matched however its parts are spaced. Where a term's
ordinary use cannot be told from its use as a term, the entry says so, and its
count is an upper bound.

Not counted: the tracker's own items, which are the Lead Product Manager's to
reword; the records under `docs/`, which say what was true on a date in
the words used then; code comments; and a string literal with no space in it,
which is a key or an identifier rather than words anybody reads.

## Summary

| Term | Proposed | In the register today | Printed strings | Personas | Guides | Governed documents |
|---|---|---|---:|---:|---:|---:|
| [`stoppage`](#stoppage) | replace | — | 156 | 2 | 168 | 4 |
| [`docket`](#docket) | replace | registered | 210 | 1 | 251 | 10 |
| [`crossing`](#crossing) | replace | — | 56 | 0 | 112 | 5 |
| [`carry-out`](#carry-out) | replace | — | 27 | 0 | 27 | 1 |
| [`environmental stop`](#environmental-stop) | replace | — | 23 | 2 | 20 | 1 |
| [`idle bound`](#idle-bound) | replace | — | 0 | 0 | 1 | 0 |
| [`provider's stream`](#providers-stream) | replace | — | 1 | 0 | 4 | 0 |
| [`stall`](#stall) | replace | — | 98 | 0 | 146 | 3 |
| [`continuation`](#continuation) | replace | — | 35 | 0 | 46 | 1 |
| [`lane`](#lane) | replace | — | 130 | 15 | 100 | 73 |
| [`summons`](#summons) | replace | — | 38 | 2 | 47 | 0 |
| [`park`](#park) | register | — | 129 | 0 | 204 | 12 |
| [`pull`](#pull) | replace | — | 233 | 6 | 299 | 5 |
| [`the line`](#the-line) | replace | — | 69 | 2 | 115 | 3 |
| [`brake`](#brake) | replace | registered | 82 | 4 | 134 | 4 |
| [`probe`](#probe) | replace | — | 95 | 4 | 142 | 5 |
| [`watch session`](#watch-session) | replace | — | 61 | 1 | 110 | 2 |
| [`the standing`](#the-standing) | replace | — | 12 | 0 | 24 | 10 |
| [`settle`](#settle) | replace | — | 195 | 1 | 284 | 20 |
| [`witness`](#witness) | register | — | 21 | 0 | 38 | 1 |
| [`drain`](#drain) | replace | — | 43 | 2 | 98 | 0 |
| [`mover`](#mover) | replace | — | 79 | 3 | 31 | 7 |
| [`shadow review`](#shadow-review) | register | — | 34 | 0 | 18 | 0 |
| [`landing`](#landing) | replace | — | 190 | 9 | 250 | 10 |
| [`exchange`](#exchange) | register | — | 150 | 0 | 106 | 25 |
| [`intake hold`](#intake-hold) | register | — | 154 | 2 | 149 | 16 |
| [`operator hold`](#operator-hold) | replace | — | 28 | 0 | 25 | 1 |
| [`sweep`](#sweep) | register | — | 120 | 2 | 292 | 13 |
| [`firing`](#firing) | replace | — | 50 | 1 | 70 | 3 |
| [`pass`](#pass) | replace | — | 100 | 2 | 203 | 46 |
| [`context bundle`](#context-bundle) | replace | — | 0 | 0 | 11 | 4 |
| [`repair`](#repair) | register | — | 222 | 11 | 433 | 18 |
| [`repair grant`](#repair-grant) | replace | — | 32 | 0 | 50 | 0 |
| [`needs-a-human`](#needs-a-human) | replace | — | 23 | 1 | 35 | 1 |
| [`integration target`](#integration-target) | replace | — | 28 | 0 | 4 | 0 |
| [`promotion`](#promotion) | replace | — | 125 | 2 | 393 | 61 |
| [`lease`](#lease) | replace | — | 39 | 0 | 166 | 37 |
| [`floor`](#floor) | replace | — | 25 | 0 | 21 | 4 |
| [`triage`](#triage) | register | — | 320 | 4 | 298 | 12 |
| [`protected path`](#protected-path) | register | — | 10 | 0 | 84 | 6 |
| [`direct-work`](#direct-work) | register | — | 14 | 0 | 54 | 2 |
| [`side thread`](#side-thread) | register | — | 138 | 0 | 75 | 13 |
| [`raise`](#raise) | replace | — | 25 | 0 | 20 | 0 |
| [`remit`](#remit) | replace | — | 6 | 1 | 12 | 9 |
| [`wake`](#wake) | replace | — | 37 | 0 | 29 | 3 |
| [`gate`](#gate) | replace | — | 52 | 4 | 223 | 50 |
| [`usage window`](#usage-window) | replace | — | 23 | 0 | 33 | 1 |
| [`sink`](#sink) | replace | registered | 91 | 1 | 217 | 17 |
| [`heartbeat`](#heartbeat) | keep | registered | 5 | 0 | 32 | 1 |
| [`steer`](#steer) | keep | registered | 13 | 0 | 44 | 1 |
| [`handback`](#handback) | keep | registered | 0 | 0 | 1 | 0 |
| [`discharge`](#discharge) | keep | registered | 22 | 0 | 20 | 1 |
| [`re-arm`](#re-arm) | keep | registered | 94 | 0 | 114 | 1 |
| [`seat`](#seat) | keep | registered | 8 | 1 | 19 | 0 |
| [`minute zero`](#minute-zero) | keep | registered | 2 | 0 | 2 | 2 |

## The terms

### stoppage

- **Means:** A run that stopped and is waiting for the development manager to decide what happens to it.
- **Proposed:** replace it. Write instead: a stopped run, or a run that stopped.
- **Where:** 330 in all, in 23 places: `internal/orchestrator` 62, `docs/conversation.md` 40, `docs/configuration.md` 33, `docs/work.md` 32, `internal/runstate` 31, `docs/operations.md` 25, and 17 more.

### docket

- **Means:** The development manager's list of stopped runs, runs that died before they started, and items dispatch would not start; `docketed` is being put on it.
- **Proposed:** replace it. Write instead: the development manager's list of stopped runs; for `docketed`, put in front of the development manager. The register's row is retired once the replacements land; `DocketStore` and other identifiers keep their names.
- **Where:** 472 in all, in 25 places: `internal/orchestrator` 77, `docs/conversation.md` 62, `docs/work.md` 53, `docs/operations.md` 50, `internal/cli` 41, `docs/configuration.md` 39, and 19 more.
- **Note:** Registered today. It is proposed for replacement because it is the most frequent term in this inventory and the operator asked what `docketed` meant.

### crossing

- **Means:** Two things: the development manager raising one item's budget cap by one step past a refusal, with a reason; and a conversation turn answered by a permitted alternate model when the configured one refused.
- **Proposed:** replace it. Write instead: for a budget, raising the cap ("raised the repair cap to 3"); for a model, a turn answered by the alternate model. The triage decision value `cross` keeps its name.
- **Where:** 173 in all, in 21 places: `docs/configuration.md` 42, `docs/configuration/recovery.md` 41, `internal/runstate` 17, `internal/chat` 15, `internal/notify` 11, `docs/authority-inventory.md` 9, and 15 more.

### carry-out

- **Means:** The harness acting on a decision the development manager recorded about a stopped run.
- **Proposed:** replace it. Write instead: carrying out her decision; for the queue of them, decisions waiting to be carried out.
- **Where:** 55 in all, in 16 places: `internal/runstate` 16, `docs/operations.md` 7, `docs/work.md` 7, `internal/dashboard/assets/dashboard.js` 3, `internal/orchestrator` 3, `docs/configuration.md` 3, and 10 more.
- **Note:** Only the hyphenated noun is counted; the verb `carry out` is ordinary.

### environmental stop

- **Means:** A run ended by something outside the work — the sandbox, the network, the tracker, a provider limit — which spends none of the item's budgets.
- **Proposed:** replace it. Write instead: stopped by something outside the work, naming what it was.
- **Where:** 46 in all, in 13 places: `internal/runstate` 14, `docs/operations.md` 6, `internal/triage` 5, `docs/work.md` 5, `internal/orchestrator` 3, `docs/configuration.md` 3, and 7 more.
- **Note:** The item replacing the three words the operator met first (yoyodyne-ifd.437.17) replaces this one. `environmental refusal` and `environmental cause` are counted with it.

### idle bound

- **Means:** The harness ending a run whose AI session produced no output for a set time.
- **Proposed:** replace it. Write instead: the time limit on a session that produces no output: "produced no output for five minutes, so the harness ended the run".
- **Where:** 1 in all, in 1 place: `docs/developing-yoyo.md` 1.
- **Note:** The item replacing the three words the operator met first (yoyodyne-ifd.437.17) replaces this one.

### provider's stream

- **Means:** The output of the AI session running a role; `went silent` is that output stopping.
- **Proposed:** replace it. Write instead: the AI session's output; for `went silent`, produced no output.
- **Where:** 5 in all, in 5 places: `internal/cli` 1, `docs/configuration.md` 1, `docs/configuration/publishing.md` 1, `docs/provider-plugins.md` 1, `docs/reporting.md` 1.

### stall

- **Means:** Nothing starting although work is ready, or a session producing no output.
- **Proposed:** replace it. Write instead: nothing is starting although work is ready; or, of a session, produced no output.
- **Where:** 247 in all, in 25 places: `docs/operations.md` 59, `docs/reporting.md` 28, `internal/cli` 27, `internal/runstate` 22, `internal/dashboard/assets/dashboard.js` 21, `docs/slack/setup.md` 18, and 19 more.
- **Note:** Counts include the `--stall-after` flag of `yoyo work`, whose name changes only with the command; a rename is its own decision.

### continuation

- **Means:** Resuming a stopped run in its kept worktree and session rather than starting it fresh; a `stall continuation` resumes one ended for producing no output, a `repair continuation` resumes one mid-repair.
- **Proposed:** replace it. Write instead: resuming the stopped run in the same session.
- **Where:** 82 in all, in 16 places: `internal/orchestrator` 21, `docs/operations.md` 15, `docs/conversation.md` 11, `docs/configuration.md` 7, `internal/runstate` 6, `docs/configuration/recovery.md` 5, and 10 more.
- **Note:** The item replacing the three words the operator met first (yoyodyne-ifd.437.17) replaces `stall continuation`.

### lane

- **Means:** The work stream one program manager owns, identified by its tracker label.
- **Proposed:** replace it. Write instead: work stream, the word the register's `program manager` row already uses; for `lane report`, the program manager's report; for `lane label`, the work stream's label.
- **Where:** 318 in all, in 29 places: `internal/chat` 72, `docs/designs/program-manager.md` 56, `docs/conversation.md` 35, `docs/configuration.md` 29, `internal/runstate` 21, `docs/authority-inventory.md` 20, and 23 more.

### summons

- **Means:** The harness starting a development manager turn at once rather than at its next scheduled time.
- **Proposed:** replace it. Write instead: starts the development manager's turn at once.
- **Where:** 87 in all, in 14 places: `docs/configuration.md` 15, `internal/orchestrator` 12, `internal/runstate` 9, `docs/configuration/runs.md` 9, `internal/cli` 8, `docs/operations.md` 7, and 8 more.

### park

- **Means:** Keeping an item in the Lead Product Manager's order but stopping the harness from selecting it until somebody releases it with `unpark`, with the reason shown wherever it is listed.
- **Proposed:** register it, with the meaning above: `park` and `unpark` are actions a role records and the operator reads in backlog listings, so the word is a name rather than decoration.
- **Where:** 345 in all, in 33 places: `docs/operations.md` 56, `docs/work.md` 52, `internal/chat` 44, `docs/conversation.md` 35, `internal/orchestrator` 27, `internal/cli` 17, and 27 more.

### pull

- **Means:** One time the harness picks the next ready item and starts a run on it.
- **Proposed:** replace it. Write instead: the next time the harness picks work.
- **Where:** 543 in all, in 34 places: `internal/orchestrator` 70, `docs/work.md` 70, `docs/operations.md` 60, `docs/configuration.md` 59, `docs/configuration/runs.md` 36, `internal/notify` 35, and 28 more.
- **Note:** `pull request` is not counted. `git pull` and other ordinary uses are, so the counts are an upper bound.

### the line

- **Means:** The harness pictured as a production line choosing and running work, as in "the line is choosing nothing".
- **Proposed:** replace it. Write instead: the harness, or say what is happening: "no work is starting".
- **Where:** 189 in all, in 28 places: `docs/operations.md` 39, `internal/notify` 33, `docs/conversation.md` 15, `docs/slack/setup.md` 15, `docs/configuration.md` 14, `docs/reporting.md` 10, and 22 more.
- **Note:** Counts include ordinary uses such as "the top line" of a message, so they are an upper bound.

### brake

- **Means:** The automatic hold on starting new work after a set number of blocked runs in a row, also written `storm brake`.
- **Proposed:** replace it. Write instead: the automatic stop on starting work; for `braked`, stopped. The register's row is retired once the replacements land.
- **Where:** 224 in all, in 19 places: `docs/operations.md` 29, `docs/configuration.md` 28, `docs/configuration/runs.md` 22, `docs/reporting.md` 20, `internal/runstate` 17, `internal/chat` 15, and 13 more.
- **Note:** Registered today. Proposed for replacement because the operator has asked for 'stopped' in place of 'braked'.

### probe

- **Means:** Two things: one trial run started under the automatic stop to see whether work can land again; and the first command a developer runs in its worktree to show commands can run there.
- **Proposed:** replace it. Write instead: a trial run; for the developer's, a first command run to show commands work. The decision value `probe` and the verification block's `probe` field keep their names.
- **Where:** 246 in all, in 24 places: `docs/configuration.md` 37, `docs/operations.md` 36, `internal/orchestrator` 29, `internal/runstate` 22, `docs/configuration/runs.md` 19, `docs/configuration/recovery.md` 16, and 18 more.

### watch session

- **Means:** The long-running `yoyo work --watch` process that picks ready work and starts runs.
- **Proposed:** replace it. Write instead: the process that starts work, or `yoyo work --watch` where the command is the point.
- **Where:** 174 in all, in 19 places: `docs/operations.md` 43, `docs/work.md` 15, `docs/configuration.md` 14, `docs/reporting.md` 14, `docs/slack/setup.md` 13, `internal/cli` 12, and 13 more.

### the standing

- **Means:** The list of role instances and what each is doing, where a restart request is shown.
- **Proposed:** replace it. Write instead: the list of role instances.
- **Where:** 46 in all, in 12 places: `docs/operations.md` 15, `internal/dashboard/assets/dashboard.js` 10, `docs/configuration.md` 3, `docs/slack/setup.md` 3, `docs/designs/program-manager.md` 3, `internal/chat` 2, and 6 more.
- **Note:** Counts include ordinary uses such as "the standing decision", so they are an upper bound.

### settle

- **Means:** Bringing something interrupted or half-finished to a recorded end: a run, a directive, a promotion.
- **Proposed:** replace it. Write instead: finish, resolve, or record how it ended — whichever the sentence means.
- **Where:** 500 in all, in 45 places: `docs/operations.md` 99, `internal/orchestrator` 46, `docs/configuration.md` 42, `docs/conversation.md` 37, `internal/cli` 29, `docs/work.md` 28, and 39 more.

### witness

- **Means:** The tracker's recorded copy of an item's `Goal served:` line, kept so a destroyed attribution can be found and restored; `yoyo goals witness` records it.
- **Proposed:** register it, with the meaning above: it is a subcommand of `yoyo goals`, so the word is typed and cannot be swept out of its help without renaming the command.
- **Where:** 60 in all, in 10 places: `internal/cli` 14, `docs/configuration.md` 13, `docs/configuration/goals.md` 13, `docs/artifacts.md` 11, `internal/beads` 3, `internal/goal` 2, and 4 more.

### drain

- **Means:** A queue emptying, or a restarting process waiting for the runs it hosts to finish first.
- **Proposed:** replace it. Write instead: empties; for a restart, waiting for its runs to finish. `execution.redeploy_drain_limit` keeps its name.
- **Where:** 143 in all, in 16 places: `docs/configuration.md` 31, `docs/operations.md` 24, `docs/work.md` 19, `internal/orchestrator` 17, `docs/configuration/runs.md` 11, `internal/cli` 10, and 10 more.

### mover

- **Means:** Whoever an item or a stopped run is waiting on to act next.
- **Proposed:** replace it. Write instead: who it is waiting on. The `waiting_on` field keeps its name.
- **Where:** 120 in all, in 11 places: `internal/dashboard/assets/dashboard.js` 64, `docs/operations.md` 17, `internal/triage` 10, `docs/conversation.md` 7, `docs/designs/program-manager.md` 6, `docs/work.md` 4, and 5 more.

### shadow review

- **Means:** A second review of the same change by another model, recorded for comparison and deciding nothing.
- **Proposed:** register it, with the meaning above: it is the `--shadow` flag of `yoyo review`.
- **Where:** 52 in all, in 5 places: `internal/cli` 25, `docs/work.md` 16, `internal/dashboard/assets/dashboard.css` 6, `internal/shadow` 3, `docs/reporting.md` 2.

### landing

- **Means:** A change merging onto the target branch; in a developer's reply, its claim of what that merge does to the item — closes it, lands evidence, or escalates.
- **Proposed:** replace it. Write instead: merge, or merged change; for a developer's claim, what the change does to the item. The `yoyodyne-landing` block keeps its name.
- **Where:** 459 in all, in 34 places: `docs/configuration.md` 96, `docs/operations.md` 69, `internal/orchestrator` 57, `internal/notify` 37, `internal/runstate` 28, `docs/work.md` 26, and 28 more.

### exchange

- **Means:** A question one role puts to another, which the harness delivers by invoking the other role and records with what it cost.
- **Proposed:** register it, with the meaning above: it is the command `yoyo exchange`.
- **Where:** 281 in all, in 26 places: `internal/cli` 41, `internal/exchange` 38, `docs/conversation.md` 24, `internal/notify` 20, `internal/runstate` 20, `docs/configuration.md` 19, and 20 more.
- **Note:** Counts include ordinary uses, so they are an upper bound.

### intake hold

- **Means:** A stop on the harness choosing new work by itself, placed by the operator or by the automatic stop; `yoyo release` lifts it, and work already running carries on.
- **Proposed:** register it, with the meaning above: it names the operator's control over autonomous work, is written in an architectural invariant, and has its own command to lift it.
- **Where:** 321 in all, in 31 places: `internal/orchestrator` 30, `internal/runstate` 30, `docs/configuration.md` 26, `internal/chat` 25, `internal/cli` 25, `internal/notify` 25, and 25 more.

### operator hold

- **Means:** The operator pausing everything the harness would spend on a provider, with `yoyo pause`.
- **Proposed:** replace it. Write instead: the operator's pause, after the command that sets it.
- **Where:** 54 in all, in 15 places: `docs/delivery-pipeline-baseline.md` 13, `internal/runstate` 12, `internal/orchestrator` 7, `docs/slack/setup.md` 4, `docs/team-mode-coordination.md` 3, `internal/cli` 2, and 9 more.

### sweep

- **Means:** One run of a recurring task and the record it leaves; `yoyo sweeps` reads them.
- **Proposed:** register it, with the meaning above: it is the command `yoyo sweeps` and the heading every recurring task's findings are listed under.
- **Where:** 427 in all, in 35 places: `docs/operations.md` 131, `docs/configuration.md` 37, `internal/cli` 36, `internal/orchestrator` 33, `internal/runstate` 29, `docs/conversation.md` 26, and 29 more.

### firing

- **Means:** One scheduled run of a recurring task, as in `FAILED FIRING`.
- **Proposed:** replace it. Write instead: run of the task; `FAILED FIRING` becomes `FAILED RUN`.
- **Where:** 124 in all, in 15 places: `docs/configuration.md` 28, `docs/operations.md` 27, `internal/orchestrator` 15, `internal/notify` 10, `internal/cli` 8, `internal/runstate` 8, and 9 more.

### pass

- **Means:** One scheduled turn of a program manager, the supervisor, or the scheduler.
- **Proposed:** replace it. Write instead: turn, or run — whichever the role does.
- **Where:** 351 in all, in 32 places: `docs/operations.md` 57, `docs/configuration.md` 56, `docs/work.md` 39, `internal/cli` 34, `internal/orchestrator` 26, `docs/designs/program-manager.md` 24, and 26 more.
- **Note:** Only noun phrases are counted; the verb, as in "checks pass", is ordinary.

### context bundle

- **Means:** The documents and state a role is given to read at the start of each turn.
- **Proposed:** replace it. Write instead: what the role is given to read at the start of its turn.
- **Where:** 15 in all, in 8 places: `docs/configuration.md` 3, `docs/delivery-pipeline-baseline.md` 3, `docs/designs/v1-harness-design.md` 3, `docs/configuration/artifacts.md` 2, `docs/configuration/runs.md` 1, `docs/developing-yoyo.md` 1, and 2 more.

### repair

- **Means:** A developer's further attempt at its own change, in the same worktree, after a failed check or review findings; `yoyo triage repair` asks for one.
- **Proposed:** register it, with the meaning above: it is a `yoyo triage` verb and a budget the status line counts.
- **Where:** 684 in all, in 35 places: `docs/configuration.md` 107, `internal/orchestrator` 85, `docs/configuration/recovery.md` 67, `docs/conversation.md` 48, `docs/work.md` 46, `internal/chat` 42, and 29 more.
- **Note:** Counts include ordinary uses such as "backlog repair", so they are an upper bound.

### repair grant

- **Means:** The development manager allowing a stopped item further review rounds.
- **Proposed:** replace it. Write instead: further review rounds granted.
- **Where:** 82 in all, in 13 places: `docs/configuration.md` 16, `docs/configuration/recovery.md` 14, `internal/orchestrator` 12, `docs/operations.md` 7, `internal/cli` 6, `internal/runstate` 6, and 7 more.

### needs-a-human

- **Means:** The line in `yoyo status` and the channel listing what is waiting on a person.
- **Proposed:** replace it. Write instead: waiting on you, the operator's own words.
- **Where:** 60 in all, in 17 places: `docs/operations.md` 22, `internal/dashboard/assets/dashboard.js` 6, `docs/reporting.md` 6, `internal/orchestrator` 4, `internal/cli` 3, `internal/readmodel` 3, and 11 more.

### integration target

- **Means:** The branch an approved change is merged onto.
- **Proposed:** replace it. Write instead: target branch, which the same strings already use.
- **Where:** 32 in all, in 9 places: `internal/gitworktree` 17, `internal/orchestrator` 4, `internal/runstate` 4, `docs/delivery-pipeline-baseline.md` 2, `internal/chat` 1, `internal/readmodel` 1, and 3 more.

### promotion

- **Means:** Moving the target branch forward to include an approved change.
- **Proposed:** replace it. Write instead: merging the change onto the target branch.
- **Where:** 581 in all, in 38 places: `docs/operations.md` 88, `docs/configuration.md` 67, `internal/orchestrator` 54, `docs/work.md` 49, `docs/delivery-pipeline-baseline.md` 38, `docs/configuration/publishing.md` 32, and 32 more.
- **Note:** An architectural invariant says `promotion`; its wording is the architect's.

### lease

- **Means:** A lock one process holds so no other process does the same thing at the same time.
- **Proposed:** replace it. Write instead: lock.
- **Where:** 242 in all, in 30 places: `docs/authority-inventory.md` 54, `docs/operations.md` 43, `docs/configuration.md` 21, `internal/runstate` 15, `docs/designs/management-and-supervision.md` 10, `docs/designs/recoverable-and-terminal-failures.md` 9, and 24 more.
- **Note:** `release` is a different word and is not counted.

### floor

- **Means:** A total that is at least the figure shown, because some runs left no record to price.
- **Proposed:** replace it. Write instead: at least: "at least $4.20; 2 runs could not be priced".
- **Where:** 50 in all, in 16 places: `internal/dashboard/assets/dashboard.js` 14, `docs/configuration.md` 6, `internal/cli` 5, `docs/developing-yoyo.md` 5, `internal/orchestrator` 3, `internal/chat` 2, and 10 more.

### triage

- **Means:** The development manager deciding what happens to each stopped run, and `yoyo triage` carrying the decision out.
- **Proposed:** register it, with the meaning above: it is the command `yoyo triage`.
- **Where:** 634 in all, in 36 places: `internal/orchestrator` 102, `internal/runstate` 77, `docs/configuration.md` 76, `internal/cli` 55, `docs/configuration/recovery.md` 53, `internal/chat` 43, and 30 more.

### protected path

- **Means:** A path a developer's change may not touch unless its work item carries a `protected-path grant:` line naming it.
- **Proposed:** register it, with the meaning above: it is the grant line an item carries and the refusal a developer receives.
- **Where:** 100 in all, in 21 places: `docs/authority-inventory.md` 29, `docs/configuration.md` 19, `internal/orchestrator` 7, `docs/configuration/artifacts.md` 7, `docs/delivery-pipeline-baseline.md` 7, `docs/configuration/agents.md` 4, and 15 more.

### direct-work

- **Means:** The two authorities a person can hold in a project's configuration: `own-intent`, over what the product is for, and `direct-work`, over work already running.
- **Proposed:** register it, with the meaning above: they are configuration values a person writes.
- **Where:** 70 in all, in 10 places: `docs/slack/setup.md` 19, `docs/configuration.md` 12, `docs/configuration/agents.md` 11, `internal/slack` 7, `docs/reporting.md` 6, `internal/config` 5, and 4 more.

### side thread

- **Means:** A secondary conversation a role opens to work something out, which judges and drafts but acts on nothing, and merges its conclusion back into the main conversation.
- **Proposed:** register it, with the meaning above: register `side thread` and write it for `side stream`, which is the same thing; the `sidestream` package keeps its name.
- **Where:** 226 in all, in 14 places: `internal/cli` 45, `internal/runstate` 39, `internal/sidestream` 36, `docs/authority-inventory.md` 32, `docs/configuration.md` 19, `internal/agentcontext` 14, and 8 more.

### raise

- **Means:** A role putting an item in front of the development manager as unmeetable as written.
- **Proposed:** replace it. Write instead: escalating the item as unmeetable.
- **Where:** 45 in all, in 6 places: `internal/chat` 19, `docs/conversation.md` 17, `internal/triage` 3, `docs/work.md` 3, `internal/orchestrator` 2, `internal/runstate` 1.

### remit

- **Means:** What a program manager is responsible for.
- **Proposed:** replace it. Write instead: what it is responsible for.
- **Where:** 28 in all, in 7 places: `docs/configuration.md` 12, `docs/designs/program-manager.md` 7, `internal/config` 3, `internal/chat` 2, `docs/designs/tool-interface.md` 2, `internal/cli` 1, and 1 more.

### wake

- **Means:** The harness starting another turn of a role or resuming a waiting process.
- **Proposed:** replace it. Write instead: started again, or given another turn.
- **Where:** 69 in all, in 16 places: `internal/orchestrator` 21, `docs/configuration.md` 16, `internal/cli` 7, `docs/work.md` 5, `internal/config` 3, `docs/operations.md` 3, and 10 more.

### gate

- **Means:** A check a change has to pass before it merges: the configured checks, independent review, the protected paths.
- **Proposed:** replace it. Write instead: name the check.
- **Where:** 329 in all, in 49 places: `docs/configuration.md` 61, `docs/operations.md` 23, `docs/developing-yoyo.md` 22, `docs/delivery-pipeline-baseline.md` 21, `docs/conversation.md` 20, `docs/work.md` 17, and 43 more.
- **Note:** Counts are of the whole word only, so `gates` and `gated` are not included.

### usage window

- **Means:** The period a provider's usage limit applies to, after which it resets.
- **Proposed:** replace it. Write instead: the provider's usage limit, until it resets at a named time.
- **Where:** 57 in all, in 13 places: `docs/operations.md` 13, `docs/reporting.md` 9, `internal/orchestrator` 7, `docs/slack/setup.md` 7, `internal/readmodel` 6, `internal/runstate` 5, and 7 more.

### sink

- **Means:** The process that posts to Slack.
- **Proposed:** replace it. Write instead: the Slack reporter. The register's row is retired once the replacements land; `internal/slack` identifiers keep their names.
- **Where:** 326 in all, in 23 places: `docs/slack/setup.md` 95, `docs/reporting.md` 43, `internal/cli` 35, `internal/slack` 27, `internal/doctor` 24, `docs/operations.md` 23, and 17 more.
- **Note:** Registered today. The coined-terms sweep (yoyodyne-ifd.206) called it the strongest rename candidate: a dataflow term with no ordinary meaning that helps.

### heartbeat

- **Means:** How often `yoyo slack` repeats that no work is starting.
- **Proposed:** no change. It is registered; it is a flag name whose help says it in plain words.
- **Where:** 38 in all, in 9 places: `docs/slack/setup.md` 15, `docs/reporting.md` 8, `docs/operations.md` 6, `internal/cli` 4, `internal/slack` 1, `docs/configuration.md` 1, and 3 more.

### steer

- **Means:** Directing the work, or changing what is being worked on.
- **Proposed:** no change. It is registered; an ordinary verb that reads correctly where it is used.
- **Where:** 58 in all, in 15 places: `docs/slack/setup.md` 19, `internal/slack` 6, `docs/configuration.md` 5, `docs/reporting.md` 4, `internal/cli` 3, `docs/configuration/agents.md` 3, and 9 more.

### handback

- **Means:** Handing the work back to the developer that made it.
- **Proposed:** no change. It is registered; it reaches nobody outside code.
- **Where:** 1 in all, in 1 place: `docs/work.md` 1.

### discharge

- **Means:** A change being the work its item asked for, so the item closes on it.
- **Proposed:** no change. It is registered; the developer and reviewer contracts decide on it.
- **Where:** 43 in all, in 10 places: `internal/orchestrator` 13, `docs/work.md` 9, `internal/landing` 8, `docs/delivery-pipeline-baseline.md` 6, `docs/operations.md` 2, `internal/review` 1, and 4 more.

### re-arm

- **Means:** Repeating a merge request a forge dropped, once per publication.
- **Proposed:** no change. It is registered; it is the verb `yoyo triage rearm`.
- **Where:** 209 in all, in 15 places: `internal/orchestrator` 42, `docs/configuration.md` 32, `docs/configuration/recovery.md` 29, `internal/cli` 23, `docs/operations.md` 21, `docs/conversation.md` 20, and 9 more.

### seat

- **Means:** One running instance of a role, as against the developer slot it fills.
- **Proposed:** no change. It is registered; the operator's own word.
- **Where:** 28 in all, in 8 places: `docs/configuration.md` 9, `docs/operations.md` 7, `internal/orchestrator` 3, `docs/configuration/runs.md` 3, `internal/chat` 2, `internal/runstate` 2, and 2 more.

### minute zero

- **Means:** Before development begins.
- **Proposed:** no change. It is registered while an invariant's wording carries it; it retires when the architect rewords that invariant.
- **Where:** 6 in all, in 4 places: `internal/terms` 2, `docs/decisions/invariants/developer-verifies-before-submitting.md` 2, `docs/configuration.md` 1, `docs/developing-yoyo.md` 1.

## The stop-cause words

The run record naming what stopped it (yoyodyne-ifd.409) prints one of ten words
in front of every stopped run's reason, as in `provider: …`. That item
asked for whoever owns their wording to confirm them before anything depends on
them, and the Lead Product Manager put the question here. They are listed and
not counted: that item's change is not on the main branch yet, and most of the
words are ordinary words a count could not tell from their ordinary use. What
is proposed is to print the plain phrase in place of the bare word, keeping the
word as the value the record stores.

| Word | Means | Proposed |
|---|---|---|
| `checks` | A configured check kept failing or could not run, a protected path kept being touched, or nothing was recorded running. | print instead: stopped at the checks |
| `review` | Findings the repair budget could not resolve, a reviewer that could not answer, or an approval whose independence could not be shown. | print instead: stopped at review |
| `integration` | Merging onto the target branch failed: the target kept moving, the change conflicts, or local and remote went different ways. | print instead: could not be merged |
| `publish` | Publishing the merged change to the forge failed, although the local merge landed. | print instead: could not be published to the forge |
| `cleanup` | Removing the run's worktree and branch after its work merged failed. | print instead: merged, but its worktree could not be removed |
| `recording` | The run's work was done but its completion record could not be written. | print instead: done, but the harness could not record that it finished |
| `provider` | The AI provider ended the run without the work being judged. | print instead: the AI provider ended the run |
| `environmental` | Something outside the work refused the run, which spends none of its budgets. | print instead: stopped by something outside the work |
| `cancelled` | The operator asked the run to stop, or its context was cancelled. | an ordinary word; keep it |
| `harness` | One of the harness's own steps around the work failed: saving state, writing the tracker, making a scratch directory. | print instead: the harness's own step failed |

## Finding terms this list does not have

`go run ./scripts/vocabulary -candidates` lists the hyphenated compounds in
the Go strings and the personas that no term above covers, most frequent first.
Nothing mechanical tells a coinage from an ordinary compound, so the list is for
a person to read: a term found there is added to the script with its
meaning and a proposal, and the document is run again. Keeping the vocabulary
from growing unnoticed between readings is what the terms check that refuses
unregistered terms (yoyodyne-ifd.437.22) is for.
