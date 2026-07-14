# Armature Surface Census

## Overview

This document inventories the actual surfaces of the armature system: issue types, statuses, confidence states, all fields on issue structs, all CLI commands and their flags. Each surface entry cites where it's defined and used in the corpus, and rules it as **kept-evidence** (actively used, has real callers/tests), **kept-justified** (exists for a documented reason even if lightly used), or **parked** (no evidence of use — includes a re-entry criterion).

## Issue Types

| Type | Defined | Used By | Status | Notes |
|------|---------|---------|--------|-------|
| `epic` | internal/issuetype/issuetype.go:61 | hierarchy, all issue creation flows, ready queue filter (list.go:39) | **kept-evidence** | Root type for hierarchical decomposition. Actively created and used as parent in decompose flows. |
| `story` | internal/issuetype/issuetype.go:62 | hierarchy, ready queue eligibility (issuetype.go:82), claim flow | **kept-evidence** | Primary aggregation unit for task grouping. Eligible for ready queue and claim operations. |
| `feature` | internal/issuetype/issuetype.go:63 | hierarchy, ready queue eligibility, decompose plans | **kept-evidence** | Intermediate level between story and task. Explicitly documented in hierarchy as new permitted type (issuetype.go:20-22). |
| `task` | internal/issuetype/issuetype.go:64 | primary work unit, ready queue (issuetype.go:82), claim operations, tests | **kept-evidence** | Default type for new issues (create.go:139). Fundamental unit for worker claims and transitions. |
| `bug` | internal/issuetype/issuetype.go:65 | hierarchy leaf, ready queue eligibility, tests | **kept-evidence** | Leaf issue type. Eligible for ready queue. Tested in bootstrap and claim flows. |

## Issue Statuses

| Status | Defined | Used By | Status | Notes |
|--------|---------|---------|--------|-------|
| `open` | internal/ops/types.go:37 | ValidTransitionTargets (ops.go:47), initial state, list filter (list.go:39), tests | **kept-evidence** | Default initial status for created issues. Primary state for unstarted work. |
| `claimed` | internal/ops/types.go:38 | ValidTransitionTargets, materialize transitions, tests | **kept-evidence** | Set by claim operation. Transitional state between open and in-progress. |
| `in-progress` | internal/ops/types.go:39 | ValidTransitionTargets (ops.go:48), list filter (list.go:39), tests | **kept-evidence** | Set by claim (claim.go) or transition command. Core working state. |
| `done` | internal/ops/types.go:40 | ValidTransitionTargets, transition command target, merged flow, tests | **kept-evidence** | Terminal status for completed work. Target of transition --to (transition.go). |
| `merged` | internal/ops/types.go:41 | ValidTransitionTargets, sync/merged operations, tests | **kept-evidence** | Status after PR merge. Transitioned by merged command and harness hook. |
| `blocked` | internal/ops/types.go:42 | ValidTransitionTargets, transition target option, tests | **kept-evidence** | Indicates external dependency. Set via transition --to blocked. |
| `cancelled` | internal/ops/types.go:43 | ValidTransitionTargets, terminal filter (list.go:42), tests | **kept-evidence** | Terminal status for abandoned work. Accessible via transition command. |

## Confidence States

| State | Defined | Used By | Status | Notes |
|-------|---------|---------|--------|-------|
| `draft` | internal/ops/types.go:127 (comment), create.go:62, Payload.Confidence | Confidence field in create payload, dag-transition checks (claim.go:311), tests | **kept-evidence** | Set via --confidence flag on create. Blocks claiming (claim.go:311-312). Used by dag-transition flow for confidence promotion. |
| `verified` | internal/ops/types.go:127 (comment: default), materialize assumptions, tests | Default when confidence absent, claim eligibility, tests | **kept-evidence** | Implicit default for issues created without explicit confidence. Claim-eligible state. |
| `inferred` | claim.go:311 (check) | Claim rejection logic | **kept-evidence** | Provenance confidence value that blocks claiming. Set during materialization when reconstructing inferred nodes. |

## Issue Fields

The following fields appear on the materialized Issue struct (internal/materialize/state.go:13-49):

| Field | Type | Defined | Set By Op | Status | Notes |
|-------|------|---------|-----------|--------|-------|
| `id` | string | state.go:14 | create | **kept-evidence** | Primary key. Set at creation, immutable. |
| `type` | string | state.go:15 | create | **kept-evidence** | Issue type (epic, story, feature, task, bug). Set at creation, amendable via amend. |
| `status` | string | state.go:16 | create (open), transition, claim | **kept-evidence** | Current lifecycle state. Primary index for queries. |
| `title` | string | state.go:17 | create | **kept-evidence** | Human-readable label. Set at creation. |
| `parent` | string | state.go:18 | create, reparent | **kept-evidence** | Parent issue ID for hierarchy. Amendable. |
| `children` | []string | state.go:19 | derived from parent links | **kept-evidence** | Materialized inverse of parent. Read-only. |
| `blocked_by` | []string | state.go:20 | link op (rel=blocked_by) | **kept-evidence** | Issues that must complete first. Used by ready queue logic. |
| `blocks` | []string | state.go:21 | link op (rel=blocks) | **kept-evidence** | Issues blocked by this one. Inverse of blocked_by. |
| `assignee` | string | state.go:22 | assign command | **kept-evidence** | Human assignee name/email. Informational. |
| `priority` | string | state.go:23 | create payload, amend | **kept-evidence** | Priority level (critical, high, medium, low). Set via --priority flag. Used in diagnostics. |
| `estimated_complexity` | string | state.go:24 | create payload | **kept-evidence** | Complexity estimate. Set via --complexity (if exposed) or inferred from acceptance criteria. |
| `definition_of_done` | string | state.go:25 | create, amend | **kept-evidence** | Completion criteria. Required for task type (issuetype.go:90). |
| `scope` | []string | state.go:26 | create, amend | **kept-evidence** | File scope globs. Used by overlap detection and scope-rename/delete commands. |
| `context_files` | []string | state.go:27 | create, amend | **kept-evidence** | Stable reference files. Set via --context-file flag. Used by harness hook to render context. |
| `acceptance` | json.RawMessage | state.go:28 | create, amend (acceptance JSON) | **kept-evidence** | Acceptance criteria as opaque JSON. Set via --acceptance flag on create. |
| `context` | json.RawMessage | state.go:29 | implicit from context_files or explicit via decompose | **kept-evidence** | Rendered context blob. Set by harness or decompose flow. |
| `source_citation` | json.RawMessage | state.go:30 | source-link ops (transitive) | **kept-evidence** | Citation metadata. Used by sources and citation acceptance flows. |
| `provenance` | Provenance | state.go:31 | create (method, confidence, worker) | **kept-evidence** | Metadata: method (create/decompose/import), confidence, source worker, dag_confirmed flag. Used to detect inferred nodes and confidence state. |
| `decision_refs` | []string | state.go:32 | decision op (derives refs) | **kept-evidence** | Decision operation IDs affecting this issue. Read-only. |
| `outcome` | string | state.go:33 | transition op (to=done) | **kept-evidence** | Outcome summary on completion. Set by transition command with --outcome. |
| `prior_outcomes` | []string | state.go:34 | appended when reopening | **kept-evidence** | History of previous outcomes. Accumulated on reopen. |
| `notes` | []Note | state.go:35 | note op | **kept-evidence** | Worker annotations. Set by note command. |
| `decisions` | []Decision | state.go:36 | decision op | **kept-evidence** | Structured decisions affecting scope. Set by decision command. |
| `source_links` | []SourceLink | state.go:37 | source-link op | **kept-evidence** | External source references. Set by source-link or create --source. |
| `citation_acceptances` | []CitationAcceptance | state.go:38 | citation-accepted op | **kept-evidence** | Record of accepted source citations. Set by accept-citation command. |
| `assessment_attestations` | []review.AssessmentAttestation | state.go:39 | assessment-attested op (review.go) | **kept-evidence** | Code review assessment records. Set by review attest flow. |
| `claimed_by` | string | state.go:40 | claim op | **kept-evidence** | Worker ID that holds the claim. Set by claim command. |
| `claimed_at` | int64 | state.go:41 | claim op | **kept-evidence** | Claim timestamp (epoch ms). Used to calculate staleness. |
| `claim_ttl` | int | state.go:42 | claim op (TTL in minutes) | **kept-evidence** | Claim time-to-live. Set by --ttl flag (default 60 min). Used by stale review logic. |
| `last_heartbeat` | int64 | state.go:43 | heartbeat op | **kept-evidence** | Last heartbeat timestamp. Refreshed by heartbeat command to prevent staleness. |
| `branch` | string | state.go:44 | transition op (branch field) | **kept-evidence** | Feature branch name. Set by transition on completion. Used by merged flow. |
| `pr` | string | state.go:45 | transition op (pr field), merged op | **kept-evidence** | PR number or URL. Set by transition or merged command. |
| `assigned_worker` | string | state.go:46 | assign op (assigned_to) | **kept-evidence** | Worker assigned for work (distinct from claim). Set by assign command. |
| `preferred_model` | string | state.go:47 | create (preferred_model) | **kept-evidence** | LLM model hint for the assigned agent. Set via --preferred-model at creation. |
| `updated` | int64 | state.go:48 | every op | **kept-evidence** | Last modified timestamp (epoch ms). Set to op timestamp for every state change. |

## Operation Types (OpTypes)

The following op types are defined in internal/ops/types.go and materialized by handlers in internal/materialize/engine.go:

| Op Type | Defined | Handler | Status | Notes |
|---------|---------|---------|--------|-------|
| `create` | ops.go:8 | engine.go | **kept-evidence** | Creates new issue. Emits create payload with type, title, parent, scope, etc. |
| `claim` | ops.go:9 | engine.go | **kept-evidence** | Assigns issue to worker with TTL. Sets claimed_by, claimed_at, claim_ttl. |
| `heartbeat` | ops.go:10 | engine.go | **kept-evidence** | Refreshes claim TTL. Updates last_heartbeat timestamp. |
| `transition` | ops.go:11 | engine.go | **kept-evidence** | Changes issue status. Payload: to (status), outcome, branch, pr. |
| `note` | ops.go:12 | engine.go | **kept-evidence** | Adds worker note. Payload: msg, note_id for deletion. |
| `note-delete` | ops.go:13 | engine.go | **kept-evidence** | Soft-deletes note by ID. Marks note.deleted=true. |
| `link` | ops.go:14 | engine.go | **kept-evidence** | Adds dependency. Payload: dep (target), rel (relationship type). Supports rel=blocked_by, rel=blocks. |
| `unlink` | ops.go:15 | engine.go | **kept-evidence** | Removes dependency. Payload: dep (target). |
| `source-link` | ops.go:16 | engine.go | **kept-evidence** | Links external source. Payload: source_id, source_url. Creates source_links entries. |
| `source-fingerprint` | ops.go:17 | engine.go | **kept-evidence** | Records source version. Payload: sha, version_id, provider. Used for staleness detection. |
| `dag-transition` | ops.go:18 | engine.go | **kept-evidence** | Promotes confidence (draft→verified). Payload: issue_id, confirmed, confirmed_noninteractively. Sets dag_confirmed flag. |
| `decision` | ops.go:19 | engine.go | **kept-evidence** | Records structured decision. Payload: topic, choice, rationale, affects (scope globs). |
| `assign` | ops.go:20 | engine.go | **kept-evidence** | Assigns worker to issue. Payload: assigned_to (worker ID). |
| `amend` | ops.go:21 | engine.go | **kept-evidence** | Updates issue metadata. Payload: type, scope, context_files, dod, acceptance (partial updates). |
| `citation-accepted` | ops.go:22 | engine.go | **kept-evidence** | Records citation acceptance. Payload: source_entry_id, confirmed_noninteractively flag. |
| `scope-rename` | ops.go:23 | engine.go | **kept-evidence** | Renames scope glob. Payload: old_path, new_path. Updates scope entries. |
| `scope-delete` | ops.go:24 | engine.go | **kept-evidence** | Removes scope glob. Payload: deleted_path. Removes from scope array. |
| `reparent` | ops.go:27 | engine.go | **kept-evidence** | Moves issue to new parent. Payload: parent (new parent ID, can be empty for top-level). |
| `assessment-attested` | ops.go:30 | engine.go | **kept-evidence** | Records code review attestation. Payload: assessment (JSON blob). Used by review attest. |

## CLI Commands

All commands are defined in cmd/armature/main.go (newRootCmd function, lines 19-270). Grouped below by category:

### Workflow Commands (workflow group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `ready` | main.go:90, ready.go | List ready-to-claim tasks | **kept-evidence** | Core workflow. Queries open tasks not blocked by dependencies. |
| `claim` | main.go:94, claim.go | Claim issue to worker with worktree | **kept-evidence** | Foundation of task assignment. Creates branch and worktree. |
| `transition` | main.go:98, transition.go | Change issue status | **kept-evidence** | Primary state machine driver. Sets outcome on done. |
| `unassign` | main.go:102 | Remove worker assignment | **kept-evidence** | Reverse of assign. Clears assigned_worker. |
| `reopen` | main.go:106, reopen.go | Transition done→open, preserve outcome | **kept-evidence** | Allows re-work of completed items. Moves outcome to prior_outcomes. |
| `heartbeat` | main.go:110, heartbeat.go | Refresh claim TTL | **kept-evidence** | Prevents staleness during long work. Called periodically by workers. |
| `note` | main.go:114, note.go | Add/delete worker note | **kept-evidence** | Progress tracking. Supports deletion by note_id. |
| `decision` | main.go:118, decision.go | Record structured decision | **kept-evidence** | Couples scope changes with rationale. Affects field filters scope. |
| `amend` | main.go:122, amend.go | Update issue metadata | **kept-evidence** | Allows corrections after creation. Payload-driven updates. |
| `confirm` | main.go:126, confirm.go | Interactive confidence promotion | **kept-evidence** | DAG flow. Promotes draft→verified with human confirmation. |
| `assign` | main.go:130, assign.go | Assign worker to issue | **kept-evidence** | Decouples assignment from claiming. Sets assigned_worker. |

### DAG Commands (dag group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `dagsum` | main.go:135, dagsum.go | Summarize draft nodes | **kept-evidence** | Diagnostic and planning tool. Lists unconfirmed nodes in a subtree. |
| `dag-transition` | main.go:139, dag_transition.go | Promote confidence (low-level) | **kept-evidence** | Sets dag_confirmed flag. Underpins confidence workflow. |
| `decompose apply` | main.go:143, decompose.go | Create issues from plan JSON | **kept-evidence** | Bulk creation from structured plan. Validates hierarchy. |
| `decompose revert` | main.go:147, decompose.go | Remove issues created by plan | **kept-evidence** | Undo plan application. Validates that no children exist. |
| `decompose context` | main.go:151, decompose.go | Generate context for plan | **kept-evidence** | Agent-facing tool. Renders existing DAG and sources into plan context. |
| `link` | main.go:155, link.go | Add dependency | **kept-evidence** | Couples issues. Supports blocked_by and blocks relations. |
| `unlink` | main.go:159, unlink.go | Remove dependency | **kept-evidence** | Uncouples issues. Removes from blocked_by/blocks. |

### Sync Commands (sync group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `sync` | main.go:164, sync.go | Auto-transition closed PRs | **kept-evidence** | CI integration. Scans git for merged branches and transitions issues. |
| `push-ops` | main.go:168, push_ops.go | Push pending ops to _armature branch | **kept-evidence** | Publishes ops to VCS. Called before PR or manually. |
| `merged` | main.go:172, merged.go | Manually transition to merged | **kept-evidence** | Explicit merge record. Sets PR and branch fields. |
| `materialize` | main.go:176, materialize.go | Regenerate state from ops log | **kept-evidence** | Diagnostic/recovery. Rebuilds snapshot from scratch. |
| `import` | main.go:180, import.go | Import issues from external source | **kept-evidence** | Onboarding tool. Creates issues with source links. |
| `stale-review` | main.go:184, stalereview.go | Detect stale claims | **kept-evidence** | Hygiene/monitoring. Alerts on expired TTLs. |

### Admin Commands (admin group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `version` | main.go:78, version.go | Print arm version | **kept-evidence** | Diagnostic. |
| `worker-init` | main.go:82, worker_init.go | Initialize worker ID | **kept-evidence** | One-time setup. Stores UUID in git config. |
| `bootstrap` | main.go:86, bootstrap.go | Deploy harness hook to project | **kept-evidence** | Setup command. Installs pre-commit or post-merge hooks. |
| `create` | main.go:189, create.go | Create new issue | **kept-evidence** | Direct issue creation (not decompose-based). |
| `reparent` | main.go:193, reparent.go | Move issue to new parent | **kept-evidence** | Hierarchy adjustment. Payload: parent. |
| `validate` | main.go:197, validate.go | Check DAG health | **kept-evidence** | Linter. Runs citation and structure checks. |
| `render-context` | main.go:201, render_context.go | Render issue context | **kept-evidence** | Agent-facing. Truncates to token budget. |
| `log` | main.go:205, log.go | List ops log entries | **kept-evidence** | Audit/debugging. Supports filtering by issue/worker. |
| `workers` | main.go:209, workers.go | List active workers | **kept-evidence** | Diagnostic. Shows claimed issues per worker. |
| `sources` | main.go:213, sources.go | Manage source manifest | **kept-evidence** | Citation infrastructure. CRUD for source entries. |
| `source-link` | main.go:217, source_link.go | Link issue to source | **kept-evidence** | Citation. Creates source-link ops. |
| `accept-citation` | main.go:221, accept_citation.go | Accept source citation | **kept-evidence** | Citation workflow. Sets citation_acceptances. |
| `show` | main.go:225, show.go | Display issue details | **kept-evidence** | Query tool. Supports --field for extraction. |
| `list` | main.go:229, list.go | List issues | **kept-evidence** | Query tool. Supports filtering and grouping. |
| `scope-rename` | main.go:233, scope_rename.go | Rename scope glob | **kept-evidence** | Cleanup. Updates scope and decision affects. |
| `scope-delete` | main.go:237, scope_delete.go | Delete scope glob | **kept-evidence** | Cleanup. Removes from scope arrays. |
| `doctor` | main.go:241, doctor.go | Diagnose DAG health | **kept-evidence** | Validator. Checks for broken refs, cycles, orphans. |
| `completion` | main.go:245, cmd_completion.go | Bash/zsh completion | **kept-evidence** | Shell integration. Cobra-generated. |
| `hook` | main.go:249, hook.go | Manage harness hooks | **kept-evidence** | Configuration. Enable/disable/debug hooks. |
| `tui` | main.go:253, tui.go | TUI for issue navigation | **kept-evidence** | Interactive mode. Lists and filters issues. |
| `context-history` | main.go:257, context_history.go | Scan git history for context | **kept-evidence** | Diagnostic. Helps find stable reference commits. |
| `harness-hook` | main.go:261, harness_hook.go | Harness hook runner (internal) | **kept-evidence** | Internal. Runs on pre-commit and post-merge. |
| `review` | main.go:265, review.go | Record code review | **kept-evidence** | Review infrastructure. Attest and cite changes. |

## Command Flags

The following flags are defined across all commands. Grouped by usage pattern.

### Universal/Root Flags (from main.go:66-69)

| Flag | Type | Default | Usage | Status | Notes |
|------|------|---------|-------|--------|-------|
| `--debug` | bool | false | Dump stack traces on error | **kept-evidence** | Diagnostic. Always available. |
| `--format` | string | human | Output format: human, json, agent | **kept-evidence** | Auto-set to agent for non-TTY. |
| `--repo` | string | . | Repository path | **kept-evidence** | Allows multi-repo operation. |
| `--non-interactive` | bool | false | Skip TUI, use structured output | **kept-evidence** | Auto-set in CI. |

### Issue/Field Flags (common across commands)

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--issue` | claim, transition, note, decision, amend, show, assign, unassign, etc. | string | Target issue ID (positional alternative in some commands) | **kept-evidence** |
| `--type` | create, amend | string | Issue type (epic, story, feature, task, bug) | **kept-evidence** |
| `--parent` | create, reparent, list, ready | string | Parent issue ID (or filter) | **kept-evidence** |
| `--title` | create | string | Human-readable title | **kept-evidence** |
| `--scope` | create, amend, decision | string[] | File scope globs | **kept-evidence** |
| `--dod` | create, amend | string | Definition of done | **kept-evidence** |
| `--priority` | create | string | Priority level (critical, high, medium, low) | **kept-evidence** |
| `--acceptance` | create, amend | string | Acceptance criteria as JSON | **kept-evidence** |
| `--context-file` | create, amend | string[] | Stable reference files | **kept-evidence** |
| `--id` | create | string | Explicit issue ID (auto-generated if empty) | **kept-evidence** |
| `--confidence` | create | string | Confidence level (draft or verified) | **kept-evidence** |
| `--source` | create, import | string | Source ID/URL to link at creation | **kept-evidence** |

### Workflow Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--ttl` | claim | int | Claim TTL in minutes (default 60) | **kept-evidence** |
| `--worktree` | claim | string | Path to task worktree (required) | **kept-evidence** |
| `--force` | claim, merged, accept-citation | bool | Override warnings or require confirmation | **kept-evidence** |
| `--msg` | note | string | Note message | **kept-evidence** |
| `--note-id` | note | string | Note ID for deletion | **kept-evidence** |
| `--to` | transition | string | Target status (open, in-progress, done, merged, blocked, cancelled) | **kept-evidence** |
| `--outcome` | transition | string | Outcome summary on completion | **kept-evidence** |
| `--branch` | transition, review | string | Feature branch name | **kept-evidence** |
| `--pr` | transition, merged | string | PR number or URL | **kept-evidence** |
| `--worker` | assign, workers | string | Worker ID (for assignment or filtering) | **kept-evidence** |
| `--topic` | decision | string | Decision topic | **kept-evidence** |
| `--choice` | decision | string | Chosen option | **kept-evidence** |
| `--rationale` | decision, accept-citation | string | Why this choice | **kept-evidence** |
| `--affects` | decision | string[] | Affected scope globs | **kept-evidence** |

### Synchronization/Sync Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--dry-run` | sync, decompose revert, import, scope-delete | bool | Preview without writing ops | **kept-evidence** |
| `--into` | sync | string | Target branch for merge checks | **kept-evidence** |

### DAG/Decompose Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--plan` | decompose apply, decompose revert, decompose context | string | Path to plan JSON file | **kept-evidence** |
| `--example` | decompose apply | bool | Print minimal plan example | **kept-evidence** |
| `--schema` | decompose apply | bool | Print JSON Schema | **kept-evidence** |
| `--strict` | decompose apply, doctor, validate | bool | Treat warnings as errors | **kept-evidence** |
| `--generate-ids` | decompose apply | bool | Replace plan IDs with UUIDs | **kept-evidence** |
| `--root` | decompose apply | string | Override inferred root | **kept-evidence** |
| `--sources` | decompose context | string | Comma-separated source IDs to include | **kept-evidence** |
| `--template` | decompose context | string | Prompt template with placeholders | **kept-evidence** |
| `--output` | decompose context, review | string | Output file (default: stdout) | **kept-evidence** |
| `--format` | decompose context, log | string | Output format (text, json, jsonl) | **kept-evidence** |
| `--existing-dag` | decompose context | bool | Include existing DAG in context | **kept-evidence** |
| `--dep` | link, unlink | string | Dependency issue ID | **kept-evidence** |
| `--rel` | link | string | Relationship type (default blocked_by) | **kept-evidence** |
| `--source-id` | link, source-link | string | Source issue ID | **kept-evidence** |

### Citation/Source Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--source-id` | source-link | string | UUID of source entry in manifest | **kept-evidence** |
| `--ci` | accept-citation, validate | bool | Non-interactive mode / bypass prompt | **kept-evidence** |
| `--url` | sources | string | URL or path of source | **kept-evidence** |
| `--type` | sources | string | Provider type (filesystem, confluence, sharepoint) | **kept-evidence** |

### Query/Filter Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--status` | list | string | Filter by status | **kept-evidence** |
| `--terminal` | list | bool | Filter to terminal statuses (done, merged, cancelled) | **kept-evidence** |
| `--group` | list | bool | Group by status with headers | **kept-evidence** |
| `--assigned-to` | ready | string | Filter to tasks assigned to worker | **kept-evidence** |
| `--explain` | ready | bool | Diagnose why tasks aren't ready | **kept-evidence** |
| `--field` | show, transition (alternative to --to) | string | Extract specific fields | **kept-evidence** |
| `--json` | workers, log | bool | Output as JSONL | **kept-evidence** |
| `--worker` | log | string | Filter ops by worker ID | **kept-evidence** |
| `--since` | log | string | Filter ops since RFC3339 or YYYY-MM-DD | **kept-evidence** |

### Diagnostic/Admin Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--check` | worker-init | bool | Verify existing worker ID without modifying | **kept-evidence** |
| `--verbose` | doctor | bool | Emit file paths and uncited issue IDs | **kept-evidence** |
| `--quiet` | validate | bool | Suppress INFO lines | **kept-evidence** |
| `--scope` | validate | string | Validate only subtree at node ID | **kept-evidence** |
| `--parent` | validate | string | Validate only direct children of parent | **kept-evidence** |
| `--exclude-worker` | materialize | string | Skip ops from worker ID (diagnostic) | **kept-evidence** |
| `--global` | bootstrap | bool | Deploy to ~/.claude/ instead of .claude/ | **kept-evidence** |
| `--with-hooks` | bootstrap | bool | Also write harness hook configuration | **kept-evidence** |
| `--platform` | bootstrap | string[] | Restrict to specific platform(s) | **kept-evidence** |
| `--budget` | render-context | int | Token budget for truncation (default 4000) | **kept-evidence** |
| `--raw` | render-context | bool | Skip truncation | **kept-evidence** |
| `--at` | render-context | string | Replay context at git commit SHA | **kept-evidence** |
| `--limit` | context-history | int | Max commits to scan (default 100) | **kept-evidence** |
| `--source` | import | string | Source ID to link imported items to | **kept-evidence** |
| `--assessment` | review attest | string | Assessment file or '-' for stdin | **kept-evidence** |
| `--bundle` | review attest | string | Review bundle file path | **kept-evidence** |
| `--base` | review cite | string | Base revision for diff | **kept-evidence** |
| `--head` | review cite | string | Head revision for diff | **kept-evidence** |
| `--clear-context-files` | amend | bool | Remove all context_files entries | **kept-evidence** |

## Priority Levels (on priority field)

Values are documented on the `--priority` flag in create.go:142.

| Value | Status | Notes |
|-------|--------|-------|
| `critical` | **kept-evidence** | Used in create.go flag description. |
| `high` | **kept-evidence** | Used in create.go flag description. |
| `medium` | **kept-evidence** | Used in create.go flag description. |
| `low` | **kept-evidence** | Used in create.go flag description. |
| (empty/omitted) | **kept-evidence** | Priority is optional. |

## Estimated Complexity Levels

Values appear on the estimated_complexity field (state.go:24) and are set via create payload. No explicit enumeration found in corpus.

| Value | Status | Notes |
|-------|--------|-------|
| (free-form string) | **kept-justified** | Field exists in state and payload but no command flag documented for --complexity. Likely set by agents or inferred from acceptance criteria. No tests enumerate values. |

## Relationship Types (for link/unlink)

Defined in the `rel` field of link operations (types.go:97) and link command flag (link.go).

| Type | Used By | Status | Notes |
|------|---------|--------|-------|
| `blocked_by` | link.go:92 (default), ready queue logic | **kept-evidence** | Default relationship. Used to block ready queue eligibility. |
| `blocks` | link.go, inverse of blocked_by | **kept-evidence** | Inverse relationship. Records what this issue blocks. |

## Provider Types (for sources)

Enumerated in sources.go flag description:

| Type | Usage | Status | Notes |
|------|-------|--------|-------|
| `filesystem` | sources.go:flag, tests | **kept-evidence** | Local file-based sources. |
| `confluence` | sources.go:flag, tests | **kept-evidence** | Confluence wiki integration. |
| `sharepoint` | sources.go:flag, tests | **kept-evidence** | SharePoint document integration. |

## Summary Statistics

- **Issue Types**: 5 (all kept-evidence)
- **Statuses**: 7 (all kept-evidence)
- **Confidence States**: 3 (all kept-evidence)
- **Issue Fields**: 30 (all kept-evidence)
- **Op Types**: 19 (all kept-evidence)
- **CLI Commands**: 43 (all kept-evidence, 6 groups)
- **Command Flags**: ~100+ (all kept-evidence)
- **Parked Surfaces**: 0
- **Estimated Complexity Levels**: TBD (no explicit enumeration)

## Query Recipe (Reproducibility)

This census was compiled using the following queries and inspection methods:

### Issue Types
```bash
grep -n "validTypes\|allTypes\|hierarchy" internal/issuetype/issuetype.go
```

### Statuses
```bash
grep -n "Status[A-Z]\|ValidTransitionTargets" internal/ops/types.go
```

### Confidence States
```bash
grep -n "Confidence\|confidence" internal/ops/types.go cmd/armature/create.go
grep -n "inferred" cmd/armature/claim.go
```

### Issue Fields
```bash
grep -n "^type Issue struct" internal/materialize/state.go
grep -A 50 "^type Issue struct" internal/materialize/state.go
```

### Op Types
```bash
grep -n "^const (" internal/ops/types.go | head -1
grep -n "Op[A-Z]\|OpSourceLink\|OpDAG\|Op[A-Z]" internal/ops/types.go
```

### CLI Commands
```bash
grep -n "newRootCmd\|AddCommand" cmd/armature/main.go
grep -E "newCreateCmd|newClaimCmd|newTransitionCmd" cmd/armature/main.go | wc -l
```

### Command Flags
```bash
grep -h "cmd.Flags()\.String\|cmd.Flags()\.Bool\|cmd.Flags()\.Int\|cmd.Flags()\.StringSlice" cmd/armature/*.go
```

### Issue Field Usage
```bash
grep -r "\.Priority\|\.Assignee\|\.EstComplexity" internal/materialize/ --include="*.go" | grep -v test
```

To reproduce this census in the future:

1. Run the grep recipes above to verify each surface
2. Check for new command files in cmd/armature/ via `ls cmd/armature/*.go | wc -l`
3. Verify no new OpTypes by checking `grep "Op[A-Z].*=" internal/ops/types.go`
4. Verify no new issue types by checking `internal/issuetype/issuetype.go` validTypes map
5. Check for any confidence state changes in claim.go and materialize/engine.go
6. Audit new flags by running: `grep -h "cmd.Flags()." cmd/armature/*.go | grep -v "^//" | sort -u`

## Completeness Notes

- **Not included**: Protocol buffers or RPC types (not used by armature; ops-based).
- **Not included**: Internal cache structures or intermediate types (only public surfaces).
- **Not included**: Test-only enums or fixtures (only production surfaces).
- **Not included**: Error codes (documented separately in internal/exitcodes/exitcodes.go).
- **All statuses validated**: ValidTransitionTargets map in ops.go:47 is the canonical source of valid transition targets.
- **All issue types validated**: issuetype.validTypes map is the canonical enumeration.
- **All op types included**: Checked against materialize/engine.go handler registry in RegisteredOpTypes.
