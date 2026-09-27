# Theme: Show and list omit coordinator fields

## Summary

After a successful write, `arm show` JSON and `arm list --parent` JSON omit fields the coordinator already has in ops or on `arm show` (worktree path, blockers).

## Evidence

- `../../raw/2026-09-27T1205Z-cursor-tooling-show-json-omits-worktree-path.md` - worktree exists at `.worktrees/CLAIMORD-W11` but JSON `worktree_path` is null/absent.
- `../../raw/2026-09-27T1206Z-cursor-tooling-list-parent-json-omits-blocked-by.md` - `blocked_by` missing/null on list even when `arm show` lists blockers.

## Candidate Follow-Ups

- Project `worktree_path` from the claim op onto `arm show` JSON.
- Include `blocked_by` (and other dependency fields) on `arm list --format json`.
