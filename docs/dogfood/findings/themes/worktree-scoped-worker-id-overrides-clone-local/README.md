# Theme: Worktree-scoped worker-id overrides clone `--local`

## Summary

`armature.worker-id` in worktree config wins over `git config --local`, and `--local --get` misses worktree-scoped ids. Tests and harnesses that impersonate another worker or read the id after `worker-init` get the wrong identity or exit 1.

## Evidence

- `../../raw/2026-09-27T1212Z-cursor-tooling-clone-local-worker-id-does-not-override-worktree.md` - ResolveIdentity still used worktree-config after `git config --local armature.worker-id other-worker-abc`.
- `../../raw/2026-09-27T1215Z-cursor-tooling-git-config-local-misses-worktree-worker-id.md` - `git config --local --get armature.worker-id` exit 1; id lives in `config.worktree`.

## Candidate Follow-Ups

- Flag > worktree config > env > clone config; pass `--worker-id` (or init a new worktree) to impersonate.
- Read `--worktree` then `--local` (or unscoped `--get`).
