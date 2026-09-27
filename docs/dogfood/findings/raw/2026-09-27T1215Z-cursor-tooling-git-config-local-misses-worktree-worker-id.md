---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD-W21 e2e claim race
tags: [df-16, worker-id, git-config, worktree, claim-order]
---

# DF-16 — `git config --local --get armature.worker-id` misses worktree-scoped ids

## User Goal

Read worker id in e2e `TestClaimRaceAndStaleReclaim` after `arm worker-init`.

## Observed

exit 1; id lives in `config.worktree`.

## Impact

Medium; harness helper updated.

## Evidence

- Slice: CLAIMORD-W21
- Command: e2e `TestClaimRaceAndStaleReclaim` after `arm worker-init`
- Happened: exit 1; id lives in `config.worktree`
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`
- Fixed in CLAIMORD-W21: harness helper updated

## Suggested Follow-Up

Read `--worktree` then `--local` (or unscoped `--get`). Harness helper updated in CLAIMORD-W21.
