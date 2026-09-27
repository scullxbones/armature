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

Low (correct; easy to miss). Recorded because stacked PRs never land on `main` during this run. Coordinators must `arm merged` after each slice even though GitHub PRs stay stacked/unmerged.

## Evidence

- Command: `arm ready --explain`
- Happened: `CLAIMORD-W12` reason `blocker(s) not merged: CLAIMORD-W11` while W11 is only claimed
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

This is intended I6; coordinators must `arm merged` after each slice even though GitHub PRs stay stacked/unmerged.
