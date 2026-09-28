# yoyodyne-ifd.430.20: replies cut in the conversation record

The finding, as raised: on 2026-09-26 the operator's assistant was copying the
architect's pending rulings out of her conversation's event log and found two of
them cut off. Her ruling on yoyodyne-ifd.375 stops at "the release's notes …"
and her ruling on 432.7 stops at "Read anything the shared to…". Both stop
before any landing text. Until yoyodyne-ifd.100 lets a role write the document
it owns, that log is where a ruling lives. Eight other rulings were copied from
the same log in pull request 809, and these two were not. The item asked which
of three things did the cutting: the event log's bound on a reply, the provider
stream's bound on a line, or the rebuild made when a session is compacted.

## The cause: the event log's bound on a reply

It was the event log's bound. Each backend parser recorded a reply as an
`agent.message` event, and cut the text with `execution.TruncateEventText` to
`MaxEventTextBytes`, 16 KiB, adding `…[truncated]` at the end. The same bound
covers command output and provider results. It was also applied to what the
conversation's record keeps of a reply. The operator's own view was never cut:
the parser passes the reply whole to the screen and to the turn's result. Only
the durable copy was cut.

The two rulings are events 1782 and 1857 in conversation
`chat-11558d325e9a214ebfd00bb4a0012750`, the architect's. They were recorded
2026-09-24 at 21:53 and 21:56 PDT. Each holds exactly 16,384 bytes of text
followed by `…[truncated]`, and the text ends as quoted above.

The other two suspects are ruled out by what the log holds:

- **The provider stream's line bound** is 1 MiB, in `execution/process.go`. A
  line cut there reaches the parser as a line it cannot decode, so no
  `agent.message` is written at all. What the log records instead is a
  `process.output` event with the anomaly `truncated_stream_line`. The cut
  rulings are `agent.message` events that decoded, and no such anomaly is
  recorded beside them.
- **The rebuild** reads the event log and writes nothing into it. A rebuilt
  session is handed whatever the log already holds, so it inherits a cut but
  never causes one. It does add a second loss on top: the role's rebuilt
  history carries the cut text as though it were the whole reply.

Nobody was told about either cut. The role that wrote the ruling never learned
the record had lost its end. Nothing in the transcript or in `--json` said so.
The only mark was the marker, at the end of a 16 KiB line, in a file nobody
reads unless they are copying from it.

## How many replies were cut

`yoyo agent cut-replies` reads every conversation log the product holds, and
counts every reply that ends in a cut marker. That includes logs whose
conversation was replaced and that no record points at any more. Run against
this product's state on 2026-09-27:

```text
13 of 2277 recorded replies across 9 conversation log(s) for yoyodyne are held cut, in 3 conversation(s).
```

| Conversation | Agent | Cut replies | Events |
|---|---|---|---|
| `chat-11558d325e9a214ebfd00bb4a0012750` | architect | 9 | 487, 637, 865, 1654, 1782, 1857, 2116, 2337, 2417 |
| `chat-3f488b20b95402f3f61e8e27fedb6608` | development manager | 2 | 895, 911 |
| `chat-91253e0e070c17b0663651cc48602122` | product manager | 2 | 425, 13284 |

Every one is exactly 16,384 bytes long. None of them records the reply's full
size, because the old bound never wrote it down. The architect has been asked to
restate 375 and 432.7. The other seven cuts in her log, and the four in the two
managers' logs, were not known to anybody until this count. Whether any of them
held a decision is for their owners to read.

## What changed

- **A reply has a bound of its own: 128 KiB** (`execution.MaxReplyTextBytes`).
  It applies to both sides of a conversation, `agent.message` and
  `operator.message`, and both backend parsers use it. 128 KiB is the most that
  can be held whole under the 1 MiB line bound the event log is read back
  under. JSON can spend six bytes encoding one byte of text (`<` is written
  `<`), and 6 × 128 KiB still fits in 1 MiB. Every cut reply above would
  have been recorded whole.
- **A reply past the bound is cut where the record says so.** The text is kept
  up to the bound, on a character boundary, and ends with `…[cut here: the
  record holds the first N of this reply's M bytes]`. The event also carries
  `cut`, `recorded_bytes`, and `whole_bytes`. A reader of the text alone,
  including a rebuilt session, sees the cut where it falls.
- **The operator is told when a reply is cut.** The reply is still shown whole.
  After it, the transcript prints a `[record]` line naming the event, the
  reply's full size, and where the record stops. `--json` carries the same thing
  as `record_cuts`, and the Slack sink writes the line to its log.
- **The role is told at the start of its next turn.** The cut is saved on the
  conversation's record as `reply_cuts`, so the notice reaches the next turn
  whichever process takes it. That turn's prompt opens with the cut and asks the
  role to restate what came after it. The notice is cleared once a turn that
  carried it succeeds, unless that turn's reply was cut too.
- **`yoyo agent cut-replies`** is the audit above, with `--json`.

Tests: `internal/chat/replycut_test.go` records a reply past the bound through a
conversation. It checks the declaration in the log and on the reply, that the
cut survives into a fresh process, that the next turn opens with the notice, and
that the notice is given once. `internal/execution/reply_test.go` covers the bound
itself and old-style cuts. `internal/cli/cutreplies_test.go` covers the audit.
