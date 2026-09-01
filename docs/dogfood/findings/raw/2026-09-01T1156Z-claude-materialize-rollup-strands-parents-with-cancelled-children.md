---
date: 2026-09-01
agent: claude
area: materialize
task: Promote stale done-under-merged tasks across the whole DAG
tags: [rollup, materialize, cancelled, merged, state-machine, defect]
---

# `RunRollup` counts a cancelled child as unmerged, permanently stranding the parent

## User Goal

Understand why 79 issues had accumulated at `done`, and whether promoting their
children would let the parents resolve on their own.

## Observed

`State.RunRollup` (`internal/materialize/engine.go:589`) promotes a story or epic
to `merged` once all of its children are merged. The child test is exact equality:

```go
for _, childID := range issue.Children {
    child, ok := s.Issues[childID]
    if !ok || child.Status != ops.StatusMerged {
        unmergedCount++
    }
}
```

A `cancelled` child is therefore counted as unmerged forever. Since `cancelled` is
terminal, the parent's in-degree can never reach zero and it can never roll up —
no sequence of subsequent operations will fix it.

The same file treats `cancelled` correctly two lines away, skipping *parents* that
are already cancelled (`:600`, `:642`). And `internal/worktree/reconcile.go:250`
already encodes the intended predicate for exactly this question:

```go
return status == ops.StatusMerged || status == ops.StatusCancelled
```

So the codebase agrees elsewhere that cancelled is a terminal-success state for
lifecycle purposes; only the rollup disagrees.

## Impact

This is a second, independent root cause of the stale-`done` backlog, distinct
from the already-filed `branch`-never-recorded defect (see Recurs). Descoping a
single task — an entirely normal planning action — silently and permanently
prevents its parent story from ever completing.

Observed in this repo on two parents, both of which had every other child merged:

| Parent | Children | Blocked by |
|---|---|---|
| `E6-S6` | 21 merged, 2 cancelled | the 2 cancelled |
| `SMTC-S1` | 12 merged, 1 cancelled | the 1 cancelled |

Both had to be promoted by hand, writing a permanent op to assert something the
materializer should have derived. `HKREFACT` (14 merged, 2 cancelled) and
`story-1780487664` remain `done` today for this reason alone and are being left in
place as live acceptance evidence for the fix.

The failure mode is quiet: nothing errors, and the parent simply looks like work
someone forgot to close out. It is indistinguishable at a glance from a genuine
omission, which is how these accumulated unnoticed.

## Evidence

- `internal/materialize/engine.go:607` — `child.Status != ops.StatusMerged`
- `internal/materialize/engine.go:600`, `:642` — cancelled *parents* correctly skipped
- `internal/worktree/reconcile.go:250` — the intended predicate, already written
- Existing tests `TestRunRollup_PromotesStoryWhenAllChildrenMerged`,
  `_DoesNotPromoteWithUnmergedChild`, `_CascadesToEpic`
  (`engine_test.go:1042,1062,1083`) — none covers a cancelled child
- Simulation of the corrected predicate over the current 765-issue DAG flips
  exactly one issue (`story-1780487664`), so the live blast radius is minimal
- After promoting their children, `ORCRUN-S1` and `HKREFACT-T14` rolled up
  automatically; `HKREFACT` and `story-1780487664` did not, isolating the cause
- Recurs: [`arm sync` skips every done issue because `branch` is never recorded](2026-08-31T1142Z-claude-workflow-sync-skips-every-issue-branch-never-recorded.md)

## Suggested Follow-Up

Change the predicate to treat `cancelled` as satisfying rollup, mirroring
`reconcile.go:250`.

One guard is required and is the only real design decision in the fix: a parent
whose children are *all* cancelled must not become `merged`, because nothing
shipped. Require at least one merged child before promoting. Without that guard a
wholly-descoped story silently claims delivery.

Filed as a DAG bug rather than fixed in place, so the change goes through the
normal claim/TDD/per-task-commit path — the same discipline whose absence this
session was cleaning up after.
