# Finding: sandbox environment restrictions caused several near-misses coordinating LNGHZN-S4

**Writer:** claude
**Area:** tooling

## What I was trying to do
Run `arm claim --worktree`, `go build`, `go test`, `make check`, and
`gremlins` mutation testing while coordinating a multi-task story in a
sandboxed background session.

## What happened
- `/tmp` itself is read-only in this sandbox; only `$TMPDIR`
  (`/tmp/claude-1000`) is writable. `arm claim --worktree /tmp/arm-task-X`
  failed with `fatal: could not create leading directories of
  '/tmp/arm-task-X/.git': Read-only file system` until worktree paths were
  moved under `$TMPDIR`.
- The Go build cache (`~/.cache/go-build`) is also outside the sandbox's
  allowed-write set by default, so `go build`/`go test`/`make check` all
  failed with `read-only file system` until run with the sandbox override.
  Every build/test/lint command in this session needed that override —
  fairly high friction for a repo whose own quality gate (`make check`) is
  central to the workflow.
- `arm merged --issue X` (worktree teardown) intermittently failed with
  `git worktree remove: Device or resource busy` /
  `error: failed to delete '.git/worktrees/<name>': Device or resource busy`
  in this WSL-backed sandbox, even after the task was already marked
  `merged`. Manual `rm -rf .git/worktrees/<name>` was only partially
  effective — some files inside the worktree metadata dir remained
  permanently un-removable (benign: `git worktree list` no longer shows the
  worktree, so it doesn't block further coordination, but `.git/worktrees/`
  accumulates orphaned directories over a long session).
- A stray `git stash pop` (run by habit while investigating an unrelated
  failure) surfaced a pre-existing, unrelated stash from a much earlier
  session (`stash@{0}` on branch `pr-63`) and produced merge conflicts
  against current HEAD. Recovered cleanly with `git checkout HEAD -- <files>`
  without dropping the stash, but it was a genuine near-miss — a
  less-careful recovery could have silently discarded someone else's
  leftover work-in-progress.

## How it changed behavior, confidence, or time spent
None of these blocked the story from completing, but each cost several
tool-call round trips to diagnose (they present as generic-looking failures
first) before the sandbox-specific cause was identified.

## Evidence
- `arm claim --worktree /tmp/arm-task-LNGHZN-S4-T1` →
  `create worktree: add worktree: git worktree add: exit status 128 ...
  fatal: could not create leading directories of '/tmp/arm-task-LNGHZN-S4-T1/.git':
  Read-only file system`
- `go build ./...` (no override) →
  `internal/audit/audit.go:10:2: open /home/brian/.cache/go-build/...:
  read-only file system`
- `arm merged --issue LNGHZN-S4-T2 --force` →
  `Error: remove worktree for LNGHZN-S4-T2: git worktree remove
  /tmp/claude-1000/arm-task-LNGHZN-S4-T2: exit status 255\nerror: failed to
  delete '.git/worktrees/arm-task-LNGHZN-S4-T2': Device or resource busy`
