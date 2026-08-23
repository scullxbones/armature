---
date: 2026-08-23
agent: claude
area: workflow
task: Close out LNGHZN-S10 and LNGHZN-S9
tags: [sync, merged, i6, promotion, squash-merge]
---

# arm sync is structurally blind to this repo's own squash-merge workflow

## User Goal

Promote the finished tasks of `LNGHZN-S10` and `LNGHZN-S9` from `done` to `merged` after
their PRs landed on `main`, using the documented `done` → `merged` path.

## Observed

`arm sync --into main --dry-run` reported "No merged branches detected" for both stories,
even though every PR involved (#97, #98, #100, #102–#106, #108–#110, #112) is MERGED on
GitHub and the content is demonstrably present on `main` (T5's four commits are the top of
`main` by message; `--from` is present in `cmd/armature/claim.go`).

Cause: `internal/sync` promotion is ancestry-based (`BranchMergedInto`). The PRs landed via
squash/rebase, which rewrites SHAs, so `task/LNGHZN-S10-T5`, `task/LNGHZN-S9-T1` and
`task/LNGHZN-S9-T2` are not ancestors of `main`. The ancestry check cannot see merged work
that arrived through the merge strategy this project actually uses.

## Impact

The bulk-promotion path is unusable in this repo. Every promotion must go through
`arm merged --issue <ID> --pr <N>` one at a time, and the operator has to already know
that `arm sync` will silently under-report rather than error. Delivered work sits in
`done` looking pending — the exact failure mode `docs/design/next-work-sequencing.md`
item 1 exists to complain about. A coordinator trusting `arm sync` would conclude the
work never landed.

## Evidence

- `arm sync --into main --dry-run` → "No merged branches detected"
- `gh pr list --state all` → #97, #98, #112 et al. all MERGED
- `internal/sync/`, `BranchMergedInto`
- `git merge-base --is-ancestor task/LNGHZN-S10-T5 main` fails despite content parity

## Suggested Follow-Up

Detect squash/rebase merges by content rather than ancestry (patch-id, cherry-mark, or
PR state via `gh`), or have `arm sync` warn explicitly when a `done` issue's branch is
absent from the target's ancestry but its commits appear by message/patch-id — silence
is the harmful part.
