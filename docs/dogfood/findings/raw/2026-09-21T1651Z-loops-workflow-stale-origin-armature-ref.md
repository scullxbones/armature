---
date: 2026-09-21
agent: loops
area: workflow
task: coordinator claim after CA ops publish
tags: [git, fetch, _armature, dogfood]
---

# git fetch left origin/_armature stale after CA publish

## User Goal

After Cloud Agent pushed `_armature` to `6a0013fe`, sync local ops worktree and claim MATENC-S1-T1.

## Observed

`git fetch origin _armature` updated FETCH_HEAD but `origin/_armature` stayed at `7a7fb2f6` until an explicit `git fetch origin refs/heads/_armature:refs/remotes/origin/_armature`. A claim was briefly committed on the stale parent, then discarded via reset + re-claim on the real tip.

## Impact

Risk of claiming against a lineage missing the story creates; extra recovery steps mid-coordinator.

## Evidence

`ls-remote` showed `6a0013fe` while `git rev-parse origin/_armature` still reported `7a7fb2f6`.

## Suggested Follow-Up

Coordinator skill: after external ops push, `git fetch origin refs/heads/_armature:refs/remotes/origin/_armature` (or `git remote update -p`) before claim. Document in dual-branch notes.
