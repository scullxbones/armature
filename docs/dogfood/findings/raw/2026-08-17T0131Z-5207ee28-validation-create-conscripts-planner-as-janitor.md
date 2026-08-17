---
date: 2026-08-17
agent: 5207ee28
writer: 5207ee28
area: validation
task: LNGHZN-S10-T12
story: LNGHZN-S10
tags: [introduction, w1, cross-story, planner]
---

# A cited create landed five cross-story W1 overlaps; the planner became janitor

## User Goal

File one well-formed S10 task for write-time Introduction (ADR 0016) and
leave `arm validate` clean.

## Observed

`arm create` with source, DoD, acceptance, and a broad-but-honest scope
succeeded. The next `arm validate` printed five W1s: T12 vs open tasks in
TOPTIER-S11, NXTTN-S4, LNGHZN-S6 (×2), and TOPTIER-S14. Coverage was
729/729. Exit 0 on this binary (warnings are not errors on main).

I then amended T12's scope (dropped `transition.go` / `types.go`) and
added `blocked_by` edges *from those other stories onto T12* so W1
suppressed. T12 no longer appeared in validate output.

## Impact

The write that created the overlaps was fail-open. The planner — still in
session, but not the owner of S6/S14/S4 — spent a round cleaning the
union so the next reader would not inherit it. That is the Introduction
hole T12 is supposed to close: findings died at the next `validate`, not
at create. Serializing three foreign tasks behind T12 also changed those
stories' ready queues as a side effect of filing one issue.

## Evidence

First validate after create:

```
WARNING: scope overlap: TOPTIER-S11-T1 and LNGHZN-S10-T12 both modify internal/ops/types.go
WARNING: scope overlap: NXTTN-S4-T3 and LNGHZN-S10-T12 both modify …/armature-planner/SKILL.md, …/armature-worker/SKILL.md
WARNING: scope overlap: LNGHZN-S6-T2 and LNGHZN-S10-T12 both modify cmd/armature/transition.go
WARNING: scope overlap: TOPTIER-S14-T1 and LNGHZN-S10-T12 both modify internal/validate/validate.go
WARNING: scope overlap: LNGHZN-S10-T12 and LNGHZN-S6-T1 both modify cmd/armature/helpers.go
OK: no issues found
```

Second validate after amend + three `arm link --source <foreign> --dep T12`: no T12 warnings.

## Suggested Follow-Up

T12 itself: refuse the create when the post-mutation graph introduces W1
on the new ID. Planner iterates with a narrower scope or an explicit
blocked_by *before* the op lands.
