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

- Worktree is created at `.worktrees/<issue-id>` (relative to repo root) unless the caller supplies an explicit `--worktree <path>` destination
- Issue branch is checked out inside the worktree
- Worktree path is recorded in the claim ops (recoverable if task is re-claimed)
- Every destination created by `arm claim` is managed by its issue binding, including explicit paths outside `.worktrees/`; an explicit destination inside the repository is excluded from broad Git staging before the claim is recorded.
- **Best-effort isolation mitigation is automatically applied:**
  - If the **main tree** uses a `go.work` file, the newly provisioned worktree is removed from its `use` directives so the main tree's gopls does not walk the worktree and get confused about module boundaries.
  - If the main tree has no `go.work` (the common case — this repo has none), this is a no-op for canonical worktrees because `.worktrees/` is gitignored; explicit in-repository destinations are protected by their claim-specific Git exclude pattern.
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
- **Ghost:** The inverse of an orphan: a live claim whose recorded worktree path has no worktree on disk. Reconciliation reports a missing explicit path as a local ghost only when the path is inside this repository or remains registered by this clone; an arbitrary absolute path replicated from another clone is not local evidence. A terminal issue whose worktree is gone is expected, not a ghost.
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

This removes the binding-selected linked worktree and frees its resources. Canonical `.worktrees/` contents remain gitignored; explicit in-repository destinations are protected from broad staging by claim-installed excludes.

## Two-Tier Gate Model (normative)

There are two gate profiles, with distinct roles in the workflow:

- **Fast gate** — runs during implementation and on every intermediate
  remediation cycle. A green fast gate is sufficient to keep iterating.
  Workers MUST NOT run the full gate on intermediate remediations. After the
  last remediation commit, the new HEAD is the final task head — that run is
  a publish gate, not an intermediate one. The command is `make check-fast`
  **when that target exists** (LNGHZN-S10-T2). Until then, iterate with
  targeted existing checks (`make lint`, `make validate-skills`, `go test` on
  changed packages) — do not invoke a missing `check-fast` target and do not
  substitute `make check`.
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

**Remediation after review (normative).** The first review runs after the
worker has transitioned to `done`. Before any remediating write, the
coordinator reopens and reclaims (`arm reopen` then `arm claim --worktree`,
which reuses the existing worktree). The remediator writes only while the
task is `claimed` or `in-progress`, then commits, runs the full gate at
that HEAD, and transitions to `done` again. Do not remediate on a `done`
or `merged` task — the harness hook treats those bindings as stale and
skips scope enforcement. Then refresh every **stale** review artifact
(head, bundle, activity index, assessment path) and dispatch confirmation
with the **same** remediating findings list as hard scope — a new bundle
alone is not confirmation. See the armature-coordinator skill.

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
