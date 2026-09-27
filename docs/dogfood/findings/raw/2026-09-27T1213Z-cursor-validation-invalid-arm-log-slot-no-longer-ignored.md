---
date: 2026-09-27
agent: cursor
area: validation
task: CLAIMORD-W21 log slot
tags: [df-14, arm-log-slot, claim-order]
---

# DF-14 — invalid `ARM_LOG_SLOT` is no longer ignored

## User Goal

Run `ARM_LOG_SLOT=A arm claim` as tests historically did with uppercase slots.

## Observed

`LOG-SLOT-INVALID` (uppercase fails `^[a-z0-9_-]{1,64}$`).

## Impact

Medium; intended. Tests that used `A`/`B` now use `a`/`b`. Fail loud, never fall back to the unslotted log.

## Evidence

- Slice: CLAIMORD-W21
- Command: `ARM_LOG_SLOT=A arm claim`
- Happened: `LOG-SLOT-INVALID` (uppercase fails `^[a-z0-9_-]{1,64}$`)
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`
- Fixed in CLAIMORD-W21: tests that used `A`/`B` now use `a`/`b`

## Suggested Follow-Up

Fail loud, never fall back to the unslotted log. Tests that used `A`/`B` now use `a`/`b`. Intended; recorded from CLAIMORD-W21.
