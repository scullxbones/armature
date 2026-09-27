---
date: 2026-09-27
agent: cursor
area: documentation
task: CLAIMORD-W11 amend scope
tags: [df-9, amend, scope, cli, claim-order]
---

# DF-9 — `arm amend --scope a b` treats extra tokens as args

## User Goal

Amend CLAIMORD-W11 scope with more than one path: `arm amend CLAIMORD-W11 --scope file1 file2`.

## Observed

USAGE accepts at most 1 arg.

## Impact

Low. Skill/docs show repeated `--scope` flags (StringSlice). Easy to get wrong.

## Evidence

- Command: `arm amend CLAIMORD-W11 --scope file1 file2`
- Happened: USAGE accepts at most 1 arg
- Source: `docs/dogfood/claim-order-findings.md` on `cursor/claimord-w21-58cc`

## Suggested Follow-Up

Skill/docs show repeated `--scope` flags (StringSlice). Easy to get wrong.
