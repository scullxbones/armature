---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD-W11 claim --worktree
tags: [df-6, show, json, worktree_path, claim-order]
---

# DF-6 — `arm show` JSON omitted `worktree_path` after `arm claim --worktree`

## User Goal

Confirm the recorded worktree path after claiming `CLAIMORD-W11` with `--worktree`.

## Observed

`worktree_path` null/absent; worktree exists at `.worktrees/CLAIMORD-W11`.

## Impact

Medium (coordinator has to `git worktree list`).

## Evidence

- Command: `arm claim CLAIMORD-W11 --worktree`; `arm show CLAIMORD-W11 --format json`
- Happened: `worktree_path` null/absent; worktree exists at `.worktrees/CLAIMORD-W11`
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Show the recorded path from the claim op.
