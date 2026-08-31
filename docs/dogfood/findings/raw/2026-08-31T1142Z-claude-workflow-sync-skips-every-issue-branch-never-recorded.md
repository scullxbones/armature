---
date: 2026-08-31
agent: claude
area: workflow
task: Close out LNGHZN-S8-T2 after PR #123 merged to origin/main
tags: [sync, merged, i6, worktree-gc, branch-field, claim, transition, detect-merges]
---

# `arm sync` skips every done issue because `branch` is never recorded

## User Goal

After PR #123 rebase-merged to `origin/main` as `5898fc22`, confirm that
Armature would detect the merge and garbage-collect the managed worktree —
before running any state-changing command.

## Observed

Both dry runs reported nothing to do:

```
$ arm sync --dry-run --into origin/main   →  No merged branches detected.
$ arm sync --dry-run --into main          →  No merged branches detected.
$ arm worktree gc --dry-run               →  would_remove: [], count 0
```

The known rebase-merge blindness explains part of this, and is already filed
twice (see Recurs below). But it is not the operative cause here, and fixing it
would not have changed the output.

`DetectMerges` (`internal/sync/sync.go:24`) skips any issue whose `Branch` is
empty, *before* it calls the merge checker:

```go
if issue.Branch == "" {
    continue
}
```

`LNGHZN-S8-T2` has no branch recorded, so `BranchMergedInto` was never called
and no ancestry comparison ever happened. The issue is not examined at all.

The field is never populated by the commands that create the branch.
`arm claim --worktree` derives the branch name deterministically
(`materialize.DeriveBranchName` → `task/<id>`), creates it, and writes a claim
op carrying `ttl`, `worktree_path` and `claim_token` — but **no `branch`**. The
ops schema confirms the payload shape (`internal/ops/schema.go:17`).
`issue.Branch` is only ever assigned from `op.Payload.Branch`
(`internal/materialize/engine.go:228`), which is populated exclusively by the
*optional* `--branch` flag on `arm transition`.

So the branch name is known at claim time, derived by Armature itself, and then
discarded — recoverable only if an agent remembers to re-supply it by hand at
transition time.

This is not specific to one task. Across the whole repo:

```
done issues: 80
…without branch recorded: 80
```

Only 6 op lines in the entire `_armature` history carry a `branch` field at
all, belonging to 3 issues (`NXTTN-S1B`, `TOPTIER-S1-T3`, `LNGHZN-S10-T5`), each
set by hand on a transition.

## Impact

`arm sync` is a no-op for every done issue in this repository, and always has
been. It exits zero and prints a reassuring `No merged branches detected.`,
which is indistinguishable from "I checked and nothing has landed."

This compounds the previously filed rebase-merge finding rather than duplicating
it. That finding proposes replacing ancestry with "stable merge evidence such as
recorded PR state or patch/content equivalence" — but no merge evidence of any
kind would help, because the issue is discarded at the `Branch == ""` guard
before evidence is consulted. A fix targeting only the merge strategy would
leave `arm sync` still reporting zero.

Downstream, `arm worktree gc` is gated on `isTerminalStatus`
(`internal/worktree/reconcile.go:249` — `merged || cancelled`), so a task stuck
at `done` keeps its worktree forever. `arm worktree list` classifies it as an
`orphan` (claim past TTL), which reads as an anomaly to be investigated rather
than the expected consequence of a lifecycle that cannot advance.

The failure is silent and self-consistent: nothing errors, nothing warns, and
the only symptom is worktrees quietly accumulating.

## Evidence

- Merged PR #123 head on `origin/main`: `5898fc22`
- Local branch tip `task/LNGHZN-S8-T2`: `c0fee05c`;
  `git merge-base --is-ancestor task/LNGHZN-S8-T2 origin/main` → non-zero
  (secondary cause; not reached)
- `arm show LNGHZN-S8-T2 --format json | jq 'keys'` lists no `branch`,
  `worktree_path`, or `pr` key
- Claim op for the task, showing `worktree_path` recorded but no `branch`:
  `["claim","LNGHZN-S8-T2",1787808916,"5207ee28-…~rem-LNGHZN-S8-T2",{"ttl":240,"worktree_path":"/home/brian/development/armature/.worktrees/LNGHZN-S8-T2","claim_token":"a7060885975268ccb810c2be32b31e80"}]`
- All three `transition → done` ops for the task carry `outcome` but no `branch`
- `arm worktree list` → `orphans: ["LNGHZN-S8-T2"]`, `gc_ready: []`
- Counted with `arm list --status done --format json`: 80 done, 80 without branch
- Recurs: [Post-merge hook runs but misses an asynchronous rebase merge](2026-08-28T1247Z-codex-workflow-post-merge-hook-misses-async-rebase.md)
- Recurs: [arm sync is structurally blind to this repo's own squash-merge workflow](2026-08-23T1533Z-claude-workflow-sync-blind-to-squash-merges.md)

## Suggested Follow-Up

Record `branch` where it is already derived, rather than asking an agent to
re-supply it later: have `arm claim --worktree` write the derived branch name
into the claim op payload alongside `worktree_path`. The value is computed by
`DeriveBranchName` at that exact moment, so this costs nothing and removes the
dependency on operator memory.

Separately, `arm sync` should distinguish its three outcomes — "no issue was
eligible to examine", "branches examined, none merged", and "merges detected" —
so that a structural no-op cannot present as a clean check. As written, the
command cannot tell the operator that it looked at nothing.
