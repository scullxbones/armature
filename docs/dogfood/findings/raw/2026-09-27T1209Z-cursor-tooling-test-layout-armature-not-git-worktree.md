---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD-W11 claim tests
tags: [df-10, locateops, tests, claim-order]
---

# DF-10 — Test layout `.armature` is not a git worktree

## User Goal

Run claim tests via `setupArmatureLayout` with LocateOps against the test ops dir.

## Observed

LocateOps against `git -C .armature` walked up to the code repo; win check lost the claim until filesystem fallback.

## Impact

Medium. Fixed in CLAIMORD-W11 by non-git fallback (local logs treated as published so existing tests keep working).

## Evidence

- Command: claim tests via `setupArmatureLayout`
- Happened: LocateOps against `git -C .armature` walked up to the code repo; win check lost the claim until filesystem fallback
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`
- Fixed in CLAIMORD-W11 by non-git fallback (local logs treated as published so existing tests keep working)

## Suggested Follow-Up

Tests should use a real ops worktree, or LocateOps must detect missing `.git` (we did the latter in W11). Fixed in CLAIMORD-W11.
