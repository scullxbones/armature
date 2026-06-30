# Armature Core Concepts — System Model for Agents

This document explains eight operational concepts that define how Armature coordinates agents and workers. Each concept is presented as a rule or pattern agents should understand when interacting with the system.

---

## 1. Ops Log and Materialization

**Concept:** All state changes are append-only JSONL operations written to per-worker log files. These ops are never edited, only appended. State files (the "materialized" view) are derived by replaying ops and are never the source of truth.

**Pattern:** Ops are append-only. State is materialized. Never edit `.armature/ops/*.log` files directly.

**How it works:**
- Every agent action (claim, transition, note, etc.) appends a single JSON operation to `.armature/ops/<worker-id>.log`
- Each worker writes only to its own log file — this is the **MRDT invariant** that makes merge conflicts architecturally impossible
- State files (`.armature/state/index.json`, `.armature/state/issues/<id>.json`) are computed locally by replaying ops from the checkpoint
- The replay is incremental: only new ops since `checkpoint.json` are replayed, making materialization O(new ops)

**Command examples:**
```bash
# View the append-only ops log for the current worker
cat .armature/ops/$(arm worker-id).log | jq '.'

# Materialize state from ops (replayed automatically on most commands)
arm list --group   # triggers materialization

# Check the checkpoint (last ops position replayed)
cat .armature/state/checkpoint.json
```

---

## 2. Worker Identity

**Concept:** Each worker (human, AI agent, or automation) has a unique identity registered once per clone. This identity is used to break claim races and sign all operations.

**Pattern:** Worker identity is stable within a clone and used to sign all ops. Multi-worker coordination relies on the worker ID to detect conflicts.

**How it works:**
- When you run `arm worker-init` (once per clone), your git config is scanned for `user.name` and `user.email`
- These are hashed to create a unique, deterministic worker ID
- All ops written by that worker carry the worker ID in the `worker_id` field
- Claim races are resolved by timestamp; ties are broken lexicographically by worker ID
- The worker ID is stable — running `arm worker-init --check` in the same clone always yields the same ID

**Command examples:**
```bash
# Register yourself as a worker (run once per clone)
arm worker-init

# Check your worker ID
arm worker-init --check

# Verify worker registration
arm doctor   # includes worker ID verification
```

---

## 3. Claim Lifecycle and TTL

**Concept:** A task must be claimed before work begins. Claims are temporary — they expire after a configurable TTL (time-to-live) unless renewed with heartbeats. Expired claims release the task back to the ready queue.

**Pattern:** Claim → heartbeat → transition. If you go silent longer than the TTL, your claim expires and another worker can claim the task.

**How it works:**
- `arm claim <task-id> --ttl 60` acquires a task for 60 minutes
- A claim consists of `claimed_by` (worker ID), `claimed_at` (timestamp), and `claim_ttl` (minutes)
- The claim is stale if: `now > claimed_at + (claim_ttl * 60) AND now > last_heartbeat + (claim_ttl * 60)`
- Heartbeats extend the deadline: `arm heartbeat <task-id>` updates `last_heartbeat` to now
- When the claim expires, the task reverts to `open` status and becomes available to claim again
- The default TTL is set in `.armature/config.json` (`default_ttl`); claims can override with `--ttl`

**Stale claim detection:**
- `arm list` marks expired claims with a staleness indicator
- `arm doctor` checks for stale claims and warns about orphaned tasks

**Command examples:**
```bash
# Claim a task for 90 minutes
arm claim TASK-001 --ttl 90

# Send a heartbeat (extends the deadline by claim_ttl minutes)
arm heartbeat TASK-001

# View stale claims
arm list   # look for [stale] markers

# Check claim status
arm show TASK-001 | jq '.claimed_by, .claimed_at, .last_heartbeat'
```

---

## 4. DAG Hierarchy and Parentage Rules

**Concept:** Work is organized as a directed acyclic graph (DAG) with three levels: epics (largest scope), stories (mid-level), and tasks (smallest, assignable unit). Parent-child relationships define the decomposition hierarchy. Dependency relationships (blocks/blocked-by) link tasks across branches.

**Pattern:** Epics contain stories; stories contain tasks. Dependencies are acyclic. Ready status propagates bottom-up: a task is ready if all its blockers are done and its parent is not blocked.

**How it works:**
- **Hierarchy:** Epic → Story → Task (one parent, many children)
- **Parent rule:** A story must have an epic parent; a task must have a story parent; epics have no parent
- **Type constraint:** A task cannot be the parent of a story; a story cannot be the parent of an epic
- **Dependencies:** A task can be blocked-by another task (different parent allowed) via link operations
- **Acyclicity:** The system prevents cycles — you cannot create a link that would close a cycle
- **Ready computation:** A task is ready if (1) all its blockers are merged or done, (2) its parent story is not blocked, and (3) no active claim has expired on it

**Command examples:**
```bash
# Create a parent-child hierarchy
arm create --type epic --title "Build Auth System"
arm create --type story --title "Implement Login" --parent EPIC-001
arm create --type task --title "Write login middleware" --parent STORY-001

# Add a dependency (task A blocks task B)
arm link TASK-001 --dep TASK-002 --rel blocks

# View the DAG interactively
arm dag-summary

# Detect cycles (none should exist)
arm doctor   # checks for cycles
```

---

## 5. Citations and Validation

**Concept:** Every task can be traced back to its originating source document (PRD, architecture guide, etc.). Source citations record which sections of which documents justify the task's existence. Validation rules ensure citations are complete and accurate.

**Pattern:** Register sources. Create tasks with citations. Accept citations with rationale. Validate coverage.

**How it works:**
- **Source registration:** `arm sources add --url docs/prd.md --type filesystem` registers a source document
- **Fingerprinting:** `arm sources sync` fetches the source and computes a fingerprint (hash) to detect changes
- **Citation:** When creating a task, attach a source citation (source ID + section + quote) via `--source-citation`
- **Validation rules:** 
  - **E7:** Every epic must cite a source (error if missing)
  - **E8:** Citations must reference sections that exist in the source document (error if invalid)
  - **E12:** A source citation cannot reference a section that conflicts with other requirements (citation conflict)
- **Citation acceptance:** Before completing a task, you may need to `arm accept-citation <task-id> --rationale "..."` to confirm the citation is valid

**Command examples:**
```bash
# Register a source document
arm sources add --url docs/requirements.md --type filesystem
arm sources sync

# Verify sources and get UUIDs
arm sources verify

# Create a task linked to a source at creation time
arm create --title "Task X" --parent STORY-001 \
  --source <source-uuid>

# Accept a citation with rationale
arm accept-citation TASK-001 --rationale "Requirement reviewed and approved by stakeholders"

# Validate citations across all tasks
arm validate
```

---

## 6. Confidence Levels: Draft to Verified

**Concept:** Work items can be created as draft (uncertain, inferred) or verified (confirmed by a human). A task moves from draft to verified when a coordinator explicitly confirms it via `arm confirm`. Downstream unblocking only occurs for verified nodes.

**Pattern:** Decomposition may produce draft items. Coordinators promote drafts to verified. Only verified tasks unblock dependents.

**How it works:**
- **Provenance:** Every issue has a `provenance` field with a `confidence` level: `draft` or `verified`
- **Draft creation:** When an AI agent creates a task via decomposition, it's initially `draft` with `method: "decomposed"`
- **Verification:** A human coordinator reviews the draft and runs `arm confirm <task-id>` to promote it to `verified`
- **Promotion operation:** The `confirm` command emits an `OpDAGTransition` that updates the provenance to `verified`
- **Unblocking rule:** A task unblocks its dependents **only if** its provenance is `verified`; draft tasks do not unblock downstream work

**Status vs. Confidence:**
- **Status** (open, claimed, in-progress, done, merged, blocked, cancelled) tracks the lifecycle of the work
- **Confidence** (draft, verified) reflects certainty about the requirement itself

**Command examples:**
```bash
# Create a task as draft (typically from decomposition)
arm create --title "Task X" --parent STORY-001 --confidence draft

# Review draft tasks
arm dag-summary   # filter by provenance.confidence manually or via `jq`

# Promote a draft to verified
arm dag-transition --issue TASK-001 --confidence verified

# View confidence in task details
arm show TASK-001 | jq '.provenance.confidence'
```

---

## 7. Single-Branch vs. Dual-Branch Modes

**Concept:** Armature adapts to your repository's branch protection policy. Unprotected repos use single-branch mode (all state on `main`). Protected repos use dual-branch mode (code on `main`, coordination on `_armature`).

**Pattern:** Use `--dual-branch` flag with `arm bootstrap` for protected-main repos. Without it, single-branch mode is used. Code changes and coordination changes never mix within a single phase.

**How it works:**

### Single-Branch Mode (Unprotected Repos)
- All `.armature/` state lives on `main`
- No separate coordination branch
- Workers commit ops directly to `main`
- Tasks move directly from `done` to complete (no merge phase)
- Simpler but no separation of concerns

### Dual-Branch Mode (Protected Repos)
- **`main`:** Code and feature branches only; direct push disabled
- **`_armature`:** Orphan branch for `.armature/` coordination data; direct push allowed by all workers
- **`.arm/` worktree:** Secondary worktree checked out on `_armature`, accessible at `.arm/.armature/`
- **Two-phase completion:**
  - `done` = orchestration complete; code change pushed to a feature branch as a PR
  - `merged` = Armature detects that the PR landed on main; tasks now unblock downstream
- **Op phases:** 
  - Phase 1: Worker appends op to ops log on `_armature` branch
  - Phase 2: Worker pushes op batch to remote
  - Phase 3: Code worktree (main) continues independently

**Initialization:**
```bash
# Auto-detect and initialize
arm bootstrap

# Force dual-branch mode (even if main is unprotected)
arm bootstrap --dual-branch

# Check current mode
cat .armature/config.json | jq '.mode'
```

**Command examples:**
```bash
# Single-branch workflow (code and state on main)
arm claim TASK-001
arm transition TASK-001 --to done

# Dual-branch workflow (state on _armature, code on main)
git checkout -b feature/TASK-001
arm claim TASK-001
arm transition TASK-001 --to done      # goes to "done"
git push -u origin feature/TASK-001 && open PR
# ... merge PR ...
arm list   # auto-detects merge via commit-message scan, transitions to "merged"
```

---

## 8. Source Documents and Decomposition

**Concept:** Armature starts with source documents (requirements, PRDs, architecture guides) that define the work to be done. An AI agent decomposes these documents into a task DAG, creating epics, stories, and tasks. The decomposition process is repeatable and auditable.

**Pattern:** Register sources → generate decomposition context → AI decomposes → apply plan → tasks become ready.

**How it works:**
- **Source registration:** `arm sources add` registers a document by URL (filesystem, GitHub, Confluence, etc.)
- **Fingerprinting:** `arm sources sync` fetches the document and records a fingerprint (SHA hash) to detect changes
- **Decomposition context:** `arm decompose-context --sources <uuid>` generates a JSON context document summarizing the source
- **AI decomposition:** The context is fed to an AI agent (e.g., Claude, Gemini) with instructions to produce a `plan.json`
- **Plan structure:** A plan is a JSON array of issue objects (id, type, title, parent, acceptance, definition_of_done, scope, source_citation)
- **Plan application:** `arm decompose-apply --plan plan.json` creates the DAG from the plan
- **Idempotency:** Re-running decomposition on the same source can yield new/updated tasks or detect conflicts

**Source providers:**
- **filesystem:** Local file (e.g., `docs/prd.md`)
- **github:** GitHub file or PR (e.g., `github.com/owner/repo/blob/main/docs/prd.md`)
- **confluence:** Confluence page (requires credentials)
- **url:** Generic HTTP fetch (requires authentication header in some cases)

**Command examples:**
```bash
# Register a source
arm sources add --url docs/requirements.md --type filesystem
arm sources add --url "https://github.com/owner/repo/blob/main/docs/prd.md" --type github
arm sources sync

# Verify and get UUID
arm sources verify
# Output:
# SOURCE-UUID  docs/requirements.md              [fingerprint: abc123...]

# Generate decomposition context for the AI agent
arm decompose-context --sources SOURCE-UUID > context.json

# The context.json contains:
# - Document title and source URL
# - Full document text
# - Current DAG state
# - Instructions for decomposition

# Feed to AI agent and get back plan.json, then apply:
arm decompose-apply --plan plan.json

# List tasks created by decomposition
arm list --confidence draft   # newly created draft tasks

# Check source citations on tasks
arm show TASK-001 | jq '.source_links'
```

---

## 9. Semantic Conformance Review

**Concept:** After each task completes, semantic review validates that the delivered work conforms to its acceptance criteria and issue contract. A reviewer assesses code quality, scope adherence, and problem-solution fit. Assessments are recorded and must pass before story sign-off.

**Pattern:** Prepare review bundle → dispatch reviewer agent → record assessment. Red ratings block progression until remediated.

**How it works:**

- **ReviewBundle:** Created by `arm review prepare --issue TASK-ID --base BASE-SHA --head HEAD-SHA`, this JSON object contains:
  - The issue's acceptance criteria (what success looks like)
  - Scope (which files are in-scope for the change)
  - The unified diff between base and head commits
  - Issue metadata (title, type, definition-of-done)

- **Reviewer dispatch:** The coordinator passes the ReviewBundle to the `armature-reviewer` skill after each task completes. The reviewer:
  - Reads the acceptance criteria and scope
  - Examines the diff to understand what changed
  - Assesses whether the delivery is complete, correct, and clean
  - Returns a `ConformanceAssessment`

- **ConformanceAssessment:** Structured reviewer output containing:
  - **Rating:** `green` (passes all criteria), `yellow` (minor issues, minor remediation), or `red` (fails acceptance or scope)
  - **Findings:** Concrete observations about code quality, completeness, and adherence to acceptance criteria
  - **Remediation:** If red or yellow, specific actions required before unblocking downstream work
  - **Timestamp:** When the review was performed

- **Rating algebra:**
  - `green`: All acceptance criteria met; no scope violations; code quality acceptable
  - `yellow`: Non-blocking issues (e.g., minor style, incomplete test edge case); delivery is acceptable but improvements suggested
  - `red`: Acceptance criteria not met; scope violated; code quality concerns that prevent merge; requires rework

- **Recording and gating:** `arm review record --issue TASK-ID --assessment <assessment.json>` persists the assessment. Red ratings are escalated to the coordinator; yellow ratings surface to the auditor. Only green assessments auto-unblock dependent tasks.

**Scope distinction:** The **Auditor** checks structural integrity (citations, repo health). The **Reviewer** checks semantic correctness (acceptance criteria, code quality). Both gates are required for story completion.

**Command examples:**
```bash
# Coordinator creates the review bundle
arm review prepare --issue TASK-001 --base abc123def --head def456ghi > bundle.json

# Pass bundle to reviewer (dispatched as a skill, returns assessment.json)

# Record the assessment
arm review record --issue TASK-001 --assessment assessment.json
```

---

## Integration with Agents

Armature ships bundled **skills** that agents use to execute these concepts:

- **armature-worker:** Quick reference for the 8 concepts above
- **armature-coordinator:** Coordinator workflow (ready → claim → render-context → dispatch → integrate)
- **armature-planner:** Decomposition workflow (register sources → decompose context → create plan → apply)
- **armature-auditor:** Validation workflow (check citations, staleness, DAG integrity)

Deploy skills with:
```bash
arm bootstrap
```

or to deploy globally:
```bash
arm bootstrap --global
```

Agents retrieve these skills automatically when working with Armature tasks.

---

## See Also

- [Getting Started](getting-started.md) — Setup and first task
- [Commands Reference](commands.md) — Complete command documentation
- [Configuration Reference](configuration.md) — TTL, token budgets, hooks
- [Validation Codes](validation-codes.md) — E1–E12 errors, W1–W11 warnings
