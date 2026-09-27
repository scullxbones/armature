---
date: 2026-09-27
agent: cursor
area: validation
task: CLAIMORD planning
tags: [df-5, doctor, d1, claim-order]
---

# DF-5 — Doctor D1 warning for unrelated `OPSCLEAN-1` in-progress

## User Goal

Run `arm doctor` before planning claim-order work.

## Observed

D1 Git commits reference issues not in done/merged: OPSCLEAN-1.

## Impact

Low (doctor still exit 0). Planner skill says clean all D1 before planning; cannot transition someone else's in-progress task.

## Evidence

- Command: `arm doctor`
- Happened: D1 Git commits reference issues not in done/merged: OPSCLEAN-1
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Planner skill says clean all D1 before planning; cannot transition someone else's in-progress task.
