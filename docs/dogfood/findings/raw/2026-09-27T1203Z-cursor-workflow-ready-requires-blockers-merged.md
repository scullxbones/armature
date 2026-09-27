---
date: 2026-09-27
agent: cursor
area: workflow
task: CLAIMORD-W12 ready
tags: [df-4, i6, ready, merged, claim-order]
---

# DF-4 — Ready queue requires blockers **merged**, not `done` (I6)

## User Goal

See why `CLAIMORD-W12` is not ready (`arm ready --explain`) during a stacked PR run.

## Observed

`CLAIMORD-W12` reason `blocker(s) not merged: CLAIMORD-W11` while W11 is only claimed.

## Impact

Low (correct I6; easy to miss). Stacked PRs in this run never land on `main`, so `arm ready` keeps dependents blocked after a slice is only `done`. `arm merged` is for confirmed-on-main. Recording `merged` while GitHub PRs stay stacked/unmerged would falsify append-only merge state and can unblock dependents early.

## Evidence

- Command: `arm ready --explain`
- Happened: `CLAIMORD-W12` reason `blocker(s) not merged: CLAIMORD-W11` while W11 is only claimed
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Keep `arm merged` for confirmed-on-main only (I6). The friction is that stacked-slice `ready` has no honest signal short of main. Prefer clearer stack-ready semantics, or an explicit non-main "slice accepted" status that does not write `merged`.
