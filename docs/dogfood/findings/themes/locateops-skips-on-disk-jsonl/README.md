# Theme: LocateOps skips on-disk JSONL

## Summary

Treating the test `.armature` dir or `filepath.Dir(opsDir)` as a git worktree makes LocateOps walk the code repo or ops git history and skip JSONL that exists only on disk.

## Evidence

- `../../raw/2026-09-27T1209Z-cursor-tooling-test-layout-armature-not-git-worktree.md` - `git -C .armature` walked up to the code repo; win check lost the claim until filesystem fallback (CLAIMORD-W11).
- `../../raw/2026-09-27T1211Z-cursor-tooling-locateops-snapshot-load-drops-uncommitted-ops.md` - snapshot Load auto-wired to LocateOps skipped uncommitted test ops (`issue not found`; CLAIMORD-W14/W21).

## Candidate Follow-Ups

- Default snapshot load stays file concat; `Options.OpsWorktree` is the explicit incremental path.
- LocateOps must detect missing `.git` (non-git fallback as in W11).
