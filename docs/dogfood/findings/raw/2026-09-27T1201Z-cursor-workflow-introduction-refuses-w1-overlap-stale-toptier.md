---
date: 2026-09-27
agent: cursor
area: workflow
task: CLAIMORD-W20 dag apply
tags: [df-2, introduction, scope-overlap, claim-order]
---

# DF-2 — Introduction refuses W1 overlap with stale open TOPTIER README tasks

## User Goal

Apply the claim-order plan (`arm dag apply --plan claimord-plan.json --dry-run`) so W2.0 work can be scheduled.

## Observed

DAG-1 Introduction on `CLAIMORD-W20` vs `TOPTIER-S8-T2` (and other live README.md scopes).

## Impact

High for W2.0 scheduling. Workaround: `blocked_by` the live README tasks so apply succeeds; `arm unlink` is also Introduction-gated (DF-3).

## Evidence

- Command: `arm dag apply --plan claimord-plan.json --dry-run`
- Happened: DAG-1 Introduction on `CLAIMORD-W20` vs `TOPTIER-S8-T2` (and other live README.md scopes)
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Overlapping scope with abandoned/open product-docs tasks should be warning-only for a new story, or `arm ready` should not require those tasks merged.
