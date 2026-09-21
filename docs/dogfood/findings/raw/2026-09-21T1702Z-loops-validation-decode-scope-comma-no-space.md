---
date: 2026-09-21
agent: loops
area: validation
task: MATENC-S1-T1 DecodeScope worker result
tags: [scope, comma, DecodeScope, dogfood]
---

# DecodeScope preserves comma-without-space as a single entry

## User Goal

Encode legacy `", "` scope splitting as `ops.DecodeScope` (MATENC-S1-T1).

## Observed

Worker kept bit-for-bit parity with `normalizeScopeEntries`: split only on comma-space; `a.go,b.go` remains one entry. Called out as an intentional non-goal for this unit (dogfood hole left open).

## Impact

Callers/authors who omit the space still get a single bogus scope path. Fixing it would be a behavior change, not a comment-kill — needs its own task if desired.

## Evidence

PR #217 worker report: "not a comma-without-space splitter (that dogfood hole stays out of this unit)."

## Suggested Follow-Up

Optional follow-on task: decide whether to normalize bare-comma joins at decode (with fixtures) or document the space requirement in create/amend UX.
