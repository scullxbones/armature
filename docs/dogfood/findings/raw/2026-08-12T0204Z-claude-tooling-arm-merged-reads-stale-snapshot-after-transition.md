---
area: tooling
writer: claude
date: 2026-08-12T02:04Z
story: LNGHZN-S5
---

# `arm merged` gates on the stale materialized snapshot that `arm transition` deliberately refuses to trust

## What the agent-user was trying to do

Close out `LNGHZN-S5` after confirming all ten child tasks were merged and PR #89
had merged. The documented two-phase path is `transition --to done`, then
`arm merged --issue` — two commands, run back to back, in that order.

## What happened

The transition succeeded and reported the new status:

```
$ arm transition LNGHZN-S5 --to done --force --outcome "..."
{"issue":"LNGHZN-S5","status":"done"}
```

The very next command disagreed with it:

```
$ arm merged --issue LNGHZN-S5
Error: issue LNGHZN-S5 is in status "in-progress"; arm merged requires
status=done (transition it to done first)
```

`in-progress` was the status *before* the transition. Nothing else ran between
the two commands. An explicit `arm materialize` cleared it:

```
$ arm materialize
Materialized 716 issues from 8329 ops
$ arm show LNGHZN-S5 --format json | jq -r '.status'
done
$ arm merged --issue LNGHZN-S5 --force
Marked LNGHZN-S5 as merged
```

## Why this is sharper than a plain caching bug

The codebase already knows the snapshot is unreliable here — in the *other*
command. `arm transition` reads its own pre-flight state by replaying ops rather
than reading the snapshot, and `cmd/armature/transition.go:261-264` says why in
so many words:

```go
// currentIssueFromOps reads the append-only source of truth without updating
// derived state. Delivery-gate decisions must not rely on a stale snapshot:
// amend, unassign, and transition --to open append authoritative state changes
// but do not synchronously materialize them.
```

`materialize.Run` is then called with `Options{WriteStateFiles: false}` — the ops
are replayed for the decision, and the snapshot on disk is left untouched by
design.

`arm merged` gates on the opposite source. At `cmd/armature/merged.go:314` it
calls `store.ReadIndex()` and rejects on `entry.Status` from that index
(`merged.go:324-326`). So one command in the closeout pair treats the snapshot as
untrustworthy for gate decisions, and the next command in the same pair treats it
as authoritative — and the first command is precisely the thing that invalidates
it.

The transition's own comment names `transition --to open` as a writer that does
not materialize; this observation extends that list to `transition --to done`,
which is the one that always immediately precedes `arm merged`.

## How it changed behavior / confidence / time spent

- The error message is actively misleading. It states a status that is no longer
  true and prescribes the remedy the agent *just successfully performed*
  ("transition it to done first"). The obvious readings are that the transition
  silently failed or that `--force` didn't take — neither is the case. The actual
  remedy, `arm materialize`, appears nowhere in the message.
- The failure is on the happy path. This is not an edge case reached by unusual
  flags: `done` → `merged` is the documented closeout sequence, so any agent
  running it back to back in one session can hit this. It is likely masked in
  normal operation only because some *other* command between the two happens to
  materialize as a side effect — which means the sequence works or fails
  depending on what else the agent ran, the worst kind of intermittent.
- Cost here was small because the contradiction was obvious enough to distrust
  immediately. An agent that believes the error would take the prescribed action
  — re-running `transition --to done` on an issue already in `done`, or escalating
  the `--force` it already used — and append further ops to permanent history (I2)
  chasing a state that was already correct.

## What would have helped

The narrow fix is for `arm merged` to resolve status the way `arm transition`
already does — replay ops via the equivalent of `currentIssueFromOps` rather than
`store.ReadIndex()` — so both halves of the closeout pair read the same source of
truth. `internal/issuetype` aside, this is a two-command inconsistency, not a
subsystem redesign.

The broader question is which commands are entitled to gate on the snapshot at
all. Right now that is decided per command, and the comment at
`transition.go:261` is the only place the hazard is written down. A stated rule —
*gate decisions read ops; display reads the snapshot* — would settle it for every
future command instead of leaving each one to rediscover it. Note this is the
same class as the `LNGHZN-S6` (agent-grade error contract) work: an error whose
`next_actions` said `arm materialize` would have made this self-clearing even
without the underlying fix.

## Related

- `docs/dogfood/findings/raw/2026-08-09T1510Z-claude-validation-delivery-gate-counts-ignored-artifacts.md`
  — also `LNGHZN-S5` closeout, also a deterministic gate rendering a verdict on
  state the worker did not put there and cannot see.
