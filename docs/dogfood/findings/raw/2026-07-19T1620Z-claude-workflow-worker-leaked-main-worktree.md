---
date: 2026-07-19
agent: claude
area: workflow
task: TOPTIER-S3 coordination
tags: [worktree, isolation, scope-leak]
---

# Worker edits leaked into the main worktree despite isolated task worktree

## User Goal

Merging wave-1 task branches (task/TOPTIER-S3-T1/T3) into feat/TOPTIER-S3 after review.

## Observed

The merge aborted: the main worktree had uncommitted modifications to Makefile and .github/workflows/ci.yml plus an untracked internal/e2eharness/ directory — stale earlier versions of the T1 worker's files. The worker was dispatched into /tmp/claude/arm-task-TOPTIER-S3-T1 but at some point wrote to the main repo as well (harness.go/lifecycle_test.go in main differed from the branch head; harness_test.go and the Makefile/ci.yml deltas were identical).

## Impact

Merge blocked until the coordinator diffed each leaked file against the task branch and discarded them. Repeats the known "worktree changes leaked into the main worktree" theme from the gap analysis (T4); binding enforcement did not prevent cross-worktree writes by a subagent whose session cwd was the main repo.

## Evidence

`git merge` abort output listing Makefile, ci.yml, and internal/e2eharness/* as would-be-overwritten; diffs showed main-worktree copies were stale relative to task/TOPTIER-S3-T1 head 9f7f3135.

## Suggested Follow-Up

Doctor check for a dirty main worktree matching an active task's scope files; harness hook could hard-block writes to the main repo when the resolved binding's worktree is elsewhere.
