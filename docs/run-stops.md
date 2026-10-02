# What stops a run or the carrying out of a decision

*For an operator or development manager reading why work cannot continue.*

## Permanent carry-out refusals

These gates act before the harness carries out a recorded repair, re-run, or
merge re-arm. They do not end a new developer run. The refusal keeps its own
words on the item's triage record and notes and on the development manager's
docket. It is delivered once as a new stoppage, including when she has already
been shown the original stopped run. No later pull retries that decision until
she changes it. There is no retry interval for these causes and no configuration
can make the same decision retry them.

The exact note owed to the item is saved with the refusal. A tracker that cannot
take the note leaves it pending for later pulls, which retry the note alone and
check for an append that already landed before writing it again. A new decision
or a cleared finding does not discard a pending note.

| Recorded cause | What the harness found | What can move it |
| --- | --- | --- |
| `worktree-gone` | The preserved checkout was retired or is missing, or the run recorded none to continue. | A re-run from the target branch, or an escalation about the change that was lost. |
| `branch-gone` | The stopped run's branch was checked and is missing. | A re-run or an escalation; a repair cannot continue the deleted branch. |
| `head-moved` | The checkout's HEAD differs from the commit the harness recorded. | A re-run or an escalation about what changed; the harness does not reset it to get past the gate. |
| `decision-superseded` | The durable decision is no longer the repair or re-run being attempted. | Carry out the current decision; record a new one if a further attempt is intended. |
| `decision-missing` | No durable decision authorizes the requested repair or re-run, including a grant made before decisions were recorded. | The development manager records it again, with an override where its budget requires one, or escalates. |
| `stoppage-missing` | The action needs a stopped run or a docketed stoppage that its records do not hold. | A re-run of a recorded run that can take it, or an escalation; the refusal names the applicable run and decision. |
| `publication-unmakeable` | The publication cannot describe a merge a re-arm can make, reported as `UnrearmablePublicationError`. | A re-run or an escalation; no later merge request of the same decision can repair the record. |

Classification reads typed refusals and confirmed repository findings, never a
match on the refusal's prose. An unreadable branch or checkout is not proof that
it has gone, and a failed reading keeps the ordinary paced retry. A newly recorded
decision, even of the same kind about the same run, releases the permanent gate
for another attempt; changing the item's notes or waiting longer does not.

A refusal before the action starts spends no repair round, repair grant, or
re-run claim. The stopped run and whatever remains of its change stay as found.
Where the forge is asked after a re-arm has already been recorded as spent, that
spend stands; the carry-out record does not undo the action's accounting.

Other gates keep their existing behavior: a directive, unfinished dependency,
or checkout somebody is using is retried after fifteen minutes; a spending
pause, intake hold, or full harness is attempted once while shut and again on
the first eligible pull after it opens. [Carrying out a decision](work.md#letting-the-harness-choose-the-work)
describes the pass that applies those rules.

This section inventories permanent carry-out refusals. The broader inventory of
bounds that end developer runs is separate work; those bounds are not enumerated
here yet.
