# Armature Architecture Document

**Purpose:** Technical architecture for implementation. Used alongside the PRD to generate epics and stories.

**Audience:** Implementers (human and AI), tech leads decomposing work.

**Relationship to design docs:** This document supersedes DESIGN.md, DESIGN-EXTENSION.md, DESIGN-WORKFLOWS.md, DESIGN-BRANCHING.md, and GAP-RESOLUTIONS.md by consolidating all final decisions into a single source of truth. The design docs remain as historical rationale artifacts — this document is what you build from.

**CLI name:** `arm`

---

## 1. System Overview

Armature is a file-based, git-native work orchestration system designed for multi-agent AI coordination. Workers (human and AI) operate independently with their own working copies, coordinating exclusively via `git push/pull` to a shared origin. The system is merge-conflict-free by construction, requires no external dependencies beyond git, and minimizes context consumption for AI agents.

Distributed as a single Go binary (`arm`). Open source.

### Key Constraints

- **Git-native:** All state lives in git. No database, no server, no daemon.
- **Single binary:** One compiled Go binary, zero runtime dependencies beyond `git` (v2.25+ for sparse checkout).
- **Zero external deps:** Core operations (claim, transition, render-context, materialize) never make network calls beyond `git push/pull`. Only `sources add` and `sources sync` contact external providers.
- **Merge-conflict-free by construction:** Each worker writes exclusively to its own log file. No two workers ever write the same file.

### Supported Platforms and Agentic Tools

Platform matrix: linux/macOS/Windows × amd64/arm64. Compatible with Claude Code, Gemini CLI, Cursor, Windsurf, Kiro, and any tool that can invoke a subprocess and parse JSON.

### Terminology

| Term | Definition |
|---|---|
| **Worker** | Any entity (human or AI agent) that claims and executes tasks. Identified by UUID. |
| **Ops branch** | The `_armature` orphan branch storing all coordination data (`.armature/` directory). Unprotected, directly pushable by all workers. |
| **Code branch** | Feature branches off `main` (protected). Code changes flow here via PR/MR. |
| **Materialization** | The process of replaying all op logs to produce current state files (`index.json`, `ready.json`, etc.). |
| **Working memory** | The rendered context slice a worker receives when claiming a task — the output of `arm render-context`. Maps to CoALA's working memory. |
| **Op** | A single append-only JSONL entry in a worker's log file representing an atomic action (create, claim, heartbeat, transition, note, link, etc.). |
| **DAG** | The typed directed acyclic graph of work items: epic → story → task hierarchy plus `blocked_by` dependency edges. |

### Target Personas

Four deployment personas drive feature scope. See section 21 for the full persona-driven feature matrix.

- **Solo freelance:** Single-branch mode, no claim races, hooks optional.
- **Solo enterprise:** Dual-branch mode, no claim races, hooks recommended.
- **Team monorepo:** Dual-branch mode, full MRDT coordination, hooks recommended.
- **Team multi-repo:** Separate armature instances per repo (v1), manual cross-repo deps.

---

## 2. Branching Model

### Dual-Branch Architecture

```
main (protected)        — code, PRs, reviewed merges
_armature (unprotected)  — .armature/ directory, direct push by all workers
```

The `_armature` branch is an orphan branch with no common ancestor with `main`. All `.armature/` content lives here. Workers push ops directly to this branch. Code lives on feature branches and merges to `main` via PR.

**Why orphan branch:** An orphan branch shares no history with `main`, so `git merge` or `git rebase` between the two is impossible by accident. The ops data and code data are structurally isolated. The orphan also does not appear in `git log main`, keeping the code history clean.

### Ops Worktree

At `worker-init`, the CLI creates a secondary git worktree:

```
git worktree add --no-checkout .arm/ origin/_armature
cd .arm/
git sparse-checkout set .armature/ops .armature/state
```

The worktree directory is `.arm/` in the repository root. This is the default location; it can be overridden via git config (`armature.ops-worktree-path`). The `.arm/` directory is added to `.gitignore` on all branches.

Sparse checkout limits the ops worktree to essential directories (`ops/` and `state/`). `sources/cache/` remains on the ops branch but is excluded from the default sparse checkout to minimize disk usage. Workers needing source content (e.g., `arm sources sync`, `arm decompose-context`) expand the sparse checkout on demand.

### Single-Branch Fallback

If `_armature` does not exist and `main` is directly pushable (no branch protection), the CLI uses single-branch mode. All `.armature/` content lives on `main` alongside code. Detection is automatic at `arm init` time. This preserves viability for solo developers and non-enterprise setups.

### Worktree Lifecycle

| Event | Action |
|---|---|
| `arm init` | Creates `_armature` orphan branch if needed, sets up worktree, sparse checkout, `.gitignore` entry |
| `arm init --repair` | Re-creates worktree if stale or missing, re-installs hooks |
| Worker operation | CLI `cd`s to ops worktree internally, pulls, materializes, executes, commits, pushes |
| Worktree corruption | `arm init --repair` deletes and recreates the worktree from remote |

### Directory Structure (within `.arm/` worktree)

```
.armature/
  ops/
    SCHEMA                     # field definitions, versioned
    <worker-id>.log            # append-only, positional JSON arrays
    compacted-<sha>.log        # periodic compaction output (deferred)
  sources/
    manifest.json              # registered source documents (provider-agnostic)
    cache/
      <source-id>.md           # normalized markdown of fetched document
      <source-id>.meta.json    # {fetched_at, version_id, provider_version, sha}
  state/                             # LOCAL-ONLY — never committed, each worker materializes independently
    index.json                 # all issues: {uuid → denormalized summary}
    ready.json                 # precomputed ready-task queue
    checkpoint.json            # {"last_materialized_commit": "<sha>", "byte_offsets": {...}}
    traceability.json          # {node-id → [source_citations]}
    sources-fingerprint.json   # {source-id → {sha, version_id, last_verified}}
    issues/
      <uuid>.json              # full materialized state per issue
  templates/
    bug.json                   # issue type templates with required fields
    story.json
    task.json
    decomposition-prompt.md    # prompt template for decompose-context
  config.json                  # repository-level configuration
  hooks/
    post-commit                # hook templates installed to .git/hooks/
    post-merge
    prepare-commit-msg
  review/
    dag-summary.md             # generated at decomposition, audit artifact
```

**Key invariant:** All mutations go through log ops. `state/` files are derived/materialized locally by each worker and never committed to the ops branch — this is essential to the merge-conflict-free guarantee. `sources/` files are derived from provider fetches and committed to the ops branch for shared caching, but are never directly mutated during normal operations.

---

## 3. Data Model

### Op Log Format

Positional JSON arrays (JSONL), one per line, approximately 40% smaller than keyed JSON objects:

```jsonl
["create","abc-123",1740700800,"worker-a1",{"title":"Fix auth","parent":"epic-1","type":"task","scope":["src/auth/**"],"acceptance":[...]}]
["claim","abc-123",1740700801,"worker-a1",{"ttl":60}]
["heartbeat","abc-123",1740700850,"worker-a1",{}]
["transition","abc-123",1740700900,"worker-a1",{"to":"done","outcome":"Added refresh logic","branch":"feat/task-01","pr":"#142"}]
["note","abc-123",1740700950,"worker-a1",{"msg":"Discovered PKCE requirement, added code_verifier"}]
["link","task-1",1740700802,"worker-a1",{"dep":"task-2","rel":"blocked_by"}]
["source-link","node-01",1740700810,"worker-a1",{"source_id":"src-001","section":"Token Refresh","anchor":"#token-refresh","quote":"short verbatim phrase"}]
["source-fingerprint","src-001",1740700820,"worker-a1",{"sha":"abc123","version_id":"v14","provider":"confluence"}]
["dag-transition","root",1740700830,"worker-a1",{"to":"approved","uncovered_acknowledged":["task-a","task-b"]}]
["decision","node-01",1740700840,"worker-a1",{"topic":"storage-backend","choice":"redis","rationale":"low-latency","affects":["src/session/**"]}]
```

**Positional format:** `[op_type, target_id, timestamp_epoch, worker_id, payload_object]`. The SCHEMA file defines field positions. Forward/backward compatibility: new fields appended to array, old readers ignore extra positions, missing positions get defaults.

**Why JSONL with positional arrays:**

- Binary formats (protobuf, msgpack) rejected: git stores text diffs efficiently; binary blobs get full-copy deltas in packfiles. `git diff`, `grep`, and standard Unix tools work on JSONL.
- JSON Schema rejected: adds validator library dependency for marginal benefit over code-level validation in a single-language CLI.
- Code-level guardrails provide the same compatibility contract: append-only fields, defaults for missing positions, never change field types.

**Exception:** If materialized snapshots exceed ~50MB, MessagePack is a viable drop-in for snapshot files only (not logs). Do not implement until measured.

### Complete Op Type Catalog

| Op Type | Target | Payload Fields | Written By |
|---|---|---|---|
| `create` | issue ID | title, parent, type, scope, acceptance, definition_of_done, context, source_citation, priority, estimated_complexity | Any worker |
| `claim` | issue ID | ttl | Claiming worker |
| `heartbeat` | issue ID | (empty) | Claiming worker |
| `transition` | issue ID | to, outcome, branch (optional), pr (optional) | Claiming worker |
| `note` | issue ID | msg | Any worker |
| `link` | source issue ID | dep, rel (`blocked_by`) | Any worker |
| `source-link` | node ID | source_id, section, anchor, quote | Any worker |
| `source-fingerprint` | source ID | sha, version_id, provider | Any worker |
| `dag-transition` | root node ID | to, uncovered_acknowledged | Signing worker |
| `decision` | node ID | topic, choice, rationale, affects | Any worker |

### Decision Conflict Resolution

When multiple `decision` ops target the same `topic`, **last-write-wins by timestamp**. During materialization, the decision with the latest timestamp takes precedence. Earlier decisions for the same topic are preserved in the log for auditability but do not appear in the active context (Layer 5). `arm validate` surfaces a warning (W8) when conflicting decisions exist for the same topic, and `arm dag-summary` displays all conflicting decisions for human resolution.

All ops route through the issuing worker's own log file. There is no shared `source-events.log` — this preserves the one-writer-per-file invariant unconditionally.

### Op Authentication

**Filename-worker-ID validation:** During materialization, the CLI validates that every op in `<worker-id>.log` has a `worker_id` field matching the log filename. Ops failing this check are flagged as invalid, excluded from materialization, and surfaced as warnings. This prevents cross-file impersonation — a worker cannot emit ops as another worker by appending to someone else's log file.

For stronger authentication, teams can maintain a `worker-registry.json` on the ops branch mapping worker UUIDs to git committer identities. When present, materialization cross-references the git commit author against the worker ID in each op. Mismatches are flagged. This is optional — git host ACLs remain the primary access control.

### Transition Op Extended Fields

The transition op records metadata needed for merge detection:

```jsonl
["transition","task-01",1740700900,"worker-a1",{"to":"done","outcome":"...","branch":"feat/task-01","pr":"#142"}]
```

`branch` (feature branch name) and `pr` (PR/MR number) are both optional. They are used by the merge detection algorithm (section 10). The `prepare-commit-msg` hook ensures issue IDs appear in commit messages as the primary detection anchor.

### Issue Statuses

| Status | Meaning | Set By |
|---|---|---|
| `open` | Created, not yet claimed | `create` op |
| `claimed` | Worker has claimed, TTL active | `claim` op |
| `in-progress` | Work actively underway | `heartbeat` op (implicit) |
| `done` | Worker believes work is complete, PR submitted | `transition` op |
| `merged` | Code confirmed on main via PR merge | CLI auto-detection during materialization |
| `blocked` | Worker unable to complete, needs resolution | `transition` op |
| `cancelled` | Reverted or discarded | `transition` op (e.g., `decompose-revert`) |

**Reverse transitions:** The following reverse transitions are permitted:
- `claimed` → `open`: Worker unclaims (wrong task, rebalancing).
- `done` → `open`: PR rejected or work found to be insufficient. Emitted via `arm reopen <issue-id>`. The previous outcome is preserved in a `prior_outcomes` array on the materialized node, providing context for the next worker.
- `blocked` → `open`: Blocker resolved externally.

All other reverse transitions (e.g., `merged` → anything) are prohibited. `merged` is terminal for tasks.

### Node Schema

Core fields per materialized issue (`state/issues/<uuid>.json`):

```json
{
  "id": "task-callback-01",
  "type": "task",
  "status": "open",
  "title": "Implement OAuth2 callback handler",
  "parent": "story-oauth-01",
  "children": [],
  "blocked_by": ["task-provider-config-01"],
  "blocks": [],
  "assignee": null,
  "priority": "high",
  "estimated_complexity": "small",
  "definition_of_done": "GET /auth/callback exchanges code for tokens and creates session",
  "scope": ["src/auth/callback.ts", "src/auth/session.ts"],
  "context_files": ["docs/auth-architecture.md"],
  "ignore": ["src/auth/legacy/**"],
  "acceptance": [
    {"type": "test_passes", "pattern": "tests/auth/callback.test.ts"},
    {"type": "function_exists", "file": "src/auth/callback.ts", "name": "handleCallback"},
    {"type": "no_regression", "suite": "tests/auth/**"}
  ],
  "context": [
    {"snippet": "Callback must validate state parameter and exchange code within 60s."},
    {"file": "docs/auth-architecture.md", "section": "## Token Lifecycle", "lines": [45, 60]}
  ],
  "source_citation": [
    {
      "source_id": "src-001",
      "section": "Callback Specification",
      "anchor": "#callback-spec",
      "quote": "exchange authorization code for access and refresh tokens"
    }
  ],
  "provenance": {
    "method": "decomposed",
    "confidence": "verified",
    "source_worker": "worker-a1"
  },
  "decision_refs": [],
  "outcome": null,
  "updated": 1740700802
}
```

**Provenance fields:**

- `method`: `decomposed` | `imported` | `manual` | `adversarial-reconciled`
- `confidence`: `verified` (human-reviewed) | `inferred` (brownfield import, requires confirmation) | `draft` (pre-sign-off decomposition output)
- `source_worker`: ID of the worker who created the node

### Index Schema

`state/index.json` carries denormalized fields for O(1) context assembly:

```json
{
  "task-callback-01": {
    "status": "open",
    "type": "task",
    "parent": "story-oauth-01",
    "children": [],
    "blocked_by": ["task-provider-config-01"],
    "blocks": [],
    "assignee": null,
    "updated": 1740700802,
    "title": "Implement OAuth2 callback handler",
    "outcome": null,
    "scope": ["src/auth/callback.ts", "src/auth/session.ts"]
  }
}
```

Added vs. DESIGN.md baseline: `title`, `outcome` (populated when status is `done` or later), `scope`. Total index size increase is bounded at ~300 chars per done issue.

### Template Schema

Templates define required fields and constraints per issue type:

```json
{
  "required_fields": ["scope", "acceptance", "definition_of_done"],
  "max_description_length": 500,
  "max_dod_length": 200,
  "required_acceptance_types": ["test_passes", "no_regression"]
}
```

`max_description_length` is intentional — forces concise, actionable specifications.

### Scope Field: Multi-Repo Preparation

Task scope includes an optional `repo` field for future multi-repo support:

```json
{
  "scope": ["src/auth/callback.ts"],
  "repo": "acme/backend"
}
```

In single-repo mode (v1), `repo` is omitted (default: current repo). This is a schema addition only, not a behavioral change. See section 21 for the multi-repo roadmap.

---

## 4. Materialization Engine

### Canonical Process Flow

Every CLI command follows one of two process flows. Sync is implicit — workers never need to think about it.

**Read-only commands** (`arm ready`, `arm render-context`, `arm validate`, `arm status`, `arm metrics`, `arm context-history`):

```
1. cd ops-worktree && git pull          (sync ops)
2. Incremental materialize              (only new ops since checkpoint)
3. Execute command                      (read from state/ files)
```

**Read-write commands** (`arm claim`, `arm transition`, `arm heartbeat`, `arm note`, `arm create`, `arm link`, `arm decompose-apply`):

```
1. cd ops-worktree && git pull          (sync ops)
2. Incremental materialize              (only new ops since checkpoint)
3. Execute command                      (append to own log in ops-worktree)
4. git add .armature/ops/<worker-id>.log  (in ops-worktree)
5. git commit                           (in ops-worktree)
6. git push (retry with pull --rebase)  (to _armature)
```

Code commits happen separately in the developer's main worktree, on their feature branch. The CLI never touches the code worktree for ops writes.

### Push Retry Loop

```bash
while ! git push; do
  git pull --rebase
done
# Cap at ~5 retries. Failure beyond that is network/auth, not data conflict.
```

Rebase always succeeds because each worker only modifies its own file. The retry targets the ops branch exclusively; code pushes go through normal PR workflow and are not retried by the CLI.

### Incremental Materialization Algorithm

```
1. Read checkpoint.json → last processed commit SHA + byte offsets per log file
2. git log <last_sha>..HEAD -- .armature/ops/    (ops branch HEAD, not code branch)
3. Seek to stored byte offset in each changed file, parse only new lines
4. Apply new ops to state/ files
5. Run merged-status auto-detection on all 'done' tasks
6. Run bottom-up rollup pass (stories, epics)
7. Recompute ready.json
8. Update checkpoint.json
```

Converts each invocation from O(all ops) to O(new ops since last run).

**State files are local-only caches.** Checkpoint and state files (`state/index.json`, `state/ready.json`, `state/checkpoint.json`, etc.) are produced by each worker's local materialization and are NOT committed to the ops branch. This preserves the MRDT merge-conflict-free guarantee — only per-worker log files are committed. Each worker materializes independently from the append-only logs. The first invocation after a fresh clone performs a full materialization; subsequent invocations are incremental from the local checkpoint. State files are listed in `.arm/.gitignore` (or excluded via sparse checkout) to prevent accidental commits.

### Merged Status Auto-Detection

During materialization, the CLI checks all `done` tasks for merge evidence using a 4-layer fallback:

**Primary — commit-message scan:**

```
git log main --oneline --grep="ISSUE_ID" --since="TASK_CREATED" -- SCOPE_FILES
```

Works across merge strategies (merge commit, squash, rebase) because it searches commit messages, not branch ancestry.

**Fallback 1 — branch-name check:** If the transition op recorded a branch name, check `git branch --merged main`. Works for non-squash merges.

**Fallback 2 — scope-file heuristic:** Check if files in `scope.modify` changed on main after the transition timestamp. Fuzzy but flags candidates for manual confirmation.

**Fallback 3 — explicit command:** `arm merged ISSUE_ID`. Manual, low friction.

### Bottom-Up Rollup

After processing merged-status promotions for tasks, the materializer runs a bottom-up rollup pass:

| Type | Becomes `done` when | Becomes `merged` when |
|---|---|---|
| Task | Worker transitions it | CLI detects code on main |
| Story | All child tasks are `merged` | Same moment (auto-set with `done`) |
| Epic | All child stories are `merged` | Same moment (auto-set with `done`) |

For stories and epics, `done` and `merged` are set simultaneously because there is no separate code artifact to review. The two-phase gap (done but not yet merged) only exists for tasks, which are the leaf nodes that produce code.

### State Files Produced (Local-Only)

All state files are produced locally by each worker's materialization and are never committed to the ops branch. This preserves the one-writer-per-file MRDT guarantee.

| File | Contents |
|---|---|
| `index.json` | All issues with denormalized summary fields |
| `ready.json` | Precomputed ready-task queue |
| `checkpoint.json` | Last materialized commit SHA + per-log byte offsets |
| `traceability.json` | Node-to-source-citation mapping |
| `sources-fingerprint.json` | Source document SHA and version tracking |
| `issues/<uuid>.json` | Full materialized state per issue |

**Cold-start behavior:** The first invocation after a fresh clone performs a full materialization from all ops logs. For large projects (10,000+ ops at ~150 bytes/op), this is ~1.5MB of parsing — under 1 second at Go speeds. Compaction (deferred, section 4) directly addresses long-term growth. A progress indicator is shown during cold-start materialization.

**Checkpoint resilience:** The checkpoint tracks both the ops branch HEAD SHA and per-log byte offsets. On SHA miss (e.g., after a remote force-push or fresh clone), the materializer falls back to byte-offset-based incremental processing. Since log files are append-only, byte offsets are stable. Full re-materialization is the final fallback.

### Cycle Detection

Dependency cycles are detected via DFS during materialization. Cyclic links are flagged `"valid": false` in the materialized state. The original op is preserved in the log for auditability. `arm validate` also checks for cycles.

### Compaction (Deferred)

`arm compact` is designed but not implemented in v1. When implemented, it will rewrite all worker logs into `ops/compacted-<sha>.log` containing one synthetic op per issue (current state), then truncate individual logs of pre-compaction ops. History is preserved in git. Do not implement until materialization performance is measured to be a problem.

---

## 5. Ready Task Computation

### Ready-Task Rules

A task is ready when all four conditions hold:

1. `status == "open"`
2. All `blocked_by` issues have `status == "merged"` (not `done` — two-phase completion requires code on main)
3. Parent is `in-progress` (or parent is null)
4. Not claimed, or current claim is expired (no heartbeat within TTL)

A story becomes `in-progress` when at least one child task is `claimed` or `in-progress`.

### Priority Sort Order

Ready tasks are sorted by:

1. **Explicit priority field** (highest first)
2. **Depth in hierarchy** (deeper = more specific = shorter work)
3. **Number of downstream issues unblocked** (higher unblock count first)
4. **Age** (oldest first, prevents starvation)

### Ready Entry Format

```json
{
  "issue": "task-3",
  "type": "task",
  "parent": "story-1",
  "title": "Implement token refresh",
  "priority": "high",
  "scope": ["src/auth/token.ts", "src/auth/refresh.ts"],
  "estimated_complexity": "small"
}
```

Nodes with `confidence: "inferred"` (brownfield imports) include `"requires_confirmation": true` and cannot be claimed until a human confirms via `arm confirm <node-id>`.

### Claim Race Resolution

Two workers can both claim the same issue between pulls. Both pushes succeed (different files). Resolution is at **read time**: first claim by timestamp wins. Deterministic tiebreaker on worker ID (lexicographic) for identical timestamps.

Losing worker discovers loss on next `pull + materialize` cycle and moves on.

### Claim TTL and Heartbeat Protocol

Workers append a heartbeat at the start of each significant operation while working a claimed issue:

```jsonl
["heartbeat","abc-123",1740700850,"worker-a1",{}]
```

**Stale claim rule:** A claim is reclaimable if no heartbeat or transition from the claiming worker exists within `ttl` minutes of the last heartbeat/claim timestamp.

TTL is per-claim. Ephemeral CI agents: `ttl: 15`. Long-running human-supervised agents: `ttl: 1440`.

### Claim-Time Scope Overlap Advisory

When a worker claims a task, the CLI checks whether any currently `claimed` or `in-progress` task has overlapping scope (using the same glob-matching logic as W1). If overlap is detected, the claim still succeeds (no blocking — consistent with the advisory philosophy) but:
- A warning is emitted to the claiming worker's output.
- A `note` op is automatically appended to both the claimed task and the overlapping task, recording the overlap for context.

This gives agents awareness of potential semantic conflicts before investing a full work cycle.

### Post-Claim Verification Flow

After claiming and pushing, the worker should pull again and re-materialize to confirm it won the race before investing work. One extra pull — cheap insurance. The `arm claim` command handles this internally: claim, push, pull, re-check.

---

## 6. Context Assembly (Working Memory Hydration)

### Overview

`arm render-context` produces the exact context slice a worker receives when it claims a task. The algorithm is deterministic — given the same materialized state, it always produces the same output. This allows human reviewers to spot-check any node and see exactly what an agent will see.

### 7-Layer Assembly Algorithm

```
function render_context(issue_id, budget=1600, include_all_siblings=false):
    index   = load("state/index.json")
    issue   = load("state/issues/{issue_id}.json")

    context = new ContextBuilder()

    # ── Fixed layers (never truncated) ───────────────────────────

    # Layer 1: Core Spec
    context.add_fixed("header", {id, type, status, priority, title})
    context.add_fixed("definition_of_done", issue.definition_of_done)
    context.add_fixed("scope", {modify, read, ignore})
    context.add_fixed("acceptance", issue.acceptance)

    # Layer 2: Context Snippets
    context.add_fixed("snippets", issue.context)

    # ── Truncatable layers (priority-ordered) ────────────────────

    # Layer 3: Blocker Outcomes (priority 1)
    #   Direct blockers first, then transitive (1 level only)
    #   Available at 'done' (informational), unblocking requires 'merged'
    for dep_id in issue.blocked_by:
        dep = index[dep_id]
        if dep.status in ["done", "merged"] and dep.outcome:
            context.add_truncatable("blocker_outcome", {id, title, outcome}, priority=1)

    # Layer 4: Parent Chain (priority 2)
    #   Bottom-up: immediate parent first, then grandparent, max depth 5
    current_id = issue.parent
    while current_id and depth < 5:
        parent = index[current_id]
        context.add_truncatable("parent_summary", {id, type, title, status}, priority=2)

    # Layer 5: Open Decisions (priority 3)
    #   Only decisions whose 'affects' scope overlaps this task's scope
    for d in find_decisions_for_scope(issue.scope):
        context.add_truncatable("decision", {topic, choice, rationale}, priority=3)

    # Layer 6: Prior Notes (priority 4)
    #   Newest first. Provides continuity from previous workers.
    for n in get_notes_for_issue(issue_id).sort_by(timestamp, desc):
        context.add_truncatable("note", {worker, time, message}, priority=4)

    # Layer 7: Sibling Outcomes (priority 5)
    #   Scope-overlapping siblings only, unless --include-siblings
    for sib in done_siblings_of(issue):
        if include_all_siblings or scopes_overlap(sib.scope, issue.scope):
            context.add_truncatable("sibling_outcome", {id, title, outcome}, priority=5)

    # ── Token Estimation & Advisory Truncation ───────────────────
    estimated_tokens = len(context.render()) / 4
    if estimated_tokens > budget:
        context.truncate_by_priority(budget)

    return {sections, budget_info}
```

### Truncation Procedure

```
function truncate_by_priority(sections, budget_tokens):
    budget_chars = budget_tokens * 4

    # Fixed sections are never removed.
    # Truncatable sections removed lowest-priority-first (highest number first).
    # Within same priority, remove newest-added first (LIFO).

    while total_chars(sections) > budget_chars:
        lowest = sections.truncatable.pop_lowest_priority()
        if lowest is null: break   # only fixed sections remain, accept over-budget
        sections.remove(lowest)
```

### Token Budget

Advisory, not enforced. Proxy metric: `chars / 4`. Target range: 650–1,600 tokens per task context. Default budget from `.armature/config.json`, overridable per-invocation with `--budget <n>`.

The `chars/4` proxy is inaccurate for code-heavy and non-English content. This is accepted as advisory — agents that hit real context limits observe the failure empirically. The `--raw` flag bypasses all truncation.

### Agent Output Schema

```json
{
  "issue_id": "task-callback-01",
  "type": "task",
  "status": "open",
  "priority": "high",
  "title": "Implement OAuth2 callback handler",
  "definition_of_done": "GET /auth/callback exchanges code for tokens and creates session",
  "scope": {
    "modify": ["src/auth/callback.ts", "src/auth/session.ts"],
    "read": ["docs/auth-architecture.md"],
    "ignore": ["src/auth/legacy/**"]
  },
  "acceptance": [
    {"type": "test_passes", "pattern": "tests/auth/callback.test.ts"},
    {"type": "function_exists", "file": "src/auth/callback.ts", "name": "handleCallback"},
    {"type": "no_regression", "suite": "tests/auth/**"}
  ],
  "context_snippets": [
    "Callback must validate state parameter and exchange code within 60s.",
    "PKCE code_verifier is stored in encrypted session cookie, not localStorage."
  ],
  "blocker_outcomes": [
    {
      "id": "task-provider-config-01",
      "title": "Configure OAuth2 provider credentials",
      "outcome": "Provider config in src/auth/config.ts, env vars GOOGLE_CLIENT_ID, GITHUB_CLIENT_ID"
    }
  ],
  "open_decisions": [
    {"topic": "session-storage", "choice": "redis", "rationale": "Low-latency session lookup"}
  ],
  "prior_notes": [],
  "sibling_outcomes": [],
  "parent_chain": [
    {"id": "story-oauth-01", "type": "story", "title": "OAuth2 Authorization Code Flow", "status": "in-progress"},
    {"id": "epic-auth-01", "type": "epic", "title": "User Authentication System", "status": "in-progress"}
  ],
  "budget": {
    "estimated_tokens": 680,
    "advisory_limit": 1600,
    "status": "within_budget",
    "truncated_sections": 0
  }
}
```

### Human Output Format

When `--format=human` (default for TTY), output is Glamour-rendered markdown using the semantic color palette:

```
╭─ task-callback-01 ── high ── open ─────────────────────────────╮
│  Implement OAuth2 callback handler                              │
╰─────────────────────────────────────────────────────────────────╯

  Done when: GET /auth/callback exchanges code for tokens and
             creates session

  Scope
    modify: src/auth/callback.ts, src/auth/session.ts
    read:   docs/auth-architecture.md
    ignore: src/auth/legacy/**

  Acceptance
    ✓ test_passes  tests/auth/callback.test.ts
    ✓ fn_exists    src/auth/callback.ts → handleCallback
    ✓ no_regress   tests/auth/**

  Context
    • Callback must validate state parameter and exchange code within 60s.
    • PKCE code_verifier is stored in encrypted session cookie.

  ── Blocker Outcomes ──
  ✔ task-provider-config-01: Provider config in src/auth/config.ts,
    env vars GOOGLE_CLIENT_ID, GITHUB_CLIENT_ID

  ── Decisions ──
  session-storage → redis (low-latency session lookup)

  ── Parent Chain ──
  story-oauth-01  OAuth2 Authorization Code Flow           in-progress
  epic-auth-01    User Authentication System               in-progress

  ── Budget ──
  ~680 tokens / 1600 limit                               ██░░░░░░ 43%
```

Color mapping: header border = `Info` (blue), "Done when" label = `OK` (green), scope paths = `Muted` (gray), acceptance checkmarks = `OK`, blocker outcomes = `OK` (done) or `Critical` (pending), budget bar = `OK` if under, `Warning` if 80–100%, `Critical` if over.

### Blocker Outcome Availability

Blocker outcomes are included in the context when the blocker's status is `done` (informational — the agent can see what was produced). However, the ready-task gate requires `merged` to unblock downstream work. This distinction matters: an agent reviewing context can see a blocker's outcome early, but cannot claim the dependent task until that blocker's code is on main.

---

## 7. Decomposition Workflow

### Overview

Decomposition transforms registered source documents (PRD, architecture doc, etc.) into a typed DAG of actionable work items. The CLI provides scaffolding — prompt generation, structural validation, atomic batch creation — but does not embed LLM calls. The intelligence lives in the external AI agent or human operator.

### `arm decompose-context`

Generates a structured context package for an external AI to perform decomposition.

```
arm decompose-context [flags]

Flags:
  --sources <id,...>     Comma-separated source IDs (default: all registered)
  --existing-dag         Include current DAG state for brownfield reconciliation
  --template <path>      Override decomposition prompt template
                         (default: .armature/templates/decomposition-prompt.md)
  --format json|md       Output format (default: md for TTY, json for pipe)
  --output <path>        Write to file instead of stdout

Output (JSON):
{
  "prompt_template": "<string>",
  "sources": [
    {"source_id": "src-001", "type": "prd", "content": "<cached markdown>", "sha": "abc123", "version_id": "v14"}
  ],
  "existing_dag": {...},            // only if --existing-dag
  "constraints": {
    "node_templates": {
      "epic":  {"required_fields": [...]},
      "story": {"required_fields": [...]},
      "task":  {"required_fields": ["scope", "acceptance", "definition_of_done"]}
    },
    "max_description_length": 500,
    "max_dod_length": 200,
    "required_acceptance_types": ["test_passes", "no_regression"],
    "hierarchy_rules": "epic → story → task"
  },
  "plan_schema": {...}
}

Exit codes: 0 success, 1 no sources registered, 2 source cache missing (run arm sources sync).
```

### `arm decompose-apply`

Validates and atomically creates all nodes from a plan file. One invocation, one commit, one push.

```
arm decompose-apply <plan.json> [flags]

Flags:
  --dry-run              Validate only, do not write ops
  --root <id>            Override root node ID (default: inferred from plan)
  --generate-ids         Auto-generate UUIDs for nodes (ignores plan IDs)
  --strict               Treat advisory warnings as errors

Output (JSON):
{
  "created": 24,
  "links": 8,
  "source_links": 24,
  "root": "epic-auth-01",
  "warnings": [...],
  "errors": []
}

Exit codes: 0 success, 1 validation errors (nothing written), 2 plan file parse error, 3 git push failure after retries.
```

**Atomicity guarantee:** All ops are appended to the worker's log file in a single file write. If validation passes but push fails after max retries, the local log is rolled back to pre-apply state.

**Idempotency protection:** Each `decompose-apply` invocation generates a batch ID (UUID) recorded in every created op's payload, along with a SHA of the plan file. Before writing ops, the CLI scans existing logs for ops with a matching plan file hash. If a prior batch from the same plan is detected (indicating a partial push that may have succeeded), the CLI warns and requires `--force` to proceed. This prevents duplicate node creation from network-timeout retries. The `--generate-ids` flag generates new UUIDs for nodes but preserves the batch-level deduplication check.

### `arm doctor`

Checks structural integrity of the local repo state. Intended as a **pre-work gate** — run once after `worker-init` and before claiming any issue to confirm the repo is in a coherent state.

```
arm doctor [flags]

Flags:
  --strict     Promote warnings to errors (non-zero exit on any finding)
  --format json

Checks:
  D1  Git/armature divergence — commits reference issues not in done/merged state
  D2  Stale claims — claimed issues with expired TTL
  D3  Orphaned ops — op log entries whose target_id is not in the issue graph
  D4  Broken parent refs — issues whose parent points to a non-existent node
  D5  Dependency cycles — blocked_by chains that form a cycle
  D6  Uncited issues — issues with no source_link and no accept-citation record

Exit codes: 0 clean (warnings OK), 1 errors found, non-zero with --strict on warnings.
```

**Design boundary — doctor vs validate:** These two commands have distinct responsibilities and distinct invocation points. Do not merge them.

| Command | Responsibility | When to run |
|---|---|---|
| `arm doctor` | **Structural** — can this repo function at all? (broken refs, orphaned ops, stale claims, cycles) | Pre-work: after `worker-init`, before claiming |
| `arm validate` | **Semantic** — is this repo honest? (citation coverage, source UUID validity, scope overlap) | Pre-push: before transitioning a story and opening a PR |

`arm doctor` operates on materialized state and the raw op log; it is intentionally fast and does not re-fetch or re-fingerprint sources. `arm validate` does deeper citation integrity (E7, E8) that requires resolving source UUIDs against the manifest. **D6 in doctor checks field presence only** (does a source_link or accept-citation record exist?); it does not validate that the cited source UUID exists in the manifest — that is validate's job (E8).

This separation keeps the pre-work gate cheap and the pre-push gate thorough. Running doctor before every claim session is low-friction. Running validate before every push is the right moment for the slower source-resolution check.

### `arm validate`

Checks semantic integrity of the current materialized DAG. Intended as a **pre-push gate** — run after all story tasks are done and before transitioning the story and opening a PR.

```
arm validate [flags]

Flags:
  --scope <node-id>      Validate subtree only (default: full DAG)
  --ci                   Machine-parseable output, non-zero exit on errors

Output (JSON):
{
  "errors": [...],
  "warnings": [...],
  "summary": {
    "total_nodes": 24,
    "cited_nodes": 22,
    "coverage_pct": 91.7,
    "cycles": 0,
    "orphan_links": 0
  }
}

Exit codes: 0 clean, 1 errors found, 0 warnings-only (unless --ci).
```

### `arm import`

Brownfield import from external sources.

```
arm import <file> [flags]

Flags:
  --format jira|linear|csv|json   Input format (default: auto-detect)
  --source <source-id>            Attach all imported nodes to this source
  --dry-run                       Parse and validate only

Behavior: Creates nodes with provenance.method="imported", provenance.confidence="inferred".
```

Imported nodes with `inferred` confidence appear in `ready.json` with `requires_confirmation: true`. Workers cannot claim them until a human confirms via `arm confirm <node-id>`.

### Structural Validation Rules

`decompose-apply` enforces these checks before writing any ops. All checks also run in `--dry-run` mode.

**Errors (block creation):**

| # | Check | Error Message |
|---|---|---|
| E1 | Node ID unique within plan AND existing DAG | `duplicate node ID: {id}` |
| E2 | Parent reference resolves (plan or existing DAG) | `unresolved parent: {parent} for node {id}` |
| E3 | Link targets resolve (plan or existing DAG) | `unresolved link target: {to} from {from}` |
| E4 | No cycles in combined DAG (DFS) | `cycle detected: {node} → ... → {node}` |
| E5 | Type hierarchy valid (epic→story→task) | `invalid hierarchy: task {id} cannot parent story {child}` |
| E6 | Task nodes have scope, acceptance, definition_of_done | `missing required field: {field} on task {id}` |
| E7 | All nodes have ≥1 source_citation | `uncited node: {id}` |
| E8 | Cited source_id exists in sources/manifest.json | `unknown source: {source_id} in citation for {id}` |
| E9 | definition_of_done ≤ max_dod_length | `definition_of_done exceeds {max} chars on {id}` |
| E10 | Scope globs are syntactically valid | `invalid glob: {pattern} on {id}` |
| E11 | Plan version field present and supported | `unsupported plan version: {v}` |
| E12 | source_versions match current cache SHAs | `stale source: {id} plan={sha} cache={sha}` |

**Warnings (advisory, surfaced in output, errors with `--strict`):**

| # | Check | Warning Message |
|---|---|---|
| W1 | Scope overlap between siblings | `scope overlap: {nodes} both modify {files}` |
| W2 | No acceptance of type `test_passes` | `no test criteria on {id}` |
| W3 | Estimated context budget exceeded | `budget advisory: {id} est. {n} tokens > {limit}` |
| W4 | Scope glob is very broad (e.g., `**/*`) | `broad scope: {id} scope covers entire tree` |
| W5 | No `context_files` for scope spanning 3+ dirs | `missing context_files on {id} with broad scope` |
| W6 | Estimated complexity inconsistent with file count | `complexity mismatch: {id} has {n} files but marked {complexity}` |
| W7 | definition_of_done contains vague language | `vague DoD: {id} contains "{word}"` (e.g., "properly", "correct", "good") |
| W8 | Conflicting `decision` ops for the same `topic` | `conflicting decisions: topic "{topic}" has {n} choices: {choices}` |
| W9 | Recent main commits lack issue ID patterns (merge detection coverage) | `low merge-detection coverage: {n}% of recent main commits lack issue IDs` |
| W10 | Scope path does not match any existing file in code worktree | `phantom scope: {path} on {id} does not match any file` (warning, not error — task may create new files) |
| W11 | Transition outcome is too short or matches low-value patterns | `vague outcome: {id} outcome is {n} chars` (triggered below 20 chars or matching patterns like "done", "completed") |

### Plan File Format

The plan file is the contract between the decomposing agent and `arm decompose-apply`:

```json
{
  "version": 1,
  "root": "epic-auth-01",
  "source_versions": {
    "src-001": "abc123",
    "src-002": "def456"
  },
  "nodes": [
    {
      "id": "epic-auth-01",
      "type": "epic",
      "title": "User Authentication System",
      "definition_of_done": "Users authenticate via OAuth2 with Google and GitHub",
      "source_citation": [{"source_id": "src-001", "section": "Authentication Requirements", "anchor": "#authentication-requirements", "quote": "support OAuth2 with multiple identity providers"}]
    },
    {
      "id": "task-callback-01",
      "type": "task",
      "parent": "story-oauth-01",
      "title": "Implement OAuth2 callback handler",
      "definition_of_done": "GET /auth/callback exchanges code for tokens and creates session",
      "scope": ["src/auth/callback.ts", "src/auth/session.ts"],
      "context_files": ["docs/auth-architecture.md"],
      "ignore": ["src/auth/legacy/**"],
      "acceptance": [
        {"type": "test_passes", "pattern": "tests/auth/callback.test.ts"},
        {"type": "function_exists", "file": "src/auth/callback.ts", "name": "handleCallback"},
        {"type": "no_regression", "suite": "tests/auth/**"}
      ],
      "context": [
        {"snippet": "Callback must validate state parameter and exchange code within 60s."}
      ],
      "source_citation": [{"source_id": "src-001", "section": "Callback Specification", "anchor": "#callback-spec", "quote": "exchange authorization code for access and refresh tokens"}],
      "estimated_complexity": "small",
      "priority": "high"
    }
  ],
  "links": [
    {"from": "task-refresh-01", "to": "task-callback-01", "rel": "blocked_by"}
  ]
}
```

### Greenfield Flow

```
1. arm sources add <prd-url>
2. arm sources add <arch-doc-url>
3. arm sources sync
4. arm decompose-context > context.json
5. <AI agent reads context.json, produces plan.json>
6. arm decompose-apply plan.json --dry-run
7. <Fix validation errors, re-run AI if needed>
8. arm decompose-apply plan.json
9. arm dag-summary          # human reviews, interactive sign-off
```

Sign-off via `arm dag-summary` is the governance gate for DAG structure. There is no PR on the ops branch — the ops branch must remain unprotected for low-latency coordination.

### Brownfield Flow

```
1. arm import existing-tasks.csv --format csv --source src-001
   # Creates nodes with confidence=inferred
2. arm sources add <prd-url>
3. arm sources sync
4. arm decompose-context --existing-dag > context.json
   # Context includes imported nodes for AI to reconcile
5. <AI produces plan.json, referencing or extending imported nodes>
6. arm decompose-apply plan.json --dry-run
7. arm decompose-apply plan.json
   # New nodes are confidence=draft, imported remain inferred
8. arm dag-summary
   # Summary distinguishes draft (new) vs inferred (imported, needs confirmation)
9. <Human reviews, confirms imported nodes individually>
10. arm confirm <node-id>    # promotes inferred → verified
```

### Partial and Failed Decomposition

**AI produces invalid plan:** `arm decompose-apply --dry-run` reports all errors. The operator fixes the plan (or re-prompts the AI with error messages) and retries. No state is written until validation passes.

**AI produces partial plan (subset of requirements):** Valid. `arm decompose-apply` creates whatever is in the plan. `arm dag-summary` shows uncited source sections (requirements not covered by any node). The reviewer flags gaps. A second decomposition pass can add nodes to an existing DAG.

**AI hallucinates source citations:** E8 validation catches references to non-existent source IDs. Citations to real sources with wrong sections are not caught structurally — this is the reviewer's responsibility at the `dag-summary` gate.

**Push fails after retries:** Local log is rolled back. The operator diagnoses the git issue and re-runs. No partial state leaks to the remote.

### `arm decompose-revert`

Discards a decomposition while preserving the audit trail. This follows double-entry accounting principles: rather than deleting ops (which would require history rewriting), the command emits cancellation ops.

```
arm decompose-revert <root-id>

Behavior:
  1. Walks the subtree rooted at <root-id>
  2. For each node under the root, emits a transition op: {"to": "cancelled"}
  3. All cancellation ops are appended to the current worker's log
  4. Single commit, single push

Effect:
  - All nodes under <root-id> transition to 'cancelled' status
  - Cancelled nodes are excluded from ready.json
  - The original create ops and the cancellation ops both remain in the log
  - Full audit trail preserved: who created, who cancelled, when
```

The alternative is `git reset --hard` on the ops branch, which is a standard git operation but destroys the audit trail. `arm decompose-revert` is preferred when traceability matters.

### Decomposition Prompt Template

Ships as `.armature/templates/decomposition-prompt.md`, overridable per-repo. The `arm decompose-context` command interpolates source content, constraints, and existing DAG into template markers (`{{SOURCES}}`, `{{EXISTING_DAG}}`, `{{PLAN_SCHEMA}}`, `{{CONSTRAINTS}}`).

The template specifies: hierarchy rules (epic → story → task), task granularity targets (1–4 hours, 1–3 files), source traceability requirements (every node must cite a source), acceptance criteria standards, context snippet guidelines, dependency rules, scope isolation, and anti-patterns to avoid.

---

## 8. Source Document Management

### Source Identity Model

Each source document is registered with a provider-agnostic identity:

```json
{
  "source_id": "src-001",
  "provider": "confluence",
  "address": {
    "base_url": "https://acme.atlassian.net/wiki",
    "space_key": "ENG",
    "page_id": "123456789"
  },
  "type": "prd",
  "registered_at": 1740700800,
  "version_at_registration": {
    "version_number": 14,
    "when": "2025-02-20T10:00:00Z"
  },
  "cached_sha": "abc123",
  "cache_path": ".armature/sources/cache/src-001.md"
}
```

### Supported Providers (v1)

| Provider | Address Fields | Version Signal | Reliability |
|---|---|---|---|
| Confluence | base_url, space_key, page_id | `version.number` (monotonic integer) | High |
| SharePoint | site_url, drive_id, item_id | `eTag` + `lastModifiedDateTime` | Medium (eTag changes on metadata edits) |
| Local filesystem | file path | File SHA | High |

### Caching Strategy

**Decision:** Aggressive caching. Cache is committed to the ops branch. Workers never fetch live documents — they read from cache. MCP calls are isolated to three commands only.

**Commands that make MCP calls:**

- `arm sources add <url>` — registers and fetches initial content
- `arm sources sync` — checks provider version metadata, fetches if changed
- `arm sources verify` — validates fingerprints, flags stale nodes

**All other commands** operate against local cache only.

**Rationale:** Documents are slowly changing and read-only by sprint time. Committed cache means all workers share the same snapshot, offline capability is preserved, and core operations remain zero-external-dependency.

The cache lives on the ops branch but is excluded from the default sparse checkout to minimize disk usage. Workers needing source content expand the sparse checkout on demand.

### Normalization to Markdown

All provider content is normalized to markdown on fetch and stored in `.armature/sources/cache/`. Mermaid diagrams embedded in Confluence pages are extracted as `<source-id>-<n>.mermaid` sidecar files. This is the abstraction boundary — everything downstream sees markdown regardless of provider.

### Fingerprinting

File-level SHA fingerprinting. Section-level SHA was rejected — the section parser complexity is not justified. The trade-off is a higher false-positive rate (entire doc flagged as changed on any edit), which is acceptable given reviewer cost is low.

`source-fingerprint` ops record the SHA, version ID, and provider for each source at the time of caching.

### Staleness Detection

During `arm sources sync`, the CLI compares the provider's current version metadata against the cached version. If changed, the CLI fetches the new content, recomputes the SHA, and emits a new `source-fingerprint` op.

**Cross-document consistency:** If a PRD version timestamp is newer than an architecture document version timestamp by more than a configurable threshold (default: 7 days), `arm dag-summary` surfaces a warning. Resolution is human responsibility.

### Credential Management

Stored in `~/.config/arm/providers.toml` — never in the repo:

```toml
[confluence]
base_url = "https://acme.atlassian.net/wiki"
auth_method = "pat"
token_env = "CONFLUENCE_PAT"

[sharepoint]
tenant_id = "..."
client_id = "..."
auth_method = "device_flow"   # interactive for humans; service_principal for CI
```

Workers without provider credentials can still perform all core operations. They cannot refresh the source cache. This is acceptable — cache refresh is a sprint-start or explicit operator action, not a per-task operation.

Environment variables (e.g., `CONFLUENCE_PAT`) provide the actual tokens. The `.toml` file maps providers to their credential location.

### MCP Integration Points

MCP is the transport layer for provider communication. The integration surface is narrow: three commands (`sources add`, `sources sync`, `sources verify`) use MCP to fetch and verify documents. The rest of the system is MCP-unaware.

---

## 9. Governance Model

### DAG Governance: `dag-summary` Interactive Sign-Off

With ops on an unprotected branch, the original governance model ("merge to protected branch = approval") cannot apply to DAG structure. The ops branch must remain unprotected for low-latency coordination.

The `dag-summary` interactive sign-off IS the governance gate for DAG structure. The interactive TUI checklist requires per-item acknowledgment, records log ops with worker attribution, and is mechanically impossible to bulk-approve.

### Code Governance: PR/MR on Protected Main

PRs on `main` remain the governance gate for code changes. This is unchanged from standard git workflow.

### Governance Boundaries

| What | Gate | Where |
|---|---|---|
| DAG structure (decomposition approval) | `dag-summary` interactive sign-off | ops branch |
| Code changes | PR/MR review on protected main | main branch |
| Source document registration | `dag-summary` coverage check | ops branch |
| Worker behavior | SKILL.md + pre-transition hooks | enforced by CLI |

### Draft Confidence Promotion

Nodes created via `arm decompose-apply` have `provenance.confidence: "draft"`. Draft nodes are not claimable. The `dag-summary` sign-off promotes `draft` to `verified` by recording a `dag-transition` op with the signing worker's ID.

### Inferred Confidence

Nodes created via `arm import` have `provenance.confidence: "inferred"`. These require explicit per-node confirmation via `arm confirm <node-id>` before workers can claim them. Bulk promotion is not supported — this rate-limiting is intentional to force human review of imported work items.

### Sign-Off Attribution

Each checklist item in `dag-summary` generates a log op with the signing worker's ID. This creates an audit trail: who approved which part of the DAG, when.

### Traceability

`arm dag-summary` surfaces coverage metrics: cited nodes / total nodes. Uncited nodes are named individually (not just counted). The reviewer must acknowledge each uncited node by name in the sign-off, creating attribution in the audit log.

`dag-summary.md` is committed to the ops branch as an audit artifact. It is generated by `arm dag-summary` and committed alongside the sign-off ops.

---

## 10. Two-Phase Completion

### `done` vs `merged` Semantics

With ops and code on separate branches, a `done` transition no longer guarantees code is on main. Downstream workers could see a blocker as "done" and start work before the code passes review.

| Status | Meaning | Set By |
|---|---|---|
| `done` | Worker believes work is complete, PR submitted | Worker via `arm transition` |
| `merged` | Code confirmed on main via PR merge | CLI auto-detection during materialization |

### Merge Detection Algorithm (4-Layer Fallback)

See section 4 (Materialization Engine) for the detailed algorithm. In summary:

1. **Commit-message scan** — searches `git log main` for the issue ID (primary)
2. **Branch-name check** — `git branch --merged main` using recorded branch name
3. **Scope-file heuristic** — checks if scoped files changed on main post-transition
4. **Explicit command** — `arm merged ISSUE_ID` (manual fallback)

### Story/Epic Rollup

Stories and epics are auto-promoted to `merged` when all their children are `merged`. For stories and epics, `done` and `merged` are set simultaneously because there is no separate code artifact to review. See section 4 for the rollup table.

### Impact on Ready-Task Computation

The ready-task blocker rule requires `status == "merged"`, not `done`. This ensures downstream work only begins after code has passed review and is on main.

### Edge Cases

**Squash-merge:** Commit-message scan (not ancestry) is the primary detection method, so squash-merges work as long as the issue ID appears in the squash commit message. The `prepare-commit-msg` hook ensures this.

**Abandoned PRs:** A task stuck at `done` with no PR merge triggers a staleness check. Configurable threshold (default: N days after `done` with no merge detected). Same pattern as expired claims — surfaced via `arm status`.

**Offline workers:** A worker transitions a task to `done` while offline. The transition op sits in their local log. When they push, the `done` status propagates. The two-phase model handles this correctly — `done` does not unblock downstream work, only `merged` does. The code PR still needs to be submitted and merged. No correctness issue.

---

## 11. Git Hook Automation

### Hook Catalog

`arm init` installs hooks from `.armature/hooks/` into `.git/hooks/`. **Git hooks are convenience, never enforcement.** Every action a git hook performs is also available as an explicit CLI command. The system is correct without git hooks installed.

**Important distinction:** Git hooks (this section) are convenience automation for heartbeats, commit-message stamping, and merge promotion. Pre-transition verification hooks (section 12) configured as `required: true` in `.armature/config.json` are **enforcement** — the CLI refuses to emit a `done` transition if a required verification hook fails, regardless of whether git hooks are installed. These are separate mechanisms with different trust models.

| Hook | Trigger | Action |
|---|---|---|
| `post-commit` | After any commit on a feature branch | Auto-emits heartbeat if worker has active claim; pushes to ops branch |
| `post-merge` | After `git pull` brings new commits to main | Runs `arm materialize` to pick up newly merged tasks and auto-promote `done` → `merged` |
| `prepare-commit-msg` | Before commit message editor opens | Prepends active claim's issue ID to commit message |

The `post-commit` hook is the highest-leverage automation: every code commit becomes a heartbeat, eliminating the "remember to heartbeat" burden. The `prepare-commit-msg` hook ensures the commit-message convention needed for merge detection is enforced mechanically.

### Installation and Bypass

- `arm init` installs hooks from version-controlled templates in `.armature/hooks/`
- `arm init --no-hooks` skips hook installation
- `arm init --repair` re-installs if hooks were removed or corrupted
- Standard `--no-verify` on git commands bypasses hooks
- Hooks fail silently (stderr redirected) — system is correct without them

### Coexistence with Hook Managers

Teams using husky, pre-commit, or other hook managers need a coexistence strategy. Recommended approach: Armature hooks are installed as scripts that can be sourced from existing hook files rather than replacing them. If a hook manager owns `.git/hooks/`, the armature hooks can be invoked from within the managed hook chain. Documentation should cover the integration pattern for the most common managers.

---

## 12. Pre-Transition Verification

### Hook Configuration

Verification hooks are configured in `.armature/config.json` on the ops branch, shared by all workers:

```json
{
  "pre_transition_hooks": [
    {
      "cmd": "npm test -- --testPathPattern={scope}",
      "label": "tests",
      "required": true,
      "exit_codes": {
        "0": "pass",
        "1": "test_failure",
        "*": "environment_error"
      }
    },
    {
      "cmd": "npx tsc --noEmit",
      "label": "typecheck",
      "required": true
    },
    {
      "cmd": "npx eslint {scope}",
      "label": "lint",
      "required": false
    }
  ]
}
```

### Auto-Detection at Init Time

`arm init` checks for well-known files and proposes defaults:

| Detected File | Proposed test_cmd | Proposed lint_cmd |
|---|---|---|
| `package.json` | `npm test` | `npx eslint {scope}` |
| `go.mod` | `go test ./...` | `golangci-lint run {scope}` |
| `pyproject.toml` | `pytest` | `ruff check {scope}` |
| `Cargo.toml` | `cargo test` | `cargo clippy -- {scope}` |
| `Makefile` (with test target) | `make test` | (none) |

The developer confirms or overrides. If nothing is detected, the CLI prompts for manual entry.

### `{scope}` Interpolation

Replaced at runtime with the task's `scope.modify` paths (space-separated for CLI args, or glob patterns depending on the tool). This is the bridge between the ops data model and the code verification tools.

### Required vs Optional Hooks

Required hooks block the transition — if they fail, the CLI refuses to emit the `done` op. Optional hooks report results but do not block. This distinction lets teams include aspirational checks (lint) without gating on them.

### Exit Code Semantics

| Exit Code | Meaning | Worker Action |
|---|---|---|
| `0` | Pass | Proceed with transition |
| `1` | Test/lint failure (actionable) | Fix code, re-run |
| Other | Environment error (not actionable) | Report issue, move on |

This distinction matters for AI agents: a test failure means "fix your code," an environment error means "report the issue and move on."

### Phase Separation

Verification is strictly two-phase: verify in code worktree, then record in ops worktree. The CLI discovers the code worktree by walking up from `cwd` to find the `.git` directory (standard git behavior). The developer runs `arm transition` from within their code worktree. The CLI runs hooks in `cwd`, then switches to the ops worktree internally to record the transition.

No cross-worktree operations occur within a single phase.

### Single-Branch Mode

In single-branch mode, there is no worktree distinction — hooks run in the same directory as ops. The config format is identical.

---

## 13. CLI Command Reference

### Command Taxonomy

| Category | Commands | Output | TUI? |
|---|---|---|---|
| Agent-invoked (scriptable) | `create`, `claim`, `transition`, `heartbeat`, `note`, `link`, `decision`, `reopen` | Plain one-line or JSON | Never |
| Read-only query | `ready`, `render-context`, `validate`, `status`, `metrics`, `context-history` | JSON (pipe) or Glamour (TTY) | Never |
| Human interactive | `dag-summary`, `stale-review` | Interactive TUI | Always |
| Workflow | `decompose-context`, `decompose-apply`, `decompose-revert`, `import`, `confirm` | JSON | Never |
| Infrastructure | `init`, `sync`, `merged`, `sources add/sync/verify`, `worker-init` | Plain | Never |

**Hard rule:** Agent-invoked commands never require a TUI. Human review commands always get interactive treatment.

### Worktree Targeting

| Commands | Reads | Writes |
|---|---|---|
| `claim`, `transition`, `heartbeat`, `note`, `create`, `link`, `decision`, `reopen`, `decompose-apply`, `decompose-revert`, `merged`, `confirm` | ops worktree | ops worktree |
| `ready`, `render-context`, `validate`, `status`, `metrics`, `context-history`, `dag-summary`, `stale-review` | ops worktree | (none) |
| `transition` (with verification hooks) | ops worktree + code worktree | ops worktree |
| `init`, `worker-init` | both | both (setup) |
| `sources add/sync/verify` | ops worktree + external providers | ops worktree |
| `sync` | ops worktree | (none, unless `--code`) |

### Key Command Specifications

#### `arm sync`

```
arm sync [flags]

Behavior:
  1. Pull ops worktree (_armature branch)
  2. Incremental materialize (process new ops since checkpoint)
  3. Report summary of changes

Flags:
  --quiet          Suppress output (for scripting)
  --code           Also pull the code branch (convenience wrapper)
  --check          Report sync status without pulling (is local behind remote?)

Output:
  synced: 3 new ops from 2 workers
  materialized: 1 task claimed, 1 task merged, 0 new tasks

Exit codes:
  0  success (or already current)
  1  ops branch not found (run arm init)
  2  network error (offline — local state is stale)
```

Implicit in all commands. Explicit `arm sync` exists for diagnostics, scripting, and batch operations.

#### `arm init`

```
arm init [flags]

Behavior:
  1. Detect branch protection (can push to main directly?)
  2. If protected: create _armature orphan branch (if needed), set up ops worktree
  3. If not protected: use single-branch mode
  4. Auto-detect project type, propose verification hooks
  5. Write .armature/config.json
  6. Install git hooks (unless --no-hooks)
  7. Run worker-init (generate UUID, store in git config)

Flags:
  --no-hooks       Skip hook installation
  --repair         Re-create worktree, re-install hooks
  --single-branch  Force single-branch mode regardless of protection

Output:
  Detected: package.json (Node.js)
  Proposed test command: npm test
  Proposed lint command: npx eslint {scope}
  Accept? [Y/n/edit]

  ✓ Config written to .armature/config.json
  ✓ Hooks installed to .git/hooks/
  ✓ Worker ID: a1b2c3d4
  ✓ Ops worktree created at .arm/
```

#### `arm merged`

```
arm merged <issue-id>

Behavior:
  Manually promotes a 'done' task to 'merged' status.
  Emits a transition op with {"to": "merged"}.
  Used as fallback when auto-detection cannot confirm merge.

Exit codes:
  0  success
  1  issue not found or not in 'done' status
```

#### `arm reopen`

```
arm reopen <issue-id> [flags]

Flags:
  --outcome <string>   Reason for reopening (required)

Behavior:
  Transitions a 'done' or 'blocked' task back to 'open' status.
  Emits a transition op with {"to": "open"}.
  The previous outcome is preserved in the materialized node's
  'prior_outcomes' array, providing context for the next worker.
  Used when a PR is rejected or work is found to be insufficient.

Exit codes:
  0  success
  1  issue not found or not in 'done'/'blocked' status
  2  issue is in 'merged' status (terminal, cannot reopen)
```

---

## 14. Worker Identity and Lifecycle

### Worker-Init

`arm worker-init` (also run as part of `arm init`) generates a UUID and writes it to repo-local git config:

```
git config --local armature.worker-id <uuid>
```

The CLI refuses to operate without a configured worker ID. First push validates uniqueness against existing log filenames in the repo — if a collision is detected (vanishingly unlikely with UUID), the worker is prompted to regenerate.

### One Worktree Per Worker

This is a hard requirement, not a recommendation. Each worker (human or AI) must have its own git worktree. Workers on separate machines each have their own worktree and their own UUID.

### Ops Worktree Setup

Part of `worker-init`. The CLI creates the ops worktree via `git worktree add`, configures sparse checkout, and stores the ops worktree path in git config:

```
git config --local armature.ops-worktree-path .arm/
```

### Multi-Machine Workers

Workers on separate machines get separate worktrees and separate UUIDs. They share the ops branch via the remote. This is the standard distributed git model — no special handling needed.

---

## 15. TUI Specifications

### Charm Ecosystem

| Library | Role |
|---|---|
| Bubble Tea | Event loop, keyboard input, terminal resize, async operations |
| Lip Gloss | Declarative styling (color, bold, border, padding) |
| Glamour | Markdown rendering with syntax highlighting |
| Bubbles | Prebuilt components (spinner, progress bar, table, viewport) |

Cross-platform: Windows Terminal, iTerm2, xterm.

### Semantic Color Palette

```go
Critical        = bold + red(196)       // blocking, must act
Warning         = bold + orange(214)    // needs attention
Advisory        = yellow(226)           // informational flag
OK              = green(82)             // confirmed good
Info            = blue(39)              // neutral information
Muted           = gray(241)             // secondary content
ActionRequired  = bold + white on red   // explicit operator action box
```

### TUI vs Plain Output

| Command | Mode | Reason |
|---|---|---|
| `dag-summary` | Interactive TUI | Sign-off requires per-item interaction |
| `stale-review` | Interactive TUI | Per-node decision flow |
| `ready` | Interactive TUI (human) / JSON (agent) | Claim action follows selection |
| `render-context` | Plain (Glamour) | Read-only, pipeable |
| `sources sync` | Plain + spinner | Progress indication, no interaction needed |
| `validate` | Plain structured | CI-friendly, machine-parseable |
| Agent-invoked commands | Plain one-line | Must be scriptable |

### `dag-summary` Interactive Checklist

The sign-off checklist is interactive — approval is mechanically impossible without completing all checklist items. Each check generates a log op with the worker ID. This creates attribution without adding process friction beyond the review itself. The TUI displays: node title, source citation, coverage status, and a per-item confirm/reject prompt.

### `stale-review` Per-Node Flow

Displays per affected node: the source change summary (section-level diff from provider version history where available), the node's citation, and the full `arm render-context` output. Options per node: confirm citation valid, flag for re-decomposition, skip. Each response is a log op.

### `render-context` Human Output

Glamour-rendered markdown with the semantic color palette. See section 6 for the full output format specification and color mapping.

### `ready` Task Selection

In human-interactive mode, displays the ready task queue with priority sort, estimated complexity, and scope summary. The operator can select a task and immediately claim it. In agent mode (`--format=json`), outputs the raw ready queue for programmatic consumption.

---

## 16. SKILL.md Contract (AI Worker Interface)

The SKILL.md is the machine-readable interface for AI workers. The CLI abstracts all worktree management — the agent never knows about dual branches, ops worktrees, or materialization.

### Setup (Once Per Session)

```
$ arm version                    # verify arm is available
$ arm worker-init --check        # verify worker identity
# If no worker ID: $ arm worker-init
```

### Work Loop

```
Repeat until no ready tasks or instructed to stop:

Step 1: Find Work
  $ arm ready --format=json
  Parse the ready task list. Select the highest-priority task.
  If no tasks are ready, stop.

Step 2: Get Context
  $ arm render-context <issue-id> --format=agent
  Read the full JSON output. This is the complete task specification.

Step 3: Claim
  $ arm claim <issue-id> --ttl=60
  The CLI handles post-claim verification internally.
  Check the output to confirm you won the claim. If not, return to Step 1.

Step 4: Execute
  Perform the work described in the task context.
  While working, emit heartbeats:
  $ arm heartbeat <issue-id>

  If you discover important context:
  $ arm note <issue-id> "Discovered X, handled by Y"

  If you encounter an undocumented decision point:
  $ arm decision <issue-id> --topic="<topic>" --choice="<choice>" --rationale="<why>" --affects="<scope-glob>"

Step 5: Verify Acceptance
  Before transitioning, verify each acceptance criterion:
  - test_passes: Run specified tests, confirm pass
  - function_exists: Confirm function is defined
  - no_regression: Run test suite, confirm no failures
  - lint_clean: Run linter on scope, confirm clean

Step 6: Complete (ops)
  $ arm transition <issue-id> done --outcome="<one-line summary>"
  The outcome MUST be a single sentence describing the concrete deliverable.
  Good: "Added refreshToken() in src/auth/token.ts with 60s expiry and rotation"
  Bad:  "Completed the task"

Step 7: Push Code (separate from ops)
  $ git add -A
  $ git commit -m "Complete <issue-id>: <title>"
  $ git push origin <feature-branch>
  Open PR against main.

Return to Step 1.
```

### Error Recovery

```
Cannot complete a task:
  $ arm transition <issue-id> blocked --outcome="<why blocked>"
  Return to Step 1.

Wrong task claimed:
  $ arm transition <issue-id> open --outcome="Unclaimed: <reason>"
  Return to Step 1.

PR rejected / work insufficient:
  $ arm reopen <issue-id> --outcome="PR rejected: <reason>"
  The task returns to 'open' status with prior outcomes preserved.
  Return to Step 1.
```

### Rules and Constraints

- Never modify files outside the task's `scope.modify`
- Never skip acceptance criteria verification
- Always include a meaningful `--outcome` when transitioning
- Emit heartbeats regularly (at least every 30 minutes of work)
- If `budget.status` shows `"over_budget"`, focus only on core spec and blocker outcomes
- Do not read files outside `scope.modify` and `scope.read` unless strictly necessary

---

## 17. Error Diagnostics

### Structured Error Format

Errors must be self-diagnosable by both humans and AI agents:

```
error: issue abc-123 is already claimed by worker-b2
  claimed_at: 2025-02-28T10:30:00Z
  last_heartbeat: 2025-02-28T10:45:00Z
  ttl_minutes: 60
  expires_at: 2025-02-28T11:30:00Z
hint: wait for claim to expire or use --force to override
```

Every error includes: the error message, relevant state (timestamps, IDs, statuses), and a hint suggesting the next action.

### `--debug` Flag

Dumps internal state: materialized issue, raw log entries, git status, ops worktree status, checkpoint state. Available on all commands.

### Ops-Worktree-Specific Errors

| Error | Cause | Hint |
|---|---|---|
| `ops branch not found` | `_armature` branch missing | `run arm init` |
| `ops worktree desync` | Local ops worktree is behind or corrupted | `run arm sync` or `arm init --repair` |
| `stale worktree` | Worktree path exists but points to wrong branch | `run arm init --repair` |
| `materialization failed` | Corrupt log line or unexpected state | Skip unparseable lines + warn; `--debug` shows details |

---

## 18. Failure Modes and Mitigations

### Consolidated Failure Table

| Failure | Impact | Mitigation |
|---|---|---|
| Worker crashes after claim, before completion | Issue stuck as claimed | Heartbeat + TTL expiry; other workers reclaim after TTL |
| Worker crashes after append, before push | Op lost locally; shared state consistent | No mitigation needed — inherently safe, worker re-issues on restart |
| Push rejected (non-fast-forward) | Temporary delay | Retry loop with `pull --rebase`, cap at ~5 |
| Corrupt log line | Materialization fails on one line | Skip unparseable lines + warn (implemented in parser) |
| Clock skew between workers | Wrong claim winner | NTP keeps skew <1s; ms timestamps make races negligible |
| Duplicate worker IDs | Real merge conflicts | UUID generation + uniqueness validation on first push |
| Log grows unbounded | Slow materialization | Incremental materialization + periodic compaction (deferred) |
| Transition-code desync (done before code reviewed) | Downstream premature start | Two-phase done/merged model; downstream requires `merged` |
| Ops branch force-push or deletion | Loss of coordination state | Configure force-push protection separately from PR requirements; local worktrees retain full history for recovery |
| Squash-merge breaks commit ancestry checks | Merge detection miss | Commit-message scan (not ancestry) as primary detection; branch-name and explicit fallbacks |
| Worktree setup failure during worker-init | Worker cannot operate | CLI creates ops branch from orphan if missing; `--repair` flag for stale state |
| Verify phase reads code worktree, record phase writes ops worktree | Cross-worktree corruption | Strict phase separation: verify(code) then record(ops); no cross-worktree operations within a phase |
| Task stuck at done if PR abandoned | Downstream permanently blocked | Staleness check (no merge within N days of done); surfaced via `arm status` |
| Manual commits to ops branch | Unexpected state | CLI ignores non-.armature/ files; contributing guide documents convention |
| Runaway agent emitting excessive ops | Materialization slowdown for all workers | CLI-side rate limiter: heartbeats capped at 1/min/issue, notes at 10/issue/session, creates at 500/commit. Materialization logs per-worker processing time; warns if single worker contributes >80% |
| Worker impersonation (ops in wrong log file) | Incorrect state attribution | Filename-worker-ID validation during materialization; mismatched ops excluded with warning (section 3) |
| Conflicting decisions on same topic | Agents receive contradictory guidance | Last-write-wins by timestamp; W8 validation warning; surfaced in dag-summary for human resolution (section 3) |
| `decompose-apply` partial push (network timeout) | Potential duplicate nodes on re-run | Batch ID (UUID) recorded in each op's payload; re-run detects existing batch by plan file hash; requires `--force` to proceed |

### Offline Mode

Workers without network access can continue appending ops to their local ops worktree. They cannot push. When network is restored, they push accumulated ops normally. No special handling is required.

**Stale claims:** A worker claims a task while offline. Another online worker claims the same task. When the offline worker pushes, both claims land in the log. Normal timestamp-based resolution applies — first claim by timestamp wins. The offline worker's clock accuracy determines whether they win.

**Stale done transitions:** A worker transitions a task to `done` while offline. When they push, the `done` status propagates. The two-phase model handles this correctly — `done` does not unblock downstream work. No correctness issue.

**Stale heartbeats:** Heartbeats emitted offline have old timestamps. They do not revive an expired claim. The reclaiming worker's claim takes precedence.

The only cost is potential duplicated effort if another worker reclaims a task during the offline period. This is acceptable and consistent with the design philosophy of advisory coordination over strict locking.

---

## 19. Time Travel and Forensics

The append-only JSONL-in-git architecture enables forensic capabilities as emergent properties, not bolted-on features.

### Forensic Reconstruction

```
arm render-context <issue-id> --at <sha>
```

Reconstructs the exact context a worker would have received at a specific ops branch commit. Uses the ops branch commit SHA, not the code branch. This enables post-incident analysis: "what did the agent see when it made that decision?"

### Context Drift Tracking

```
arm context-history <issue-id>
```

Shows how a task's rendered context changed over time: which ops modified the context, when, and by which worker. Useful for understanding why an agent's output diverged from expectations.

### Selective Replay

```
arm materialize --exclude-worker <worker-id>
```

Rebuilds state excluding one worker's ops. Useful for diagnosing whether a specific worker introduced corruption or inconsistency.

### Bisect

```
arm validate
```

Run `arm validate` at different ops branch commits using `git bisect` to find when a structural error (cycle, orphan link, missing field) was introduced.

### Sprint Bookmarks

Git tags on the ops branch mark sprint boundaries. Example: `git tag sprint-12 _armature` at sprint close. Enables reconstructing state at any sprint boundary.

### Code-Ops Correlation

The transition op records `branch` and `pr` fields, bridging the ops history (on `_armature`) with the code history (on `main` and feature branches). Given an ops commit, you can find the corresponding code PR. Given a code commit, the issue ID in the commit message (from the `prepare-commit-msg` hook) links back to the ops.

---

## 20. Configuration

### Configuration Layers

| Layer | Location | Scope | Contents |
|---|---|---|---|
| Repository | `.armature/config.json` (on ops branch) | Shared by all workers | Templates, pre-transition hooks, token budget, TTL defaults, staleness thresholds |
| Worker | Repo-local git config | Per-worktree | `armature.worker-id`, `armature.ops-worktree-path` |
| User | `~/.config/arm/providers.toml` | Per-machine | Provider credentials (Confluence, SharePoint auth) |
| Environment | Environment variables | Per-session | Provider tokens (e.g., `CONFLUENCE_PAT`) |

### Repository Configuration Schema

```json
{
  "version": 1,
  "token_budget": 1600,
  "claim_ttl_default": 60,
  "staleness_threshold_days": 7,
  "max_dod_length": 200,
  "max_description_length": 500,
  "required_acceptance_types": ["test_passes", "no_regression"],
  "pre_transition_hooks": [
    {
      "cmd": "npm test -- --testPathPattern={scope}",
      "label": "tests",
      "required": true,
      "exit_codes": {"0": "pass", "1": "test_failure", "*": "environment_error"}
    }
  ],
  "templates": {
    "task": ".armature/templates/task.json",
    "story": ".armature/templates/story.json",
    "epic": ".armature/templates/epic.json"
  }
}
```

### Worker Configuration

```
git config --local armature.worker-id a1b2c3d4-5678-90ab-cdef-1234567890ab
git config --local armature.ops-worktree-path .arm/
```

### User Configuration

```toml
# ~/.config/arm/providers.toml
[confluence]
base_url = "https://acme.atlassian.net/wiki"
auth_method = "pat"
token_env = "CONFLUENCE_PAT"

[sharepoint]
tenant_id = "..."
client_id = "..."
auth_method = "device_flow"
```

---

## 21. Deployment Topologies and Personas

### Persona-Driven Feature Matrix

| Feature | Solo Freelance | Solo Enterprise | Team (Monorepo) | Team (Multi-Repo) |
|---|---|---|---|---|
| Branch mode | Single-branch | Dual-branch | Dual-branch | Dual-branch |
| Ops location | `.armature/` on main | `_armature` branch | `_armature` branch | Hub repo (future) |
| Claim races | N/A (one worker) | N/A (one worker) | Full MRDT | Full MRDT |
| Two-phase completion | Optional (no PR gate) | Yes | Yes | Yes |
| Merge detection | Immediate (direct push) | Commit-message scan | Commit-message scan | Cross-repo scan (future) |
| Cross-repo deps | N/A | N/A | N/A (monorepo) | Manual (v1), Hub (future) |
| Hooks | Optional | Recommended | Recommended | Required (hub config) |
| Workers per repo | 1 | 1 | Many | Many across repos |

### Multi-Repo Strategy

**v1: Separate instances per repo (Option A).** Each repo has its own `arm init`, its own ops branch, its own DAG. Cross-repo dependencies are tracked manually (notes on tasks). The CLI has no awareness of other repos.

**Future: Hub-repo topology (Option B, designed-for but not implemented).** A dedicated coordination repository (e.g., `acme/armature-ops`) contains all armature ops for the project. Individual code repos reference this hub. Workers in any code repo push ops to the hub.

Implementation path: `arm init --ops-repo=acme/armature-ops` configures the ops worktree to point at the external hub repo. All CLI commands operate against the hub. The code worktree is the current repo.

**Design-for signals in v1:**

1. Task scope includes optional `repo` field (schema addition only)
2. Ops worktree path is configurable (supports external repo)
3. Task IDs are globally unique UUIDs (no collision on merge)
4. Merge detection does not assume code and ops are in the same repo (configurable per-task code repo)

**When to implement:** When a paying customer or significant user cohort requests cross-repo coordination.

---

## 22. Distribution and Packaging

### Binary

Single Go binary, `CGO_ENABLED=0`. Static compilation, zero runtime dependencies beyond `git`.

**Platform matrix:**

| OS | Architecture |
|---|---|
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| Windows | amd64, arm64 |

Binary size estimate: 8–12MB (4–6MB compressed).

### Git Dependency

Minimum git version: 2.25+ (required for sparse checkout support, released January 2020). The CLI shells out via `os/exec` rather than using a pure-Go git library — shelling out is more debuggable and avoids a large dependency.

### Skill Distribution

The tool is distributed as three artifacts:

1. **Binary** — GitHub releases, per-platform
2. **SKILL.md** — Natural language description of CLI commands, behavior, and integration patterns. The machine-readable skill interface for AI workers.
3. **schema.json** — Structured tool definitions for AI platforms that support them (function calling schemas, parameter types, return types).

The skill interface is defined by the CLI contract (commands, arguments, JSON output), not the implementation language. This makes it portable across Claude Code, Gemini CLI, Cursor, Windsurf, Kiro, and any future tool that can invoke a subprocess and parse JSON.

---

## Appendix A: Decisions That Are Final

These decisions are locked and should not be revisited without significant new evidence.

- Dual-branch architecture: `_armature` (unprotected, orphan) + `main` (protected, code)
- Single-branch fallback when main is not protected
- Ops worktree via `git worktree add` with sparse checkout
- All ops route through worker's own log file (no shared log files)
- Worker UUID in repo-local git config
- Two-phase completion: `done` (self-reported) then `merged` (auto-detected)
- Downstream unblocking requires `merged`, not `done`
- Commit-message scan as primary merge detection method
- `dag-summary` interactive sign-off as DAG governance gate (not PR on ops branch)
- PR/MR as the governance gate for code changes
- Git hooks for heartbeat, merge promotion, commit-message stamping — convenience only
- Committed source cache, workers never fetch live documents
- Charm (Bubble Tea + Lip Gloss + Glamour) for TUI
- Free-text structured citation (not tagged source doc annotation) for v1
- File-level (not section-level) fingerprinting for v1
- `state/` directory is materialized output, local-only (never committed to ops branch), never source of truth
- Git hooks are convenience; pre-transition verification hooks configured as `required` in config.json are enforcement
- Decision conflicts resolved by last-write-wins (timestamp), with W8 validation warning
- Compaction deferred until materialization is measured slow
- Adversarial decomposition deferred, human gate is sufficient
- Language: Go. Distribution: single static binary.
- Token proxy: `chars/4`, advisory only
- `decompose-revert` via cancellation ops (double-entry), not history rewriting
- `arm doctor` = structural pre-work gate (D1–D6, field presence only); `arm validate` = semantic pre-push gate (citation UUID integrity, scope overlap, coverage). They are distinct commands with distinct invocation points — do not merge. D6 in doctor intentionally checks only field presence; source UUID validation belongs in validate (E8).

## Appendix B: Scope Boundaries

| Concern | Decision |
|---|---|
| Malicious actors / tampering | Out of scope. Delegated to git host ACL + protected branches + PR/MR approval. |
| Semantic correctness of decomposition | Out of scope. Developer is accountable at sign-off gate. |
| Context budget enforcement | Advisory only. Hard gates rejected. |
| Compaction | Deferred. Optimize when materialization performance is measured. |
| Adversarial dual-agent decomposition | Deferred. Human gate serves same purpose at lower cost. |

## Appendix C: Residual Risk Assessment

| # | Finding | Severity | Rationale for Acceptance |
|---|---|---|---|
| L1 | Token proxy `chars/4` is inaccurate for code-heavy or non-English content | Low | Advisory only; real tokens not available without a tokenizer binary. |
| L2 | Sibling scope overlap detection uses glob matching, which is approximate for complex patterns | Low | Go's `filepath.Match` handles standard globs. Exotic patterns not supported. |
| L3 | Plan file schema is not formally specified as JSON Schema in v1 | Low | Documented by example. Formal schema added when a second consumer needs it. |
| L4 | `decompose-context` prompt template quality is load-bearing | Low | Template ships with tested defaults and is overridable. `dag-summary` gate catches structural issues. |
| L5 | Brownfield `import` format support (Jira, Linear) requires per-provider parsers | Low | CSV/JSON are baseline. Jira/Linear parsers added on demand. |
| L6 | `decompose-revert` emitting cancellation ops adds noise to the log | Low | Preferable to `git reset --hard` which loses audit trail. Noise is bounded (one op per reverted node). |
| L7 | Advisory budget truncation order may remove a critical decision | Low | Decisions are priority 3 (above notes and siblings). Core spec and snippets never truncated. `--raw` bypasses all truncation. |
| L8 | Cold-start materialization on fresh clone requires full log replay | Low | Under 1 second for 10,000 ops at Go speeds. Compaction (deferred) addresses long-term growth. Progress indicator shown. |
| L9 | Worker impersonation via spoofed log filename (new file with fake UUID) | Low | Requires repo push access (git host ACL is primary control). Optional worker-registry.json cross-references git committer identity. |
| L10 | Scope path validation (W10) cannot distinguish "create new file" from "typo in path" | Low | Warning only. Decomposition reviewers verify scope correctness at `dag-summary` gate. |
| L11 | Claim-time scope overlap check uses glob matching, which is approximate | Low | Same as L2. Advisory only — warns but does not block. |
| L12 | Merge detection coverage degrades silently without `prepare-commit-msg` hook | Low | W9 validation warning monitors coverage. `arm validate` surfaces low coverage rate. Fallback detection methods remain available. |