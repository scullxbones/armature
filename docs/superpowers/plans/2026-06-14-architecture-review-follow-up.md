# Architecture Review Follow-Up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deepen the three highest-friction seams from the June 14 architecture review: Repository Snapshot, Graph Facts, and Command Runtime.

**Architecture:** Expand `internal/snapshot` into the canonical module for current task truth, finish the pure `internal/dag` Graph Facts seam so callers stop restating graph knowledge, and remove the remaining CLI runtime globals so Cobra commands cross one explicit runtime seam. The result should be one interface for repo state, one interface for DAG facts, and one interface for command execution state.

**Tech Stack:** Go stdlib, Cobra, Bubble Tea, existing internal packages (`snapshot`, `materialize`, `ops`, `dag`, `ready`, `validate`, `context`, `config`)

---

## Source Inputs

- Review artifact: `/tmp/architecture-review-20260614-131817.html`
- Architecture doc: `docs/design/architecture.md`
- Domain glossary: `CONTEXT.md`
- Prior follow-up PRD: `docs/superpowers/specs/2026-06-13-architecture-deepening-follow-up-prd.md`
- Prior plan slice to preserve as prior art: `docs/superpowers/plans/2026-06-13-architecture-deepening-follow-up.md`

## Scope Note

This plan intentionally keeps all three recommendations from the June 14 review in one execution document:

1. Deepen the Repository Snapshot module
2. Make Graph Facts the canonical deep module
3. Collapse CLI runtime globals into one Command Runtime module

It does **not** reopen harness-hook truth recovery. It also does **not** relitigate the already-landed `context_files` lifecycle work.

## File Map

### Repository Snapshot

- Modify: `internal/snapshot/snapshot.go`
- Modify: `internal/snapshot/snapshot_test.go`
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/sync.go`
- Modify: `cmd/armature/claim.go`
- Modify: `cmd/armature/create.go`
- Modify: `cmd/armature/reparent.go`
- Modify: `cmd/armature/materialize.go`
- Modify: `cmd/armature/harness_hook.go`
- Modify: `cmd/armature/helpers.go`

### Graph Facts

- Modify: `internal/dag/dag.go`
- Modify: `internal/dag/dag_test.go`
- Modify: `internal/ready/compute.go`
- Modify: `internal/validate/validate.go`
- Modify: `internal/context/assemble.go`
- Modify: `cmd/armature/render_context.go`
- Modify: `cmd/armature/create.go`
- Modify: `cmd/armature/context_history.go`
- Modify: `cmd/armature/harness_context.go`

### Command Runtime

- Modify: `cmd/armature/main.go`
- Modify: `cmd/armature/helpers.go`
- Modify: `cmd/armature/ready.go`
- Modify: `cmd/armature/show.go`
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/create.go`
- Modify: `cmd/armature/claim.go`
- Modify: `cmd/armature/assign.go`
- Modify: `cmd/armature/main_test.go`

---

## Chunk 1: Repository Snapshot Adoption

### Task 1: Expand `internal/snapshot` until command callers stop rebuilding task truth

**Files:**
- Modify: `internal/snapshot/snapshot.go`
- Modify: `internal/snapshot/snapshot_test.go`
- Modify: `cmd/armature/materialize.go`
- Test: `internal/snapshot/snapshot_test.go`

- [ ] **Step 1: Add failing tests for the snapshot interface**

Cover these behaviors in `internal/snapshot/snapshot_test.go`:
- `Load` returns `State`, `Index`, `Issues`, and warnings together.
- `Load` preserves validated-op warnings for mismatched worker IDs.
- `Load` is the only call a command needs to answer index and issue lookups.
- `Load` keeps empty-repo behavior stable.

Run:

```bash
go test ./internal/snapshot/... -v
```

Expected: one or more new tests fail because the helper methods / fields needed by callers do not exist yet.

- [ ] **Step 2: Deepen the snapshot interface**

In `internal/snapshot/snapshot.go`, keep `Load(...)` as the entry point and add the smallest helpers needed by callers so they do not reopen files manually after load. The interface should support:
- index lookup
- issue lookup
- warning access
- current task truth from one load call

Do **not** move queue policy, graph policy, or Cobra formatting into `internal/snapshot`.

- [ ] **Step 3: Update the `materialize` command to consume the deeper snapshot seam where sensible**

Keep `arm materialize` as the explicit replay command, but remove any duplicated post-replay reload logic it no longer needs once `internal/snapshot` exposes stable current truth.

- [ ] **Step 4: Re-run the focused snapshot tests**

Run:

```bash
go test ./internal/snapshot/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/snapshot/snapshot.go internal/snapshot/snapshot_test.go cmd/armature/materialize.go
git commit -m "feat(snapshot): deepen repository snapshot interface"
```

### Task 2: Migrate remaining manual state-loading commands onto Repository Snapshot

**Files:**
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/sync.go`
- Modify: `cmd/armature/claim.go`
- Modify: `cmd/armature/create.go`
- Modify: `cmd/armature/reparent.go`
- Modify: `cmd/armature/harness_hook.go`
- Modify: `cmd/armature/helpers.go`
- Test: `cmd/armature/main_test.go`

- [ ] **Step 1: Add command-level regression tests**

In `cmd/armature/main_test.go`, add or extend tests that prove these commands agree on current task truth after loading the same repo state:
- `arm show`
- `arm list`
- `arm ready`
- `arm render-context`
- `arm claim` / `arm create` preflight paths that inspect existing state

Run:

```bash
go test ./cmd/armature -run 'Test(Show|List|Ready|RenderContext|Create|Claim)' -v
```

Expected: at least one test fails or requires fixture updates because callers still use mixed loading paths.

- [ ] **Step 2: Replace manual `read ops -> materialize -> reload` choreography**

For each command in scope:
- replace `readAllOpsFromDirWithOffsets(...)` + `materialize.Materialize(...)` + `LoadIndex(...)` / `LoadIssue(...)` sequences
- use one `snapshot.Load(...)` call
- keep command-local output formatting intact

Leave `readAllOpsFromDirWithOffsets(...)` in place only for callers that truly need raw accepted-op slices or checkpoint offsets.

- [ ] **Step 3: Preserve warning behavior**

Where commands currently print warnings from validated op loading, keep stderr behavior the same by reading warnings from the snapshot module rather than from helpers in `cmd/armature/helpers.go`.

- [ ] **Step 4: Re-run targeted command tests**

Run:

```bash
go test ./cmd/armature -run 'Test(Show|List|Ready|RenderContext|Create|Claim)' -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/armature/list.go cmd/armature/sync.go cmd/armature/claim.go cmd/armature/create.go cmd/armature/reparent.go cmd/armature/harness_hook.go cmd/armature/helpers.go cmd/armature/main_test.go
git commit -m "refactor(commands): load current task truth through snapshot"
```

---

## Chunk 2: Graph Facts Completion

### Task 3: Move remaining hierarchy and traversal facts behind `internal/dag`

**Files:**
- Modify: `internal/dag/dag.go`
- Modify: `internal/dag/dag_test.go`
- Modify: `internal/validate/validate.go`
- Modify: `internal/ready/compute.go`
- Test: `internal/dag/dag_test.go`
- Test: `internal/validate/validate_test.go`

- [ ] **Step 1: Add failing graph-facts tests**

In `internal/dag/dag_test.go`, add or extend tests for:
- hierarchy legality with `feature`
- ancestry
- descendants
- depth
- dependency cycle checks over graph facts, not consumer-local helpers

Run:

```bash
go test ./internal/dag ./internal/validate -run 'Test(Graph|Hierarchy|Cycle|Feature)' -v
```

Expected: at least one failure shows a caller still owns graph knowledge that should move into `internal/dag`.

- [ ] **Step 2: Add the missing Graph Facts operations**

In `internal/dag/dag.go`, expose the pure graph facts the callers need:
- hierarchy legality
- graph construction from current materialized truth
- traversal helpers that callers currently rebuild

Keep queue ordering, TTL policy, and human-facing validation strings out of this module.

- [ ] **Step 3: Remove duplicated hierarchy tables and graph builders from consumers**

Update:
- `internal/validate/validate.go`
- `internal/ready/compute.go`

so they consume Graph Facts instead of maintaining separate legality or traversal logic.

- [ ] **Step 4: Re-run focused graph tests**

Run:

```bash
go test ./internal/dag ./internal/validate ./internal/ready -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dag/dag.go internal/dag/dag_test.go internal/validate/validate.go internal/ready/compute.go
git commit -m "refactor(dag): make graph facts canonical"
```

### Task 4: Make working-memory callers consume Graph Facts instead of rebuilding node indexes

**Files:**
- Modify: `internal/context/assemble.go`
- Modify: `cmd/armature/render_context.go`
- Modify: `cmd/armature/context_history.go`
- Modify: `cmd/armature/harness_context.go`
- Test: `internal/context/context_test.go`
- Test: `cmd/armature/main_test.go`

- [ ] **Step 1: Add failing working-memory tests**

Cover these behaviors:
- `render-context` uses the same parent-chain and sibling-outcome truth as Graph Facts
- `context_history` and harness-context paths agree on ancestry traversal
- callers no longer rebuild ad hoc node maps unless Graph Facts requires that exact input

Run:

```bash
go test ./internal/context ./cmd/armature -run 'Test(RenderContext|ContextHistory|HarnessContext)' -v
```

Expected: one or more tests fail or require adjustment because callers still own graph construction details.

- [ ] **Step 2: Replace ad hoc graph assembly in callers**

Update the working-memory callers so they depend on one Graph Facts construction path. If a helper is needed, put it in `internal/dag` rather than copying node-map assembly into every caller.

- [ ] **Step 3: Re-run focused working-memory tests**

Run:

```bash
go test ./internal/context ./cmd/armature -run 'Test(RenderContext|ContextHistory|HarnessContext)' -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/context/assemble.go cmd/armature/render_context.go cmd/armature/context_history.go cmd/armature/harness_context.go
git commit -m "refactor(context): consume canonical graph facts"
```

---

## Chunk 3: Command Runtime Consolidation

### Task 5: Remove the global fallback from the command runtime seam

**Files:**
- Modify: `cmd/armature/main.go`
- Modify: `cmd/armature/helpers.go`
- Modify: `cmd/armature/main_test.go`
- Test: `cmd/armature/main_test.go`

- [ ] **Step 1: Add failing runtime-state tests**

In `cmd/armature/main_test.go`, add or extend tests that prove:
- command execution state comes from Cobra context, not package globals
- missing command context fails explicitly instead of silently falling back
- worker-specific state directory setup remains stable

Run:

```bash
go test ./cmd/armature -run 'Test(State|CurrentCtx|WorkerState|ExecutionState)' -v
```

Expected: failures reveal direct or indirect dependence on `appCtx`, `appPusher`, or `appTracker`.

- [ ] **Step 2: Replace package-global fallback with one explicit runtime interface**

Refactor `cmd/armature/helpers.go` and `cmd/armature/main.go` so:
- `executionState` is the only command-runtime module
- helpers consume explicit runtime state
- `mustState(...)` no longer falls back to package globals

If transitional helpers are needed, keep them private and short-lived.

- [ ] **Step 3: Re-run the focused runtime tests**

Run:

```bash
go test ./cmd/armature -run 'Test(State|CurrentCtx|WorkerState|ExecutionState)' -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/main.go cmd/armature/helpers.go cmd/armature/main_test.go
git commit -m "refactor(runtime): remove global command state fallback"
```

### Task 6: Migrate workflow commands to the explicit Command Runtime seam

**Files:**
- Modify: `cmd/armature/ready.go`
- Modify: `cmd/armature/show.go`
- Modify: `cmd/armature/list.go`
- Modify: `cmd/armature/create.go`
- Modify: `cmd/armature/claim.go`
- Modify: `cmd/armature/assign.go`
- Modify: `cmd/armature/helpers.go`
- Test: `cmd/armature/main_test.go`

- [ ] **Step 1: Add failing workflow-command tests**

Cover:
- commands resolve current context through runtime state only
- high-stakes op append paths still work
- no command requires package-global `appCtx` to succeed inside normal Cobra execution

Run:

```bash
go test ./cmd/armature -run 'Test(Ready|Show|List|Create|Claim|Assign)' -v
```

Expected: at least one failure identifies a command or helper still using package-global runtime state.

- [ ] **Step 2: Move each command onto the explicit runtime seam**

Update the scoped commands so they:
- obtain context through `currentCtx(cmd)` / explicit runtime state only
- pass runtime state explicitly into helper paths that need push or tracker behavior
- stop depending on package-level globals as hidden adapter inputs

- [ ] **Step 3: Re-run focused workflow tests**

Run:

```bash
go test ./cmd/armature -run 'Test(Ready|Show|List|Create|Claim|Assign)' -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/armature/ready.go cmd/armature/show.go cmd/armature/list.go cmd/armature/create.go cmd/armature/claim.go cmd/armature/assign.go cmd/armature/helpers.go cmd/armature/main_test.go
git commit -m "refactor(commands): route workflow commands through runtime seam"
```

---

## Final Verification

- [ ] **Step 1: Run targeted package tests**

```bash
go test ./internal/snapshot ./internal/dag ./internal/ready ./internal/validate ./internal/context ./cmd/armature -v
```

Expected: PASS

- [ ] **Step 2: Run full repository tests**

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 3: Run validation gate**

```bash
go run ./cmd/armature validate --ci
```

Expected: `OK: no issues found`

- [ ] **Step 4: Run doctor**

```bash
go run ./cmd/armature doctor
```

Expected: no structural health regressions

- [ ] **Step 5: Commit final fixups**

```bash
git add docs/superpowers/plans/2026-06-14-architecture-review-follow-up.md
git commit -m "docs(plan): add june 14 architecture review follow-up"
```

## Self-Review

- Spec coverage: all three recommendations from the June 14 review appear as top-level chunks in this plan.
- Placeholder scan: no `TBD`, `TODO`, or “write tests for the above” placeholders remain.
- Type consistency: this plan consistently names the three deepened modules `Repository Snapshot`, `Graph Facts`, and `Command Runtime`.

Plan complete and saved to `docs/superpowers/plans/2026-06-14-architecture-review-follow-up.md`. Two execution options:

1. Subagent-Driven (recommended) - I dispatch a fresh subagent per task, review between tasks, fast iteration
2. Inline Execution - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
