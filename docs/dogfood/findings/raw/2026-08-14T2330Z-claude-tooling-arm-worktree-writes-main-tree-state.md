---
area: tooling
writer: claude-planner-session
date: 2026-08-14T23:30Z
story: LNGHZN-S10 (planning)
---

# arm in a linked worktree writes materialized state into the main tree

## What the agent-user was trying to do

Run `arm validate` and `arm dag apply --dry-run` from a linked git worktree
(`.claude/worktrees/gate-efficiency-plan`) while the main working tree was in
use by a concurrent coordinator process.

## What happened

Every arm command, including read-only ones and `--dry-run`, attempted to
materialize snapshot state at the **main tree's** path:

```text
Error: load snapshot: materialize: write index: open
/home/brian/development/armature/.armature/state/<writer>/index.json:
read-only file system
```

arm resolves the repository root through the git common dir, so a worktree
session operates on the main tree's `.armature/`. The sandbox blocked the
write, which was correct — the main tree belonged to another process — but it
means arm cannot be used at all from a worktree while the main tree is busy.

## How it changed behavior, confidence, or time spent

Planning had to be staged as files (spec, plan JSON, ADR) with `arm dag apply`
deferred until the main tree is free. Even `--dry-run` validation of the plan
was unavailable, so schema errors will surface later than they should.

## What would have helped

Either materialize snapshot/index cache into the current worktree (it is
derived state, not source truth), an explicit read-only mode that skips index
writes, or a documented way to point materialization at a per-worktree cache
directory. Concurrent coordinator-in-main plus planner-in-worktree is exactly
the multi-process shape managed worktrees (ADR 0013) encourage.
