# yoyodyne-a0s: the program manager pass-and-query run stalled in the session its re-adoption resumed

The work item that made the harness continue a first silent-stream stall itself
(yoyodyne-a0s) asked one question about the second of the two stalls it was
filed on. The stalled run was run-008b0e25, on the item about a program
manager's pass opening with the read model and asking one named query
(yoyodyne-ifd.430.13.8). The run had been stopped for a redeploy and re-adopted
before it stalled. The question was whether the stall began in the developer
session that re-adoption resumed.

**It did.** The session the re-adoption resumed is the one the harness stopped
as stalled, and it had already written its final reply when it was stopped. What
kept its process alive was the background checks it had started.

## What the records say

Times are Pacific daylight time on 2026-09-28. The records keep them in UTC,
seven hours later. The sources are `watch.jsonl`, `runs/run-008b0e2554ccd3869a60c088b6d6f5f6.json`,
and that run's `.events.jsonl`, all under the product's state directory.

| When (PDT) | What |
| --- | --- |
| 11:28:14 | run-008b0e25 starts. Its developer session 5adada29-d461-410b-8cf8-56204549efc0 begins (event 5, `run.started`). |
| 11:42:47 | The watch session watch-4c4e9daf finds a build deployed over it and starts draining, with a 15-minute bound. |
| 11:58:02 | The drain bound runs out with two runs still going. The session stops both and preserves them for the session that comes back: yoyodyne-ifd.428.50 and yoyodyne-ifd.430.13.8. |
| 11:58:12 | The run is re-adopted. The same developer session, 5adada29…, is resumed (event 550, `run.started`). |
| 11:58:25 | The resumed developer says the harness saved its work, and that it is running `make test` and `make race` again. It starts them in the background and sets a watch on them. |
| 11:58:33 | The resumed session writes its final reply for the turn, "Waiting on `make test` and `make race`." (event 571, `run.completed`, `terminal_reason: completed`). |
| 11:58:33 – 12:03:34 | Nothing more is written. The process does not exit, because its background tasks are still running. |
| 12:03:34 | The harness stops the invocation as stalled, five minutes after the last event, and leaves the run in flight. |
| 12:41:27 | The sweep settles the run as one whose process vanished, and dockets it for the development manager. |

The run's record names one developer session for its whole life. The only
`run.started` after the redeploy stop is the resumption of that session, and
every event from there to the stall belongs to it. So the stall began in the
session the re-adoption resumed.

## What that means

This stall was not a provider that went quiet mid-turn. The AI session finished
its turn and said it was waiting. Its process stayed alive only because the
checks it had started in the background kept it running. The harness's liveness
bound counts silence on the stream, so it read a finished turn with a live
process as a stall.

Under yoyodyne-a0s the harness now continues such a run itself, once, in the
same session. That is the right answer for the run: the developer is handed its
session back and can read what its checks said. The run still spends the
harness's one continuation, though, on a turn that had actually ended. Telling
"the stream went silent mid-turn" apart from "the turn ended and the process
lingered" is work for the provider adapters' classification of a stopped
invocation, and is not part of yoyodyne-a0s.

From yoyodyne-a0s on, a run's record keeps the redeploy stop it was re-adopted
from, as `readopted`. The docket entry for a stall after one says so. It says
the stall began in the resumed session only where the run stalled at the phase
it was re-adopted at; otherwise it says only that the run had been re-adopted
before it stalled.
