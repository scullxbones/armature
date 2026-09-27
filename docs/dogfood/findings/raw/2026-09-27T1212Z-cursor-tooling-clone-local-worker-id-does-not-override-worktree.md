---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD-W21 claim identity
tags: [df-13, worker-id, worktree, claim-order]
---

# DF-13 — clone `--local` worker-id does not override worktree-scoped id

## User Goal

Impersonate another worker for lost-race tests: `git config --local armature.worker-id other-worker-abc` then `arm claim`.

## Observed

ResolveIdentity still used worktree-config; lost-race tests claimed as the same worker.

## Impact

High for W2.1. Fixed in tests via `--worker-id`.

## Evidence

- Slice: CLAIMORD-W21
- Command: `git config --local armature.worker-id other-worker-abc` then `arm claim`
- Happened: ResolveIdentity still used worktree-config; lost-race tests claimed as the same worker
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`
- Fixed in CLAIMORD-W21 in tests via `--worker-id`

## Suggested Follow-Up

Flag > worktree config > env > clone config. Tests and coordinators must pass `--worker-id` (or init a new worktree) to impersonate another worker. Fixed in CLAIMORD-W21 tests via `--worker-id`.
