---
date: 2026-09-21
agent: loops
area: workflow
task: MATENC-S1-T1 transition after PJ squash-merge
tags: [delivery-gate, squash, worktree, dogfood]
---

# Delivery gate misses squash on main when claim worktree is stale

## User Goal

After PJ squash-merged PR #217 onto main, transition MATENC-S1-T1 to done.

## Observed

`arm transition MATENC-S1-T1 --to done` failed delivery gate: no conventional commits matching the issue since `ccb54351`. Main HEAD *was* `feat(MATENC-S1-T1): … (#217)`, but the claim recorded `worktree_path` `.worktrees/MATENC-S1-T1` still sitting on pre-merge `task/MATENC-S1-T1` at the old tip.

## Impact

Coordinator cannot close the task without `--skip-delivery-gate` or refreshing the worktree to contain the evidence commit. Cloud-Agent + PJ squash path does not update the claim worktree.

## Evidence

TRANSITION-1 CommitReference failure; worktree `git log` still at #216 tip; main has squash `18e11163`.

## Suggested Follow-Up

Delivery gate: also accept commits on `main`/`master` matching the issue id since claim time when `--pr` is set. Or coordinator skill: after remote squash-land, `git -C worktree fetch && reset --hard origin/main` before transition.
