---
date: 2026-09-27
agent: cursor
area: validation
task: CLAIMORD-W21 concurrent transition
tags: [df-15, log-slot-collision, aoc, claim-order]
---

# DF-15 — overlapping same-id appends fail `LOG-SLOT-COLLISION` instead of serializing

## User Goal

Run two concurrent `arm transition` on one worker id.

## Observed

Second `TryLock` returns `LOG-SLOT-COLLISION`; AOC append-once still holds because only one writer proceeds.

## Impact

Low; tests updated. Spec §9 wants collision, not blocking `Lock` (which would hang `TestLogSlotCollisionDetected`).

## Evidence

- Slice: CLAIMORD-W21
- Command: two concurrent `arm transition` on one worker id
- Happened: second `TryLock` returns `LOG-SLOT-COLLISION`; AOC append-once still holds because only one writer proceeds
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`
- Fixed in CLAIMORD-W21: tests updated

## Suggested Follow-Up

Spec §9 wants collision, not blocking `Lock` (which would hang `TestLogSlotCollisionDetected`). Tests updated in CLAIMORD-W21.
