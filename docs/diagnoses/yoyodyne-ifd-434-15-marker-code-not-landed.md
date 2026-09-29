# yoyodyne-ifd.434.15: the stale state-root marker item cannot be built until the state-root build lands

The item asks for two changes to the state-root marker: no test may write it
into a checkout's real Git directory, and a marker naming a root that is gone
is reported with a one-command remedy, saying which process wrote it and when,
instead of refusing every command. Both change code that the state-root build
(yoyodyne-j5e) adds. That build had not reached `main` when this run started.

## What the repository says

This run was cut from `62ebd90c` on 2026-09-28. The state-root build's two
commits, `00be8141` and `8c66f6c4`, are on `yoyodyne/yoyodyne-j5e/9d63e7cb`
and are not ancestors of that commit or of `origin/main`. The tracker export
holds yoyodyne-j5e as in progress, with pull request #893 queued for the
forge's merge and not yet merged. So on this base there is no marker: no
`internal/runstate/stateroot.go`, no `AgreeRoot`, no `SplitRootError`, and no
`yoyo doctor` finding about the state root. A fix written here would have
nothing to fix, and one written against the unmerged branch would not apply to
this base.

## What the work needs, once the build is on main

Read from the state-root branch at `8c66f6c4`:

- `AgreeRoot` in `internal/runstate/stateroot.go` creates
  `<git dir>/yoyodyne/state-root` in whatever checkout the command's
  configuration resolves to. A test that runs a command against this
  repository's own configuration, in a worktree, writes into the primary
  checkout's shared `.git`. The item's guard is a test fixture that refuses a
  marker write outside a temporary Git directory.
- The marker holds only the root's path. Saying which process wrote it and
  when needs the marker to carry the writer and the time as well, read back by
  `ReadRootMarker` in a way that still accepts a marker holding only a path.
- `SplitRootError` refuses whenever the recorded root differs from the one
  this process resolved. Where the recorded root does not exist on disk, the
  refusal and `yoyo doctor` should name the remedy as one command. No such
  command exists on that branch. The item names `yoyo state-root rebind`, or
  whatever verb the machine-home design names. The machine-home build
  (yoyodyne-ifd.434.12) is also moving where the marker lives.
- `docs/operations.md` gains a paragraph on the state root in that build. The
  item asks for that paragraph to say what a stale marker does and how to
  clear it.

Until the build lands, checking `.git/yoyodyne/state-root` in the primary
checkout before deploying stays a person's step. When this run looked, on
2026-09-28, the primary checkout's `.git/yoyodyne/` held only `go-build` and no
`state-root`, so the state-root build's checks had left no marker there.
