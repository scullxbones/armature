---
date: 2026-09-27
agent: cursor
area: workflow
task: CLAIMORD-W20 unlink
tags: [df-3, unlink, introduction, claim-order]
---

# DF-3 — `arm unlink` is Introduction-gated the same as create

## User Goal

Unlink a serial-overlap edge so the new claim-order task can run (`arm unlink --source CLAIMORD-W20 --dep TOPTIER-S6-T3`).

## Observed

UNLINK-1 cannot introduce Graph Finding (scope overlap W1).

## Impact

High. CLAIMORD-W20 remains blocked by `TOPTIER-S6-T3`; CLAIMORD-W21 remains blocked by `bug-1783480206`.

## Evidence

- Command: `arm unlink --source CLAIMORD-W20 --dep TOPTIER-S6-T3`
- Happened: UNLINK-1 cannot introduce Graph Finding (scope overlap W1)
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Unlink of a serial-overlap edge should be allowed so the new task can run; or Introduction should not apply to unlink.
