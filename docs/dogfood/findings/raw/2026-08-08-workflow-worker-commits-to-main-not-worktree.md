---
date: 2026-08-08
agent: claude
area: workflow
task: LNGHZN-S5 (coordinator)
tags: [worktree, binding-isolation, worker, git]
---

# Dispatched workers commit into the main repo instead of their claimed worktree

## User Goal

As coordinator for LNGHZN-S5, dispatch haiku workers into isolated worktrees (`arm claim --worktree` at `.worktrees/<id>`) so each task's commits land on its own `task/<id>` branch for task-scoped review and merge.

## Observed

Multiple workers did their edits/commits in the MAIN repo checkout (`/home/brian/development/armature`, on `feat/LNGHZN-S5`) rather than in their assigned worktree:

- **T2 worker** committed the entire task (5 files, commit `7f405ad9`) directly onto `feat/LNGHZN-S5`; its worktree branch `task/LNGHZN-S5-T2` stayed at the base SHA. `arm review prepare --base <base> --head task/LNGHZN-S5-T2` then failed with "delivery contains no changed files".
- **T1 worker** left a *broken partial duplicate* of its work as uncommitted changes in the main tree (a duplicated `WorktreePath` struct field that wouldn't compile), separate from its correct committed work on `task/LNGHZN-S5-T1`.

Only after I added an explicit, forceful "cd into the worktree and verify `git rev-parse --show-toplevel`" instruction (T3) did a worker reliably commit to its own branch.

## Impact

- Broke the task-scoped review/merge model the coordinator skill depends on; I had to detect the misplacement, reconstruct the correct commit range by hand, and discard broken stray edits.
- The binding-isolation invariant (each agent operates under exactly one issue binding via its worktree `.git/armature-issue-id`) is silently defeated when the agent simply runs commands in the wrong cwd. The harness hook's file-path binding resolution can't help if writes happen in the main tree.
- ~30+ minutes of coordinator overhead across the story; two of three code waves needed manual git forensics.

## Evidence

- `git branch --contains 7f405ad9` → `feat/LNGHZN-S5` (not `task/LNGHZN-S5-T2`).
- `git diff <base>..task/LNGHZN-S5-T2` → empty; `arm review prepare` → "delivery contains no changed files".
- T1 stray uncommitted diff contained `+\tWorktreePath string ...` twice (duplicate field).

## Suggested Follow-Up

The worker (armature-worker) skill and coordinator dispatch prompt should make the worktree cwd non-optional and self-verifying: first action is `cd <worktree>` + assert `git rev-parse --show-toplevel` equals the worktree path, and refuse to commit otherwise. Consider an `arm`-side guard that rejects a task transition to `done` when the task's delivery commits are absent from `task/<id>`. Related: [[2026-06-22-tooling-worktree-lsp-workspace-warnings]].
