---
date: 2026-08-15
agent: 5207ee28
writer: 5207ee28
area: commands
task: LNGHZN-S10-T11
story: LNGHZN-S10
tags: [scope, dag-apply, create, phantom-scope]
---

# Scope normalizer still requires comma-space, so a comma-joined create payload stays one phantom path

## User Goal

File `LNGHZN-S10-T11` with a multi-file scope, then later repair that payload
so `arm validate` and the delivery gate see real paths.

## Observed

This is the leftover of
`2026-08-09T1430Z-claude-commands-dag-apply-scope-not-split.md`.

That finding got a partial fix: `arm dag apply` and
`materialize.normalizeScopeEntries` split a single scope entry only on
`", "` (comma + space). Tests encode `"a.go, b.go"`. A JSON array is still
rejected by the plan schema (`PlanIssue.scope` is `string`).

T11's create op stored one element with **commas and no spaces**:

```text
["cmd/armature/ready.go,cmd/armature/stalereview.go,cmd/armature/dagsum.go,cmd/armature/tui.go,cmd/armature/*_tui.go,.gremlins.yaml,docs/design/gate-efficiency.md"]
```

`normalizeScopeEntries` sees no `", "`, leaves the string whole, and
`arm validate` reports one phantom path. `arm show` / `arm ready` display
the same single entry. The delivery gate would treat that as a path that
matches nothing — in effect no write boundary.

`arm create --scope` and `arm amend --scope` are pflag `StringSlice` and
*do* split on commas. The hole is the stored-string path (create/apply
payload that already contains one joined element, and any materialize of
legacy ops that omitted the space).

No armature issue tracks “plan `scope` should be a JSON array” or “split
on any comma.” Parked here for a later bug-bash epic.

## Impact

A task can sit in `arm ready` looking scoped while having no usable
scope constraint. Repair is `arm amend --scope` (which splits). Discovery
is a later `arm validate` phantom-scope INFO, not a create-time error.

T11 was unsafe to dispatch until amended for this reason (plus missing
acceptance / overlong DoD / uncited).

## Evidence

- Create op `_armature` `d6e0bdc1` (`LNGHZN-S10-T11`).
- `internal/materialize/engine.go` `normalizeScopeEntries`: split only on
  `", "`.
- `internal/decompose/apply.go`: `strings.Split(issue.Scope, ", ")`.
- `internal/decompose/plan.go`: `Scope string`.
- Prior finding:
  `docs/dogfood/findings/raw/2026-08-09T1430Z-claude-commands-dag-apply-scope-not-split.md`.

## Suggested Follow-Up

Bug-bash: (1) split on comma with trim, not comma-space; (2) accept a
JSON array in the plan schema and treat the string form as legacy; (3)
reject a create/amend payload whose single entry contains a comma and
matches no file.
