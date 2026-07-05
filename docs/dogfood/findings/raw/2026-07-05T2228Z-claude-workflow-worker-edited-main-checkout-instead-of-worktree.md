---
date: 2026-07-05
agent: claude
area: workflow
task: ARCHIMP-S18-T4
tags: [worktree, worker, dispatch, recovery]
---

# Background worker edited the main checkout instead of its assigned worktree

## User Goal

Haiku worker implementing ARCHIMP-S18-T4 in `.worktrees/arm-task-ARCHIMP-S18-T4` (created by `arm claim --worktree`, task branch pre-checked-out).

## Observed

Despite the prompt stating the working directory and branch, the worker made all 20 file edits in the main repository checkout (`feat/ARCHIMP-S18` working tree), ran its gates there, transitioned the task to `done`, and returned — leaving the task branch empty and the story branch dirty with uncommitted work. `arm review prepare` then failed with "delivery contains no changed files".

## Impact

Coordinator had to detect the miss (empty task branch + dirty main tree), export the diff, apply and commit it onto the task branch, and clean the main tree before review could proceed. ~10 minutes of recovery; risk of the uncommitted work being clobbered by any other main-tree operation in the interim.

## Evidence

- `git log 26010006..task/ARCHIMP-S18-T4` → empty; `git -C .worktrees/arm-task-ARCHIMP-S18-T4 status` → clean.
- Main repo `git status` → 20 modified files matching the T4 scope.
- Recovery: `git diff > t4.patch; git -C <worktree> apply && commit; git checkout -- .`

## Suggested Follow-Up

Have `arm transition --to done` (or the worker skill's pre-flight) verify the task branch contains commits beyond the claim base before allowing the transition — this would have caught the miss at the source.
