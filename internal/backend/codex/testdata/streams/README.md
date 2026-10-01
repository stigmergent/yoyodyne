# Recorded Codex streams

What `codex exec --json` wrote to standard output on real invocations, recorded
so the parser in `../../parser.go` is tested against what the CLI actually says
rather than against anybody's account of it. Each file is one invocation's
standard output, one event per line, unedited. None was written by hand.

Each directory is named for the `codex --version` that wrote the streams in it.

## codex-cli-0.159.2

Recorded on 2026-10-01 by the developer run for yoyodyne-ifd.435.7 from
`/Applications/ChatGPT.app/Contents/Resources/codex-cli/bin/codex` (SHA-256
`50ab38ba21d0d9f8346f32f41848382f15b556190f3c7a07e885a4fb73e379c8`, the same
wrapper `../cli-help` was recorded from), which runs the `aarch64-apple-darwin`
binary at `../CodexCLI.app/Contents/MacOS/codex` (SHA-256
`50ac633af64851511f9bbc71032cdae7f1ba20b3234c189687d61ba846c354c5`).

- `no-provider-reached.jsonl` is
  `echo hi | codex exec --sandbox read-only --json --skip-git-repo-check -`, run
  with `CODEX_HOME` set to an empty directory, inside a sandbox whose network
  proxy refused every connection to `api.openai.com` and `chatgpt.com`. **It never
  reached the provider**, so it carries no reply, no usage, and no terminal: the
  CLI was still waiting for the network when the recording was stopped after
  twelve lines. What it does show is genuine — the CLI writes bare events with
  no `msg` envelope, names them `thread.started`, `turn.started`, and
  `item.completed`, carries the session as `thread_id`, and reports each
  reconnect attempt as a top-level `error` event that does not end the turn.
  Nothing in it was secret: the thread id is the throwaway session this
  invocation started, and no credential was loaded.

## What is still missing

No stream here reached the provider, so none shows a reply, token usage, or the
way a turn ends. The run that recorded this one could not record one: the
sandbox a developer run is confined to refuses network to the provider and
refuses writes to the operator's `~/.codex`, where the signed-in CLI keeps its
session state. A recording of a successful turn has to be made outside that
sandbox, for example by the operator:

```sh
mkdir -p "$HOME/codex-recording" && cd "$HOME/codex-recording"
echo "Reply with exactly the word: pong" |
  codex exec --sandbox read-only --json --skip-git-repo-check - > turn-completed.jsonl
codex --version
```

and one that runs a shell command, so the command items are recorded too.
