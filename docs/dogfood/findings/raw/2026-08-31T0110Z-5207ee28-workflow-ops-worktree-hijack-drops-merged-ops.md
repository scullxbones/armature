---
date: 2026-08-31
agent: grok
writer: 5207ee28
area: workflow
task: Close out LNGHZN-S6-T1 through LNGHZN-S8-T1 after PRs #116–#122 merged
tags: [ops-worktree-path, arm-merged, worktree-teardown, i6, github-merge]
---

# A leftover `/tmp` PR-122 ops checkout stole `armature.ops-worktree-path`, so `arm merged` committed onto detached HEAD

## User Goal

Promote the GitHub-merged stack starting at PR #116 from `done` to `merged` and
tear down the leftover worktrees. `arm sync --into origin/main` reported
"No merged branches detected" (same ancestry blind spot as
[sync-blind-to-squash-merges](2026-08-23T1533Z-claude-workflow-sync-blind-to-squash-merges.md)
and [post-merge-hook-misses-async-rebase](2026-08-28T1247Z-codex-workflow-post-merge-hook-misses-async-rebase.md)).

## Observed

`git config armature.ops-worktree-path` was `/tmp/armature-pr122-ops`, a detached
checkout left from PR #122 prfix — not the canonical `.armature` worktree that
has `_armature` checked out.

`arm merged --issue … --pr …` for #116–#122 succeeded and removed the managed
`.worktrees/<id>` checkouts, but the seven transition ops landed as:

```
dd8a5918..9565c865  ops: transition LNGHZN-S6-T1 … LNGHZN-S8-T1
```

on that detached HEAD. Canonical `.armature` stayed at `6ad001c1`
(`ops: transition AOC-S1-T2`). `arm show` reported `merged` because it reads
the configured ops path; `git -C .armature log` and the `.armature` snapshot
still showed `done`.

Removing `/tmp/armature-pr122-ops` without first fast-forwarding `_armature`
would drop the only ref to those merge ops.

## Impact

GitHub merge already fails to auto-promote (`arm sync` / post-merge hook). The
manual recovery path (`arm merged`) then writes to whichever leftover checkout
last overwrote `armature.ops-worktree-path`. Closeout looks successful while
the durable `_armature` branch never moves. A later `git worktree remove` of
the `/tmp` tree, or a session that points config back at `.armature`, reverts
issues to `done` with the worktrees already gone.

## Evidence

- `git config armature.ops-worktree-path` → `/tmp/armature-pr122-ops`
- `git worktree list`: `.armature` on `_armature` at `6ad001c1`; `/tmp/armature-pr122-ops` detached at `9565c865`
- Last seven commits on the tmp checkout are the `--pr 116`–`122` transitions
- `arm sync --into origin/main --dry-run` → `No merged branches detected`

## Suggested Follow-Up

Refuse to treat an unbound `/tmp` checkout as the ops worktree when `.armature`
already has `_armature`. `arm merged` / `arm doctor` should warn when
`armature.ops-worktree-path` is not the live `_armature` worktree. Recurs the
squash-merge detection gap.
