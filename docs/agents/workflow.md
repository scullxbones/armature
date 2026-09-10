# Workflow & Operating Model

Armature coordinates task-driven work; it does not execute or supervise external harnesses.

Agents follow the paved road. That is the one blessed pipeline. Everything else is an `[escape hatch]`. Classification lives in [docs/paved-road.md](../paved-road.md).

## Paved road

```
bootstrap → plan/decompose → wave dispatch → work → review → sync
```

### 1. Bootstrap

Initialize the repo, register this clone as a worker, and confirm the binary.

```bash
arm bootstrap
arm worker-init --check || arm worker-init
arm version
```

### 2. Plan / decompose

Register sources, apply a plan, promote drafts, and add dependency edges.

```bash
arm sources add --url PATH --title "TEXT" --type filesystem
arm sources sync
arm dag context
arm dag apply --plan plan.json --dry-run
arm dag apply --plan plan.json
arm dag transition --issue ROOT-ID
arm link --source A --dep B
```

Cite at apply time with the plan `source` field. Do not mint issues by flags.

### 3. Wave dispatch

Survey the graph, take the ready queue, claim with a worktree, and render the worker spec.

```bash
arm list
arm doctor
arm ready
arm claim --issue TASK-ID --worktree
arm render-context TASK-ID
arm worktree list
arm show TASK-ID
```

Daily dispatch loop:

```
arm ready → arm claim --worktree → arm render-context → (launch worker outside Armature) → arm transition
```

`--worktree` is required. The worktree lands at `.worktrees/<issue-id>` unless the caller supplies an explicit `--worktree <path>`. Issue branch is checked out inside the worktree. The path is recorded in the claim ops.

Every destination created by `arm claim` is managed by its issue binding, including explicit paths outside `.worktrees/`. Explicit destinations inside the repository are rejected unless they are under the canonical `.worktrees/` root, because `.git/info/exclude` is shared by every linked worktree.

Best-effort isolation mitigation is applied at claim:

- If the **main tree** uses a `go.work` file, the newly provisioned worktree is removed from its `use` directives so the main tree's gopls does not walk the worktree and get confused about module boundaries.
- If the main tree has no `go.work` (the common case — this repo has none), this is a no-op for canonical worktrees because `.worktrees/` is gitignored. Arbitrary in-repository custom destinations are rejected; external custom destinations remain supported without modifying the shared exclusion file.
- The mitigation **never creates a `go.work`** — not in the worktree and not in the main tree. A bare `go.work` with no `use` directive would break `go build ./...` inside the worktree.
- It is best-effort and non-fatal: a failure only degrades IDE ergonomics and never fails the claim.

`arm worktree list` classifies worktrees:

- **Bound:** A worktree whose issue binding names a live claim at that claim's recorded path (the healthy steady state).
- **Orphan:** A worktree whose issue binding names a known issue with no live claim; it is real work with no current owner, not an error by itself.
- **Ghost:** The inverse of an orphan: a live claim whose recorded worktree path has no worktree on disk. Reconciliation reports a missing explicit path as a local ghost only when the path is inside this repository or remains registered by this clone; an arbitrary absolute path replicated from another clone is not local evidence. A terminal issue whose worktree is gone is expected, not a ghost.
- **GC Removal Set:** Merged/cancelled issues with an existing worktree (ready for cleanup)

Unclaimed (orphan) worktrees can be re-bound by claiming again with `--worktree`.

### 4. Work

Record progress, keep the graph green, run the configured gate, and mark the issue done.

```bash
arm note --issue TASK-ID --msg "..."
arm decision --issue TASK-ID --topic "X" --choice "Y" --rationale "Z"
arm validate --ci
arm gate run full
arm doctor
arm transition --issue TASK-ID --to done --outcome "..."
```

`arm validate --ci` and `arm doctor` are the task-completion sanity check, separate from the `make check` commit gate — see [quality-gates.md](quality-gates.md).

### 5. Review

```bash
arm review prepare --issue TASK-ID --base BASE-SHA --head HEAD-SHA
arm review record --issue TASK-ID --assessment assessment.json
arm review commits TASK-ID
```

### 6. Sync

Publish ops, then let merged PRs promote issues.

```bash
arm push-ops
arm sync
```

## Invariants

- Ops are append-only JSONL in `.armature/ops/<worker-id>.log`; each worker writes only its own log.
- Materialized state is derived from ops, not source of truth.
- `done` = worker-complete; `merged` = confirmed on main branch.

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
  substitute `make check`. Prefer `arm gate run` when profiles are configured.
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
coordinator reopens and reclaims (`arm reopen` `[escape hatch]` then
`arm claim --worktree`, which reuses the existing worktree). The remediator
writes only while the task is `claimed` or `in-progress`, then commits, runs
the full gate at that HEAD, and transitions to `done` again. Do not remediate
on a `done` or `merged` task — the harness hook treats those bindings as stale
and skips scope enforcement. Then refresh every **stale** review artifact
(head, bundle, activity index, assessment path) and dispatch confirmation
with the **same** remediating findings list as hard scope — a new bundle
alone is not confirmation. See the armature-coordinator skill.

## Escape hatches

These commands stay in the CLI. They are not the paved-road pipeline. Marked `[escape hatch]` to match `--help`.

`arm harness-hook` `[escape hatch]` is a harness-native integration surface (guardrails), not a queue runner.

Worktree garbage collection after merged/cancelled. Prefer `arm sync` teardown when the PR lands.

```bash
# [escape hatch]
arm worktree gc
```

Manual merged promotion. Prefer `arm sync` after the PR lands. This removes the binding-selected linked worktree. Canonical `.worktrees/` contents remain gitignored; arbitrary in-repository custom destinations are rejected, while external custom destinations do not need a shared exclusion entry.

```bash
# [escape hatch]
arm merged --issue TASK-ID
```

Rework after done. The road completes once, then syncs. Used when a reviewer required remediation.

```bash
# [escape hatch]
arm reopen TASK-ID
arm claim --issue TASK-ID --worktree
```

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
