---
date: 2026-07-16
agent: claude
area: workflow
task: LNGHZN-S1
tags: [migration, bootstrap, backup, ux]
---

# Collapse migration leaves unexplained .arm.collapsed-<timestamp> backup dir

## User Goal

Complete the dual-branch → collapsed layout migration by running `arm bootstrap`
(after the pre-LNGHZN-B1 sources-debris reconciliation fix landed).

## Observed

The migration succeeded, but left a 75MB `.arm.collapsed-20260716123839/`
directory at the repo root with no explanation. The user had to ask: "It's
unclear, is this to be deleted? Or do I need to somehow save these files
somewhere?" Bootstrap printed nothing about what the directory is, that it is a
pre-migration safety snapshot, or when it becomes safe to remove.

## Impact

Post-migration uncertainty and a support round-trip: verifying deletability
required a three-way diff of the backup against the live `.armature/` worktree
plus confirming the reconcile commit landed on `_armature`. Every user who
migrates will hit the same question. The snapshot is by design never needed for
rollback (the `git worktree move` is atomic; see the comment above the backup
creation in `migrateDualBranchToCollapsed`, cmd/armature/bootstrap.go), so in
the success case it is pure leftover.

## Evidence

- `ls -la .arm.collapsed-20260716123839/` → `.armature/`, `.git` (pointer
  file), `.gitignore`, `state/` — 75MB total.
- `diff -rq` of backup inner state dir and `state/` against live `.armature/`:
  zero backup-only files, zero differing files (only "Only in .armature/"
  entries from post-migration activity).
- Backup is snapshotted *after* the sources-debris reconcile commit
  (`3bbfe6de sources: reconcile pre-LNGHZN-B1 uncommitted sources state`), so
  its full contents are committed history on `_armature`.

## Suggested Follow-Up

On successful migration, bootstrap should print the backup path with one line
of guidance, e.g. "safety snapshot of the pre-migration ops worktree; all
contents are committed on _armature — safe to delete once you've verified the
collapsed layout." Alternatively (or additionally) `arm doctor` could detect
`.arm.collapsed-*` dirs and suggest cleanup.
