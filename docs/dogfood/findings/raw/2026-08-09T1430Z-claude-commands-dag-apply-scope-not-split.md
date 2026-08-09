---
area: commands
writer: claude
date: 2026-08-09T14:30Z
pr: 89
story: LNGHZN-S5
---

# `arm dag apply` stores scope as one joined string, so every task it creates fails validation immediately

## What the agent-user was trying to do

File three follow-up tasks under `LNGHZN-S5` from a decomposition plan, each with
a multi-file scope, then get straight to work on the first one.

## What happened

The plan schema documents `scope` as a string: *"Comma-separated file paths this
issue is scoped to — stored as a single string, not an array"*. An array is
rejected outright:

```
Error: parse plan file ...: json: cannot unmarshal array into Go struct field
PlanIssue.issues.scope of type string
```

So the comma-separated string is the only accepted form. But `dag apply` then
stores that string **whole**, as a single scope entry, rather than splitting it.
`arm validate` immediately reports it as one giant nonexistent path:

```
INFO: phantom scope: internal/worktree/inventory.go,internal/worktree/inventory_test.go,...,CONTEXT.md
  on LNGHZN-S5-T6 does not match any file
```

Every task the plan created was affected. `arm amend --scope` (whose flag IS a
`strings` slice and does split on commas) had to be run once per task to repair
state that `dag apply` had just written.

## How it changed behavior / confidence / time spent

- Three extra commands to repair three tasks, immediately after creating them.
  This scales linearly with plan size — a twenty-task plan needs twenty repairs.
- The failure is silent at creation time: `dag apply --dry-run`, including
  `--strict`, reported no warning at all. The problem only appeared in a later
  `arm validate`, by which point the ops were already appended and the incorrect
  scope was in the log permanently (I2: history is never rewritten).
- Scope is not cosmetic — it is the delivery gate's write boundary. A task
  carrying one phantom scope entry has, in effect, no usable scope constraint.
- Confidence cost: the schema is explicit and was followed exactly, and the
  result was still wrong. That makes the plan format feel untrustworthy rather
  than merely awkward.

## What would have helped

`dag apply` splitting the documented comma-separated string on commas, the same
way `arm amend --scope` already does. Failing that, `--dry-run` reporting that
the scope it is about to write matches no file, so the mistake is caught before
the ops are appended rather than after.
