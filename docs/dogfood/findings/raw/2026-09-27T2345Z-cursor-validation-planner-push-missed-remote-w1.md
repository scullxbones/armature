---
date: 2026-09-27
agent: cursor
area: validation
task: pre-push graph validate guard
tags: [push-ops, W1, validate-graph]
---

# Planner publish used a local graph that CI would reject

## User Goal

Stop a planner from publishing a grilled story whose W1 overlaps only appear
after `_armature` is merged with origin (the #284 shape).

## Observed

`arm validate --ci` on a checkout without `origin/_armature` attached, or on a
stale ops worktree, can be green while CI `make validate-graph` (same binary
flag, ops from `origin/_armature`) is red. `arm push-ops` previously pushed
without that contract. High-stakes publish rebased only after a failed first
push, so a fast-forward of a locally dirty graph never revalidated.

## Impact

LNGHZN-S11 landed on origin with 11 W1 overlaps against in-flight CLAIMORD
tasks. Local and CI disagreed. Agents treated local green as shippable.

## Evidence

- PR 284 CI job `validate-graph` on run 36356237702
- `.github/workflows/ci.yml` always attaches `origin/_armature`
- `cmd/armature/push_ops.go` was Push-only before this change

## Suggested Follow-Up

Keep a single `validate.CIOptions` / `arm validate --ci` definition. Every
`_armature` publish should integrate then run it.
