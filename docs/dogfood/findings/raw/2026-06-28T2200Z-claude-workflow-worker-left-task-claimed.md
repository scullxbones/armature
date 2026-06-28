---
writer: claude
area: workflow
slug: worker-left-task-claimed-not-done
date: 2026-06-28T22:00Z
---

# Haiku worker completed work but left task in `claimed` state

## What I was trying to do

Coordinating SMTC-S1. After the haiku worker for T2 (SMTC-S1-T2) returned saying "task transitioned to done", I ran `arm merged --issue SMTC-S1-T2` to promote the task and merge the branch.

## What happened

```
$ arm merged --issue SMTC-S1-T2
Error: issue SMTC-S1-T2 is in status "claimed"; arm merged requires status=done
```

The worker's summary said it ran `arm transition SMTC-S1-T2 --to done`, but the actual DAG showed the task still as `claimed`. Had to manually run:

```
$ arm transition SMTC-S1-T2 --to done --outcome "..."
$ arm merged --issue SMTC-S1-T2
```

Note: the first `arm merged` call after the manual transition still failed with the same error message — had to run `arm merged` a second time before it succeeded.

## Why it matters

- The coordinator skill's "Worker Recovery" section covers this case but it's easy to miss that it applies to foreground agents too, not just background.
- The double-failure (worker didn't transition; then `arm merged` failed on first retry) is confusing: it appears as two separate bugs.
- Requires the coordinator to inspect every task after worker return, not just trust the worker summary.

## Evidence

- Worker agent summary: "Task transitioned to `done`"
- `arm list --parent SMTC-S1 --format json` after worker return: `SMTC-S1-T2 → "status": "claimed"`
- Manual `arm transition SMTC-S1-T2 --to done ...` → `{"issue":"SMTC-S1-T2","status":"done"}`
- First `arm merged` immediately after: still "claimed" error
- Second `arm merged`: `Marked SMTC-S1-T2 as merged`

## Potential mitigations

- Coordinator skill should explicitly check `arm list --status in-progress` (or `claimed`) after every worker return, not rely on the worker's self-reported status.
- The recovery section could be elevated earlier in the "After Workers Return" checklist.
- `arm merged` could accept a `--force` that runs the transition itself if the task is `claimed`.
