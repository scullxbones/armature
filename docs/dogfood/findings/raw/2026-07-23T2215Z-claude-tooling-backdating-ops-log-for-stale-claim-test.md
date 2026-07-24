---
date: 2026-07-23
agent: claude
area: tooling
task: LNGHZN-S3 coordination
tags: [materialize, testing, ops-log, staleness]
---

# Simulating a stale claim in a test requires backdating the whole ops log, not one op — undocumented

## User Goal

Writing `TestShouldHeartbeatSkipsAlreadyStaleClaim_REQ_LNGHZN_S3_T1`, an
end-to-end test proving the harness hook doesn't emit a heartbeat for a claim
whose TTL has already expired.

## Observed

The natural approach — claim a task normally, then append one additional
`OpClaim` with a backdated `Timestamp` (e.g. now minus an hour) to force
`ClaimedAt`/`LastHeartbeat` into the past — failed with `materialize: claim:
issue task-01 not found`. The cause: `internal/materialize/pipeline.go` sorts
*all* ops globally by `Timestamp` before replay
(`sortOpsByTimestamp`), so a backdated claim op sorted *before* the task's
`create` op (which has a realistic near-`now()` timestamp), and materialize
tried to apply a claim to an issue that, in timestamp order, didn't exist yet.
The fix required reading back every existing op in the worker's log, deleting
the log file, and re-appending every op with its `Timestamp` shifted by the
same delta — preserving relative order across the whole history rather than
just the one op under test.

## Impact

Cost one full debug cycle (writing a throwaway debug test, adding print
statements, confirming the sort-order hypothesis by reading
`internal/materialize/pipeline.go` directly) to find the real cause, purely
because backdating "the interesting op" seemed like the obvious approach and
nothing in the docs or existing test helpers warned that ops are replayed in
global timestamp order rather than per-file append order.

## Evidence

- `internal/materialize/pipeline.go:159` / `:230`: `sortOpsByTimestamp(allOps)`
  / `sortOpsByTimestamp(filteredOps)` — global sort by `Timestamp` before
  replay.
- First attempt's error: `materialize: claim: issue task-01 not found`,
  reproduced in isolation via a throwaway debug test directly constructing a
  `snapshot.Store` over the same ops directory.
- Working fix (`backdateAllOps` test helper): read all ops via `ops.ReadLog`,
  `os.Remove` the log file, then `ops.AppendOp` each one back with
  `op.Timestamp -= int64(delta.Seconds())`, preserving order.

## Suggested Follow-Up

- Add a shared test helper (e.g. in a `testutil` package or as an exported
  helper in `internal/materialize`) for "backdate this issue's entire ops
  history by N", since any future test needing a stale-claim or TTL-expiry
  scenario end-to-end will hit the same trap.
- Document the global-timestamp-sort replay behavior somewhere test-writer-
  facing (docs/conventions.md or a comment near `sortOpsByTimestamp`
  itself already explains *why* it sorts, but doesn't warn that naive
  single-op backdating in tests will break replay ordering).
