---
date: 2026-09-27
agent: cursor
area: tooling
task: CLAIMORD wave planning
tags: [df-7, list, json, blocked_by, claim-order]
---

# DF-7 — `arm list --parent` JSON does not include `blocked_by`

## User Goal

List CLAIMORD children with dependency fields needed for wave planning.

## Observed

`blocked_by` field missing/null even when `arm show` lists blockers.

## Impact

Low. Coordinator cannot get dependency fields from the list surface.

## Evidence

- Command: `arm list --parent CLAIMORD --format json`
- Happened: `blocked_by` field missing/null even when `arm show` lists blockers
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

List includes dependency fields the coordinator needs for wave planning.
