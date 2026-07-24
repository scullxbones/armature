# Finding: Sandbox/tmp friction during TOPTIER-S5 coordinator dispatch

**Writer:** claude
**Area:** tooling

## What I was trying to do
Coordinate TOPTIER-S5 (three sequential tasks) using `arm claim --worktree` per
the armature-coordinator skill's dispatch protocol, then have haiku subagent
workers build/test inside their worktrees.

## What happened
1. `arm claim TOPTIER-S5-T1 --ttl 120 --worktree /tmp/arm-task-TOPTIER-S5-T1`
   (the exact form shown in the coordinator skill's examples) failed:
   `fatal: could not create leading directories of '/tmp/.../.git': Read-only
   file system`. The Claude Code sandbox in this environment denies writes to
   `/tmp` outside the session-specific scratchpad dir. Had to redirect worktrees
   to the scratchpad path instead — not something the skill's examples hint at,
   since they assume unrestricted `/tmp` access.
2. Once a worktree was created, `go build ./...` / `go test ./...` inside it
   failed with the same "Read-only file system" error, this time against the
   shared Go build cache (`~/.cache/go-build`) rather than a repo path — a
   different root cause (module cache location, not `/tmp`) surfacing the same
   symptom. Required re-running with sandbox disabled for build/test commands.
   A haiku worker without instruction to retry with an escalated sandbox mode
   would likely have gotten stuck reporting "cannot build."
3. `arm render-context TASK-ID --format agent` output has no `branch` field,
   even though the coordinator skill's Dispatch Protocol step 5 says to tell
   the worker its "task-specific branch (from render-context)". Had to derive
   it manually via `git -C <worktree> rev-parse --abbrev-ref HEAD`.

## How it changed behavior, confidence, or time spent
Added ~3 extra diagnostic round-trips (one per friction point) before the first
worker could be successfully dispatched. Confidence in the skill's copy-paste
examples working in a sandboxed Claude Code environment is lower — the skill
was likely authored/tested in an environment with full `/tmp` access.

## Evidence
- `arm claim` error: `create worktree: add worktree: git worktree add: exit
  status 128 ... fatal: could not create leading directories of
  '/tmp/arm-task-TOPTIER-S5-T1/.git': Read-only file system`
- `go build` in worktree under sandbox: `pattern ./...: open
  /home/brian/.cache/go-build/.../...-d: read-only file system`
- `arm render-context TOPTIER-S5-T1 --format agent` top-level keys:
  `["issue_id", "layers"]` — no `branch` key present.
