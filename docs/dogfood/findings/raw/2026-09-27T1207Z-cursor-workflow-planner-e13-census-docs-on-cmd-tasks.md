---
date: 2026-09-27
agent: cursor
area: workflow
task: CLAIMORD dag transition
tags: [df-8, e13, planner, census, claim-order]
---

# DF-8 — Planner E13 forces census docs onto every `cmd/**` task

## User Goal

Transition the CLAIMORD story (`arm dag transition --issue CLAIMORD`) without pulling surface-census docs onto cmd slices they do not own.

## Observed

E13 CLAIMORD-W11/W13 touch cmd/** while W21 owned `docs/commands.md` and surface-census.md.

## Impact

Low after amend. Still a planning footgun. Scopes for W11/W13 were amended.

## Evidence

- Command: `arm dag transition --issue CLAIMORD`
- Happened: E13 CLAIMORD-W11/W13 touch cmd/** while W21 owned `docs/commands.md` and surface-census.md
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Documented; we amended W11/W13 scopes. Still a planning footgun.
