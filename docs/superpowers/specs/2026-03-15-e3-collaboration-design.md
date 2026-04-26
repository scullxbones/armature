# E3 Collaboration — Design Spec

**Date:** 2026-03-15
**Epic:** E3 — Multi-Worker Collaboration
**Issues:** E3-001, E3-002, E3-003, E3-004
**Status:** Draft

---

## Context

E2 (dual-branch mode) is complete. All E2 issues are merged. The `_armature` orphan branch holds per-worker op logs; the `AppendAndCommit` function commits ops locally to the worktree. There is currently no push, fetch, or remote sync — ops never leave the local machine. E3 adds the remote sync layer, worker visibility, audit tooling, and advisory assignment.

### Key Finding: Remote Sync Gap

A test (`TestConcurrentWorkerPush_SecondPushRejected` in `internal/git/git_test.go`) confirms the expected push behavior: Worker A pushes first (fast-forward, succeeds). Worker B's push is rejected (remote tip has moved). Worker B must `fetch + rebase` before pushing. Because each worker writes to their own `ops/<worker-id>.log`, the rebase is always conflict-free — the MRDT guarantee holds.

---

## E3-001: Multi-Worker Conflict Resolution

### Op Binning

Ops are binned into high-stakes and low-stakes to achieve eager push on state-critical ops while batching informational ops.

| High-stakes (eager push) | Low-stakes (count-batched) |
|---|---|
| `claim` | `note` |
| `transition` (includes `reopen`, which reuses `OpTransition` with `to: "open"`) | `heartbeat` |
| `create` | `source-link` |
| `decision` | `source-fingerprint` |
| `link` | `dag-transition` |
| `assign` | |

**Rationale:** High-stakes ops change what other workers see as ready, blocked, or claimed. This includes `reopen` — it reuses `OpTransition` and clears `ClaimedBy`/`ClaimedAt` in materialization, making the issue immediately claimable by other workers. Stale visibility of high-stakes ops causes wasted work (claim races) or semantic conflicts (scope overlap). Low-stakes ops are informational and their staleness has no downstream coordination impact. `dag-transition` is correctly low-stakes: `engine.go` treats it as a no-op in materialization state.

### Push Strategy: Hybrid A+C

- **Piggybacked (A):** When a high-stakes op fires, any pending low-stakes commits (already written and locally committed, not yet pushed) are included in the same push cycle.
- **Count-based (C):** Low-stakes ops are pushed automatically after accumulating N locally-committed-but-unpushed ops (default N=5, configurable as `lowStakesPushThreshold` in `.armature/config.json`).

### Single-Branch Mode

In single-branch mode, `AppendCommitAndPush` degrades to `AppendAndCommit` — the `FetchAndRebase` and `Push` steps are skipped entirely. The `Pusher` interface is satisfied by a no-op implementation in this mode. This preserves the single-branch guarantee that no push ever touches the working branch history.

### New Interfaces

```go
// internal/ops/push.go

// Pusher handles remote sync for the _armature branch.
// In single-branch mode, both methods are no-ops returning nil.
type Pusher interface {
    FetchAndRebase(branch string) error
    Push(branch string) error
}

// PendingPushTracker counts locally-committed-but-unpushed low-stakes ops.
// Returns true from Increment when the threshold is reached (flush needed).
type PendingPushTracker interface {
    Increment() (shouldFlush bool)
    Reset()
}
```

### New Functions

**`AppendCommitAndPush`** (for high-stakes ops):
1. `AppendOp` to log file
2. `CommitWorktreeOp`
3. If the tracker has pending low-stakes commits (tracked internally by `FilePushTracker`): those commits are already local and will be swept into the upcoming push — no additional write needed
4. `FetchAndRebase(branch)`
5. `Push(branch)` — retry up to 3× with exponential backoff (1s, 2s, 4s) on rejection
6. `tracker.Reset()`

**`AppendAndCommit`** (unchanged — low-stakes ops call this, then `tracker.Increment()`):
- When `Increment()` returns `true` (threshold reached): call `FetchAndRebase` + `Push` and `tracker.Reset()`

**Clarification on "flush":** low-stakes ops are written to the log file and committed locally at the time they are recorded (via the existing `AppendAndCommit`). The tracker count reflects commits that are local but unpushed. "Flushing" means pushing those already-committed changes — not re-writing them.

### Tracker Persistence

Counter stored in `.armature/state/pending-push-count` (a plain integer file, never committed to git). Survives process restarts. Resets to 0 on any successful push.

### Git Client Additions

Two new methods on `*Client` in `internal/git/git.go`:

```go
// Push pushes the local branch to origin.
func (c *Client) Push(branch string) error

// FetchAndRebase fetches origin and rebases the local branch onto origin/<branch>.
func (c *Client) FetchAndRebase(branch string) error
```

### Stale State Impact (Design Basis)

| Scenario | Worst case (no sync) | With eager high-stakes push |
|---|---|---|
| Claim race | Hours of discarded code work | At most one sync cycle (~seconds) of wasted work |
| Scope overlap | Semantic code merge conflict | Advisory warning arrives before significant work |
| Blocker visibility | Idle worker | One count-cycle delay at most |
| Cancellation | PR opened on cancelled task | Worker sees cancel before starting significant work |

---

## E3-002: Worker Presence & Claim Visibility

### New Command: `arm workers [--json]`

Derived entirely from materialized state. No new op types.

**Human-readable output:**
```
=== active ===
  worker-a3f2b1c   T1-001 (claimed 12m ago, TTL 48m remaining, heartbeat 1m ago)
  worker-b7e9d4a   T2-003 (in-progress, heartbeat 3m ago)

=== idle ===
  worker-c1a2b3f   (no active claim, last seen 8m ago)
                   assigned: T4-002, T4-005 (unclaimed)

=== stale ===
  worker-d9f0e2c   T3-007 (claimed 3h ago, TTL expired 2h ago — no heartbeat)
```

### Staleness Rules

Staleness is computed **per-claim**, not per-worker, using the existing `claim.IsClaimStale` function as the single source of truth. This ensures `arm workers` and materialization agree on what is stale.

- A worker is **`active`** if it has at least one claim where `claim.IsClaimStale(...)` returns `false`.
- A worker is **`stale`** if all of its claims satisfy `claim.IsClaimStale(...) == true`.
- A worker is **`idle`** if it has no active claims and has emitted at least one op within a configurable inactivity window (default: `2 × config.DefaultTTL` minutes from the last op timestamp).

**TTL=0 (unlimited claims):** `claim.IsClaimStale` already returns `false` for `ttlMinutes <= 0`. A worker holding a TTL=0 claim is never stale due to TTL expiry. It may become idle only when it holds no claims at all.

**No-claim workers:** for workers with no claim history, the staleness baseline is the timestamp of their last emitted op. If `now > lastOpTimestamp + (2 × config.DefaultTTL minutes)`, the worker is considered idle-and-gone and omitted from output entirely.

**Worker status definitions:**
- `active` — has at least one live (non-stale) claim on an issue
- `idle` — no active claims, last op within the inactivity window
- `stale` — all claims are stale per `IsClaimStale`

**Behavior:** read-only, side-effect free. Stale workers are reported but claims are not released — that is handled by the existing `IsClaimStale` logic in materialization.

**JSON output** (`--json`): array of objects with fields — `assigned_issues` is always present for all worker statuses (active, idle, stale), populated with any issues where `IndexEntry.AssignedWorker` matches the worker ID, regardless of claim state:
```json
{
  "worker_id": "worker-a3f2b1c",
  "status": "active",
  "claimed_issue": "T1-001",
  "ttl_remaining_seconds": 2880,
  "last_heartbeat_epoch": 1742042531,
  "assigned_issues": ["T4-002", "T4-005"]
}
```

---

## E3-003: Audit Log Viewer

### New Command: `arm log [--issue ID] [--worker ID] [--since TIME] [--json]`

Reads raw op log files directly (not materialized state) to provide a complete audit trail including superseded ops (e.g., losing claim race entries).

**Filters** (all optional, combinable):
- `--issue ID` — ops where `target_id` matches
- `--worker ID` — ops from a specific worker's log file
- `--since TIME` — ops with `timestamp ≥ TIME` (Unix epoch or `2006-01-02T15:04:05`)

**Human-readable output:**
```
2026-03-15T14:02:11  claim        T1-001  worker-a3f2b1c  ttl=60m
2026-03-15T14:03:44  heartbeat    T1-001  worker-a3f2b1c
2026-03-15T14:05:01  claim        T1-001  worker-b7e9d4a  ttl=60m  [lost race]
2026-03-15T14:31:22  transition   T1-001  worker-a3f2b1c  done → "implemented X"
```

`[lost race]` is a rendering annotation on claim ops that are not the winner per `ResolveClaim`. Not stored in the op log.

**JSON output** (`--json`): JSONL — one raw op object per line, plus `"_lost_race": true` on losing claim ops.

**Implementation:** `internal/audit/` package — walks `ops/*.log`, merges streams by timestamp, applies filters, formats. Imports `internal/claim` for `ResolveClaim` to compute `[lost race]` annotations (a targeted per-issue claim grouping pass, not a full materialization). No dependency on `internal/materialize`. No new op types.

---

## E3-004: Issue Assignment & Reassignment

### New Commands

```
arm assign --issue ID --worker WORKER-ID
arm unassign --issue ID
```

### New Op Type: `assign`

Payload field: `assigned_to string` (distinct from `Op.WorkerID` which records the actor performing the assignment). Unassign is `assign` with `assigned_to: ""`. Last `assign` op wins (last-write-wins semantics). Reassignment is a new `assign` op — no special op type needed.

`assign` is **high-stakes** — triggers eager push since it affects another worker's ready queue ordering.

**Payload struct addition** in `internal/ops/types.go`:
```go
AssignedTo string `json:"assigned_to,omitempty"`
```

`OpAssign` must also be added to the `ValidOpTypes` map in `types.go` (used for op validation) and to `schema.go` (the forward-compatibility op type registry). Without these additions, `assign` ops will be rejected during validation and invisible to schema readers.

### Effect on `arm ready`

- `arm ready --worker ID` — assigned issues for that worker sort first (before the existing priority tiebreakers: explicit > depth > unblock count > age)
- `arm ready` (no worker) — uses existing sort order unchanged; another worker's assigned issues sort to the bottom
- Assignment is advisory — any worker can still claim any unblocked issue

### Effect on `arm workers`

Idle workers show their assigned-but-unclaimed issues:
```
=== idle ===
  worker-c1a2b3f   assigned: T4-002, T4-005 (unclaimed)
```

### Materialization Changes

`IndexEntry` and `Issue` structs in `internal/materialize/state.go` gain `AssignedWorker string` (empty = unassigned). This is **additive** alongside the existing `IndexEntry.Assignee` field (which maps `ClaimedBy` and is unrelated). `engine.go` adds an `applyAssign` case to the `ApplyOp` switch and maps `AssignedWorker` in `BuildIndex`. The ready queue sort in `internal/ready/compute.go` reads from `AssignedWorker` (not `Assignee`) for assignment-aware ordering and accepts an optional `workerID string` parameter.

**Deployment ordering risk:** `engine.go`'s `ApplyOp` switch has `default: return fmt.Errorf("unknown op type: %s", op.Type)`. Any `assign` op written by an E3-deployed worker will cause `arm materialize` to hard-fail for any worker running a pre-E3 binary. All four E3 issues must ship atomically in a single release — no partial deployment. As a forward-compatibility measure, `OpAssign` should be added to `engine.go`'s default passthrough (alongside other non-state ops like `OpSourceLink`) so that pre-E3 binaries can tolerate the op if encountered.

**`reopen` and `AssignedWorker`:** A `reopen` transition (which clears `ClaimedBy`/`ClaimedAt`) does **not** clear `AssignedWorker` — the assignment persists across reopens. Assignment is an advisory relationship separate from the claim; clearing it on reopen would require an explicit `arm unassign`.

### Constraints

- Any worker can assign or unassign any issue — no ownership gate
- Assignment op is recorded with the assigning worker's ID (`Op.WorkerID`) for audit trail; `Payload.AssignedTo` records the target worker
- Assignment does not block claims — it is purely ordering guidance

---

## Implementation Order

Dependencies flow strictly:

```
E3-001 (push/sync layer)
  └── E3-002 (workers command — needs synced state to be meaningful)
  └── E3-004 (assignment — needs eager push for assign op)

E3-003 (audit log — no push dependency, can be implemented in parallel with E3-001)
```

All four should ship together for coherence.

---

## Files Affected

| File | Change |
|---|---|
| `internal/git/git.go` | Add `Push`, `FetchAndRebase` methods |
| `internal/git/git_test.go` | Tests for push/fetch/rebase (concurrent push test already added) |
| `internal/ops/push.go` | New file: `Pusher` interface, `PendingPushTracker` interface, `AppendCommitAndPush` |
| `internal/ops/tracker.go` | New file: `FilePushTracker` implementation (persists count to `.armature/state/pending-push-count`) |
| `internal/ops/commit.go` | Unchanged |
| `internal/ops/types.go` | Add `OpAssign = "assign"` constant to `ValidOpTypes` map; add `AssignedTo string` to `Payload` struct |
| `internal/ops/schema.go` | Add `assign` to op type list; document `assigned_to` payload field |
| `internal/ready/compute.go` | Accept optional `workerID string` for assignment-aware sort |
| `internal/materialize/state.go` | Add `AssignedWorker string` to `Issue` and `IndexEntry` structs |
| `internal/materialize/engine.go` | Add `applyAssign` case to `ApplyOp` switch; map `AssignedWorker` in `BuildIndex`; add `OpAssign` to the `OpSourceLink`/`OpSourceFingerprint`/`OpDAGTransition` no-op passthrough case for pre-E3 forward compatibility |
| `internal/audit/audit.go` | New file: log reader, filter, formatter; imports `internal/claim` for `ResolveClaim` |
| `internal/claim/claim.go` | Unchanged (reused by audit package) |
| `cmd/trellis/workers.go` | New file: `arm workers` command |
| `cmd/trellis/log.go` | New file: `arm log` command |
| `cmd/trellis/assign.go` | New file: `arm assign` / `arm unassign` commands |
| `cmd/trellis/helpers.go` | Thread `Pusher` and `PendingPushTracker` through `appCtx` |
| `cmd/trellis/main.go` | Register new commands |

---

## Non-Goals

- Automatic stale claim release (materialization already handles TTL expiry)
- Forced assignment (assignment is advisory only)
- Push retry queue persistence (in-process retry with backoff is sufficient for v1)
- Webhook delivery retry (deferred to E4-003)
