---
date: 2026-07-03
agent: claude
area: workflow
task: HOOKBIND coordination
tags: [render-context, arm-show, acceptance-criteria, workaround]
---

# arm show exposes acceptance criteria render-context omits

## User Goal

After three consecutive red/yellow reviews caused by workers never seeing
contracted test names (see
2026-07-03T1810Z-claude-workflow-render-context-omits-acceptance-criteria.md),
the coordinator needed a way to give workers the acceptance criteria before
dispatch.

## Observed

`arm show HOOKBIND-T4` prints an `Acceptance:` field listing the exact
contracted test names. `arm render-context --format agent` — the command the
coordinator skill mandates as "the worker's complete task spec, pass it
verbatim" — omits this field entirely. An attempt to extract the contract via
`arm review prepare --base HEAD --head HEAD` failed ("delivery contains no
changed files"), so `arm show` is the only pre-dispatch source.

## Impact

Coordinator now runs `arm show` per task and pastes the acceptance list into
each worker prompt manually. Works, but contradicts the skill's "render-context
is the complete task spec" contract and is easy to forget.

## Evidence

- `arm show HOOKBIND-T4` → `Acceptance: ["TestMergedFailsOnViolations_REQ_HOOKBIND_T4 passes", ...]`
- `arm render-context HOOKBIND-T4 --format agent` → no acceptance layer.

## Suggested Follow-Up

Add acceptance criteria as a render-context layer; until then, update the
armature-coordinator skill to instruct running `arm show` alongside
render-context at dispatch time.
