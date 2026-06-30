# Theme: Parallel Coordination Conflicts

## Summary

Concurrent operations (such as running checkouts and merges in the same working tree simultaneously) or parallel branching on overlapping files result in race conditions, branch contamination, or silent reverts that git's automatic line-level merge fails to flag.

## Evidence

- [Parallel git checkout race branch contamination](../../raw/2026-06-27T0003Z-coordinator-workflow-parallel-git-checkout-race.md) - Running parallel checkout and merge commands in a single working directory results in checkout race conditions, contaminating branches with unrelated commits.
- [Parallel branch semantic revert](../../raw/2026-06-27T1955Z-5207ee28-coordination-parallel-branch-semantic-revert.md) - Parallel stories branching from the same base and touching the same files can silently revert each other's changes, merging cleanly with no git conflict markers.

## Candidate Follow-Ups

- Enforce sequential merges/checkouts in the coordinator skill, or execute them in separate, isolated git worktrees.
- Configure `arm validate` or `arm ready` to warn when parallel stories scope overlapping files, highlighting potential integration order issues.
- Update the coordinator skill to require running a diff audit (`git diff A..B`) on overlapping files after merging parallel branches, rather than relying solely on git reporting a clean merge.
