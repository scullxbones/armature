---
area: tooling
writer: 5207ee28
date: 2026-08-15T13:00Z
story: LNGHZN-S10
task: LNGHZN-S10-T3
---

# Agent default cwd is the story checkout, not the task worktree

## What I was trying to do

Remediate PR #106 (`task/LNGHZN-S10-T3`) after PRs 104 and 105 landed on
main. The task checkout already exists at `.worktrees/LNGHZN-S10-T3`. I
wanted latest `check-fast` / skills from main *in that worktree*, and a
one-off `./bin/arm` there — not a merge on the story branch.

## What happened

`git merge origin/main` ran in `/home/brian/development/armature`
(`feat/LNGHZN-S10`) because the conversation workspace is the story
checkout. The merge hit skill conflicts (`armature-worker`,
`armature-reviewer`, `armature-coordinator`, `docs/agents/workflow.md`)
on the story branch before I aborted. The T3 worktree was untouched.

This is the same class as
[`2026-08-08-workflow-worker-commits-to-main-not-worktree.md`](2026-08-08-workflow-worker-commits-to-main-not-worktree.md)
and
[`2026-07-05T2228Z-claude-workflow-worker-edited-main-checkout-instead-of-worktree.md`](2026-07-05T2228Z-claude-workflow-worker-edited-main-checkout-instead-of-worktree.md):
the agent sees the story tree as "the repo" and only discovers the bound
task worktree after an explicit `git worktree list`.

## How it changed behavior, confidence, or time spent

A few minutes and an abort. Low damage because `git status` still showed
the story branch before any conflict resolution. Easy to miss on the next
session if the first git command is not `git -C <task-worktree>`.

## Evidence

- Conversation workspace: `/home/brian/development/armature` on
  `feat/LNGHZN-S10`
- Task worktree: `/home/brian/development/armature/.worktrees/LNGHZN-S10-T3`
  on `task/LNGHZN-S10-T3` at `5c7d71af`
- Abort restored the story branch; T3 remained clean and up to date with
  `origin/task/LNGHZN-S10-T3`
