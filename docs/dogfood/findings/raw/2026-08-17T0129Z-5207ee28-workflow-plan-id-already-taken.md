---
date: 2026-08-17
agent: 5207ee28
writer: 5207ee28
area: workflow
task: LNGHZN-S10-T12
story: LNGHZN-S10
tags: [planner, issue-id, plan-json, handoff]
---

# Handoff and plan JSON reused T6; the live DAG already had a merged T6

## User Goal

Turn the ratified three-door grill into one Armature task under LNGHZN-S10,
as the T4 handoff specified (`LNGHZN-S10-T6`).

## Observed

`arm show LNGHZN-S10-T6` is a merged overlap-matcher task from 2026-08-15.
`docs/design/plan-gate-efficiency.json` still ended at T5 and did not list
the later dogfood tasks (T6–T11). The live story list had T1–T11 already.
Creating `LNGHZN-S10-T6` again would have collided with a merged ID that
means something else.

Filed as `LNGHZN-S10-T12` instead.

## Impact

An extra discovery pass (`arm show` / `arm list --parent`) before create.
A planner who trusted the handoff or the on-disk plan file would have
attached the Introduction work to the wrong node or skipped apply because
the ID already existed. The design doc and the DAG had drifted; the DAG
was the source of truth, the plan file was not.

## Evidence

- Handoff: `.worktrees/LNGHZN-S10-T4/docs/design/handoff-lnghzn-s10-t6.md`
- `arm show LNGHZN-S10-T6` → merged, "Scope overlap must match on files/globs"
- `arm show LNGHZN-S10-T12` → the new Introduction task
- Plan file on main listed only T1–T5 until this session amended it

## Suggested Follow-Up

Next-ID allocation should consult the live DAG, not the last row of a
design-plan JSON. A `dag apply` of an existing ID is a skip-if-exists
today — silent and wrong when the ID is recycled with new meaning.
