# Workflow & Operating Model

Armature coordinates task-driven work; it does not execute or supervise external harnesses.

Normal loop:
```
arm ready → arm claim → arm render-context → (launch worker outside Armature) → arm transition
```

`arm harness-hook` is a harness-native integration surface (guardrails), not a queue runner.

## Invariants

- Ops are append-only JSONL in `.armature/ops/<worker-id>.log`; each worker writes only its own log.
- Materialized state is derived from ops, not source of truth.
- `done` = worker-complete; `merged` = confirmed on main branch.

## Managed Worktree Lifecycle

Armature automatically manages worktrees tied to tasks, bugs, features, and stories. Worktrees are created, reconciled, and cleaned up as part of the normal workflow:

### Provisioning (at claim)

When you claim a task with the `--worktree` flag, Armature auto-provisions a worktree:

```bash
arm claim --issue TASK-ID --worktree
```

- Worktree is created at `.worktrees/<issue-id>` (relative to repo root)
- Issue branch is checked out inside the worktree
- Worktree path is recorded in the claim ops (recoverable if task is re-claimed)
- **Best-effort isolation mitigation is automatically applied:**
  - If the **main tree** uses a `go.work` file, the newly provisioned worktree is removed from its `use` directives so the main tree's gopls does not walk the worktree and get confused about module boundaries.
  - If the main tree has no `go.work` (the common case — this repo has none), this is a no-op: the worktree is already isolated because `.worktrees/` is gitignored.
  - The mitigation **never creates a `go.work`** — not in the worktree and not in the main tree. A bare `go.work` with no `use` directive would break `go build ./...` inside the worktree.
  - It is best-effort and non-fatal: a failure only degrades IDE ergonomics and never fails the claim.

### Inspection & Reconciliation

Use `arm worktree list` to inspect all worktrees and their claim status:

```bash
arm worktree list
```

This classifies worktrees into categories:
- **Bound:** A worktree whose issue binding names a live claim at that claim's recorded path (the healthy steady state).
- **Orphan:** A worktree whose issue binding names a known issue with no live claim; it is real work with no current owner, not an error by itself.
- **Ghost:** The inverse of an orphan: a live claim whose recorded worktree path has no worktree on disk. A terminal issue whose worktree is gone is expected, not a ghost.
- **GC Removal Set:** Merged/cancelled issues with an existing worktree (ready for cleanup)

### Reclamation & Garbage Collection

Unclaimed (orphan) worktrees can be reclaimed or cleaned up:

```bash
arm worktree gc
```

This removes worktrees for issues in merged or cancelled status. Orphaned worktrees (with no active claim) can be re-bound by re-claiming the task:

```bash
arm claim --issue TASK-ID --worktree
```

### Teardown (at merged)

When a task is confirmed merged on main, its worktree is automatically cleaned up:

```bash
arm merged --issue TASK-ID
```

This removes the linked worktree and frees its resources. The `.worktrees/` directory remains gitignored, so worktree contents are never committed.

## Two-Tier Gate Model (normative)

There are two gate profiles, with distinct roles in the workflow:

- **Fast gate** (`make check-fast`) — deterministic, diff-routed. Runs during
  implementation and on every intermediate remediation cycle. A green fast
  gate is sufficient to keep iterating. Workers MUST NOT run the full gate on
  intermediate remediations. After the last remediation commit, the new HEAD
  is the final task head — that run is a publish gate, not an intermediate one.
- **Full/publish gate** (`make check`) — unchanged in content from before this
  model; mandatory at the task's clean delivery HEAD (the worker's
  responsibility: commit first, then run the full gate before `done`, see the
  armature-worker skill) and once cumulatively at story integration (the
  coordinator's wave verification gate, see the armature-coordinator skill).

Only a green full gate confers delivery — a green fast gate never substitutes
for it. This preserves Constitution I5 (deterministic gates decide): the full
gate is still the thing that decides, the fast gate only shortens iteration.
See `docs/design/gate-efficiency.md` (D1) for the full rationale and evidence
op acceptance rule (D4).

## Before closing out work

```bash
arm validate --ci
arm doctor
```

This is a task-completion sanity check, separate from the `make check` commit gate — see [quality-gates.md](quality-gates.md).

## Recovery & Claim State Management

When tasks fail to complete, become orphaned, or blockers are left unresolved, the system requires deliberate recovery actions. Coordination failures (stale claims, expired TTLs, skipped redispatches) can leave tasks silently stuck and block downstream work.

See [Recovery State Machine](../design/recovery-state-machine.md) for:
- A complete matrix of issue status × claim liveness combinations
- Correct reconciliation actions for `arm doctor` and the Coordinator for each state
- Recovery procedures for three key failure scenarios:
  - **D1 — Branch Divergence:** Commits reference issues not yet done/merged
  - **D2 — Orphaned Claims:** Tasks with expired TTLs and no worker activity
  - **Redispatch Starvation:** Coordinator fails to re-dispatch when claims expire

The state machine defines which state combinations are valid, which are errors, and what action each should trigger.
