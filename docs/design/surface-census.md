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
| `draft` | internal/ops/types.go:127 (comment), create.go:62, Payload.Confidence | Confidence field in create payload, ready-queue gate (internal/ready/compute.go:~60, ~113), dag-transition promotion, tests | **kept-evidence** | Birth is always draft. Gates *readiness*, not claiming: `draft` issues are excluded from the ready queue in internal/ready/compute.go (ComputeReady and ExplainNotReady both skip `Confidence == "draft"`). Promoted to `verified` via dag-transition/confirm flow. |
| `verified` | internal/ops/types.go:127 (comment: default), materialize assumptions, tests | Default when confidence absent, claim eligibility, tests | **kept-evidence** | Implicit default for issues created without explicit confidence. Claim-eligible state. |
| `inferred` | claim.go:311 (check) | Claim rejection logic | **kept-evidence** | Provenance confidence value that blocks *claiming* (claim.go:311 rejects `Confidence == "inferred"` specifically — it does not check `draft`). Set during materialization when reconstructing inferred nodes. |

## Issue Fields

The following fields appear on the materialized Issue struct (internal/materialize/state.go:13-49):

| Field | Type | Defined | Set By Op | Status | Notes |
|-------|------|---------|-----------|--------|-------|
| `id` | string | state.go:14 | create | **kept-evidence** | Primary key. Set at creation, immutable. |
| `type` | string | state.go:15 | create | **kept-evidence** | Issue type (epic, story, feature, task, bug). Set at creation, amendable via amend. |
| `status` | string | state.go:16 | create (open), transition, claim | **kept-evidence** | Current lifecycle state. Primary index for queries. |
| `rollup_status_before` | string | state.go:17 | RunRollup (set on derived promotion, cleared on retraction and by any transition op) | **kept-evidence** | The status `RunRollup` replaced when it promoted this issue to merged, marking that promotion as derived rather than op-asserted. Rollup restores it before recomputing, so a parent promoted because its children were terminal is retracted when one of them leaves a terminal state — without it, an incremental materialization (which reloads prior promotions from the state cache) and a cold replay of the same log disagree on whether the parent is merged. Empty for a status that came from the log, including an op-asserted merged, which rollup must never walk back. See TOPTIER-B1. |
| `title` | string | state.go:17 | create | **kept-evidence** | Human-readable label. Set at creation. |
| `parent` | string | state.go:18 | create, reparent | **kept-evidence** | Parent issue ID for hierarchy. Set at creation, changed via `reparent` (not `amend` — amend registers no `--parent` flag and applyAmend never touches Parent). |
| `children` | []string | state.go:19 | derived from parent links | **kept-evidence** | Materialized inverse of parent. Read-only. |
| `blocked_by` | []string | state.go:20 | link op (rel=blocked_by) | **kept-evidence** | Issues that must complete first. Used by ready queue logic. |
| `blocks` | []string | state.go:21 | derived automatically as the inverse of a `link` op with rel=blocked_by applied to the other issue (engine.go applyLink) | **kept-evidence** | Issues blocked by this one. Inverse of blocked_by. Never set directly by a `rel=blocks` op — invalid `--rel` values are rejected at the CLI layer (link.go RunE) before the op is ever appended to the log. |
| `assignee` | string | state.go:22 | (no writer found) | **parked** | Dead field: no code path sets `Issue.Assignee` anywhere in internal/ or cmd/ (verified via `grep -rn '\.Assignee *=' internal/ cmd/`). The `assign` command sets `AssignedWorker` (internal/materialize/engine.go, applyAssign), not `Assignee`. Re-entry criterion: a writer is added and exercised by a test, or the field is removed from state.go. |
| `priority` | string | state.go:23 | create payload | **kept-evidence** | Priority level (critical, high, medium, low). Set via --priority flag on create. `amend` registers no `--priority` flag and `applyAmend` never touches `Priority`. Used in diagnostics. |
| `estimated_complexity` | string | state.go:24 | create payload | **kept-justified** | Complexity estimate. No CLI flag or plan field currently sets it — nothing in cmd/ or internal/decompose/ produces `EstComplexity`; it is only ever copied through from whatever the create payload already contains. See the Estimated Complexity Levels section below for what actually reads/interprets this field (validate.go's small/large enum checks). |
| `definition_of_done` | string | state.go:25 | create, amend | **kept-evidence** | Completion criteria. Required for task type (issuetype.go:90). |
| `scope` | []string | state.go:26 | create, amend | **kept-evidence** | File scope globs. Used by overlap detection and scope-rename/delete commands. |
| `context_files` | []string | state.go:27 | create, amend | **kept-evidence** | Stable reference files. Set via --context-file flag. Used by harness hook to render context. |
| `acceptance` | json.RawMessage | state.go:28 | create, amend (acceptance JSON) | **kept-evidence** | Acceptance criteria as opaque JSON. Set via --acceptance flag on create. |
| `context` | json.RawMessage | state.go:29 | (no current producer) | **kept-justified** | Rendered context blob. Copied through from ops in engine.go, but nothing currently sets `Payload.Context` — internal/decompose/apply.go's create payload construction (~lines 183-193) does not populate it, and no CLI flag sets it either. Under-documented/unproduced; similar situation to `estimated_complexity`. Flagged for a future story. |
| `source_citation` | json.RawMessage | state.go:30 | source-link ops (transitive) | **kept-evidence** | Citation metadata. Used by sources and citation acceptance flows. |
| `provenance` | Provenance | state.go:31 | create (method, confidence, worker) | **kept-evidence** | Metadata: method (create/decompose/import), confidence, source worker, dag_confirmed flag. Used to detect inferred nodes and confidence state. |
| `decision_refs` | []string | state.go:32 | decision op (derives refs) | **kept-evidence** | Decision operation IDs affecting this issue. Read-only. |
| `outcome` | string | state.go:33 | transition op (to=done) | **kept-evidence** | Outcome summary on completion. Set by transition command with --outcome. |
| `prior_outcomes` | []string | state.go:34 | appended when reopening | **kept-evidence** | History of previous outcomes. Accumulated on reopen. |
| `notes` | []Note | state.go:35 | note op | **kept-evidence** | Worker annotations. Set by note command. |
| `decisions` | []Decision | state.go:36 | decision op | **kept-evidence** | Structured decisions affecting scope. Set by decision command. |
| `source_links` | []SourceLink | state.go:37 | source-link op | **kept-evidence** | External source references. Set by source-link or create --source. |
| `citation_acceptances` | []CitationAcceptance | state.go:38 | citation-accepted op | **kept-evidence** | Record of accepted source citations. Set by accept-citation command. |
| `assessment_attestations` | []review.AssessmentAttestation | state.go:39 | assessment-attested op (review.go) | **kept-evidence** | Code review assessment records. Set by review record flow. |
| `claimed_by` | string | state.go:40 | claim op | **kept-evidence** | Worker ID that holds the claim. Set by claim command. |
| `claimed_at` | int64 | state.go:41 | claim op | **kept-evidence** | Claim timestamp (epoch ms). Used to calculate staleness. |
| `claim_token` | string | state.go:52 | claim op (unique per-claim nonce) | **kept-evidence** | Unique per-claim identity, since ClaimedAt alone has only 1-second resolution and cannot distinguish two same-worker claims in the same second. Set by claim command (crypto/rand nonce). A compensating rollback transition stamps `if_claim_token` with this value so materialize.applyTransition can apply it only if it still matches at replay time — see docs/design/recovery-state-machine.md. |
| `claim_ttl` | int | state.go:42 | claim op (TTL in minutes) | **kept-evidence** | Claim time-to-live. Set by --ttl flag (default 60 min). Used by stale review logic. |
| `last_heartbeat` | int64 | state.go:43 | heartbeat op | **kept-evidence** | Last heartbeat timestamp. Refreshed by heartbeat command to prevent staleness. |
| `last_claiming_worker_activity` | int64 | state.go:51 | claim, heartbeat, transition ops (only when op.WorkerID == ClaimedBy) | **kept-evidence** | Liveness signal scoped to the claiming worker only, unlike `updated` (bumped by every op regardless of author). Used by `doctor --fix`'s `claimExpired` to avoid a third party's unrelated note/link op masking a crashed worker's stale claim. |
| `worktree_path` | string | state.go:52 | claim op (auto-provisioned worktree path) | **kept-evidence** | Absolute path of the worktree provisioned by `arm claim --worktree`. Set by claim command; lets recovery tooling locate the worktree deterministically. Legacy claim ops without the field replay cleanly (omitempty). |
| `branch` | string | state.go:53 | transition op (branch field) | **kept-evidence** | Feature branch name. Set by transition on completion. Used by merged flow. |
| `pr` | string | state.go:54 | transition op (pr field), merged op | **kept-evidence** | PR number or URL. Set by transition or merged command. |
| `assigned_worker` | string | state.go:55 | assign op (assigned_to) | **kept-evidence** | Worker assigned for work (distinct from claim). Set by assign command. |
| `preferred_model` | string | state.go:56 | (no writer found) | **parked** | Dead field: no CLI flag sets `Payload.PreferredModel` anywhere — `create.go` registers no `--preferred-model` flag (only --title through --source), and neither does `decompose-apply`. `applyCreate` (internal/materialize/engine.go) only copies through whatever is already in the payload, which is always empty. Same situation as `assignee` (row above). Re-entry criterion: a writer (flag or decompose plan field) is added and exercised by a test, or the field is removed from state.go. |
| `updated` | int64 | state.go:57 | every op | **kept-evidence** | Last modified timestamp (epoch ms). Set to op timestamp for every state change. |

## Operation Types (OpTypes)

The following op types are defined in internal/ops/types.go and materialized by handlers in internal/materialize/engine.go:

| Op Type | Defined | Handler | Status | Notes |
|---------|---------|---------|--------|-------|
| `create` | internal/ops/types.go:8 | engine.go | **kept-evidence** | Creates new issue. Emits create payload with type, title, parent, scope, etc. |
| `claim` | internal/ops/types.go:9 | engine.go | **kept-evidence** | Assigns issue to worker with TTL. Sets claimed_by, claimed_at, claim_ttl. |
| `heartbeat` | internal/ops/types.go:10 | engine.go | **kept-evidence** | Refreshes claim TTL. Updates last_heartbeat timestamp. |
| `transition` | internal/ops/types.go:11 | engine.go | **kept-evidence** | Changes issue status. Payload: to (status), outcome, branch, pr. |
| `note` | internal/ops/types.go:12 | engine.go | **kept-evidence** | Adds worker note. Payload: msg, note_id for deletion. |
| `note-delete` | internal/ops/types.go:13 | engine.go | **kept-evidence** | Soft-deletes note by ID. Marks note.deleted=true. |
| `link` | internal/ops/types.go:14 | engine.go | **kept-evidence** | Adds dependency. Payload: dep (target), rel (relationship type). Only rel=blocked_by is a supported input; invalid `--rel` values are rejected at the CLI layer (link.go's RunE) before the op is appended to the log, not at replay/materialize time. `blocks` is derived automatically as the inverse and is never a valid input. For backward compatibility, engine.go's `applyLink` silently no-ops on any non-blocked_by rel it encounters during replay, so historical op-log entries predating this validation still replay cleanly (see `TestApplyLinkOp_LegacyNonBlockedByRelIsNoOp`). |
| `unlink` | internal/ops/types.go:15 | engine.go | **kept-evidence** | Removes dependency. Payload: dep (target). |
| `source-link` | internal/ops/types.go:16 | engine.go | **kept-evidence** | Links external source. Payload: source_id, source_url. Creates source_links entries. |
| `source-fingerprint` | internal/ops/types.go:17 | engine.go | **kept-evidence** | Records source version. Payload: sha, version_id, provider. Used for staleness detection. |
| `dag-transition` | internal/ops/types.go:18 | engine.go | **kept-evidence** | Promotes confidence (draft→verified). Payload: issue_id, confirmed, confirmed_noninteractively. Sets dag_confirmed flag. |
| `decision` | internal/ops/types.go:19 | engine.go | **kept-evidence** | Records structured decision. Payload: topic, choice, rationale, affects (scope globs). |
| `assign` | internal/ops/types.go:20 | engine.go | **kept-evidence** | Assigns worker to issue. Payload: assigned_to (worker ID). |
| `amend` | internal/ops/types.go:21 | engine.go | **kept-evidence** | Updates issue metadata. Payload: type, scope, context_files, dod, acceptance (partial updates). |
| `citation-accepted` | internal/ops/types.go:22 | engine.go | **kept-evidence** | Records citation acceptance. Payload: source_entry_id, confirmed_noninteractively flag. |
| `scope-rename` | internal/ops/types.go:23 | engine.go | **kept-evidence** | Renames scope glob. Payload: old_path, new_path. Updates scope entries. |
| `scope-delete` | internal/ops/types.go:24 | engine.go | **kept-evidence** | Removes scope glob. Payload: deleted_path. Removes from scope array. |
| `reparent` | internal/ops/types.go:27 | engine.go | **kept-evidence** | Moves issue to new parent. Payload: parent (new parent ID, can be empty for top-level). |
| `assessment-attested` | internal/ops/types.go:30 | engine.go | **kept-evidence** | Records code review attestation. Payload: assessment (JSON blob). Used by review record. |

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
| `confirm` | main.go:126, confirm.go | Interactive confidence promotion | **kept-evidence** | DAG flow. Promotes draft→verified. Plan Release runs whole-graph strict validate. |
| `assign` | main.go:130, assign.go | Assign worker to issue | **kept-evidence** | Decouples assignment from claiming. Sets assigned_worker. |

### DAG Commands (dag group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `dag` | main.go, dag.go | DAG group command | **kept-evidence** | Container for DAG-related subcommands. |
| `dag apply` | dag.go, decompose.go | Create issues from plan JSON | **kept-evidence** | Bulk creation from structured plan. Validates hierarchy. |
| `dag context` | dag.go, decompose.go | Generate context for plan | **kept-evidence** | Agent-facing tool. Renders existing DAG and sources into plan context. |
| `dag revert` | dag.go, decompose.go | Remove issues created by plan | **kept-evidence** | Undo plan application. Validates that no children exist. |
| `dag summary` | dag.go, dagsum.go | Summarize draft nodes | **kept-evidence** | Diagnostic and planning tool. Lists unconfirmed nodes in a subtree. |
| `dag transition` | dag.go, dag_transition.go | Promote confidence (low-level) | **kept-evidence** | Sets dag_confirmed flag. Underpins confidence workflow. Promotion to verified requires a strict-green arm validate of the whole graph (Plan Release). |
| `dag override-release` | dag.go, dag_override_release.go | Human Release Override | **kept-evidence** | TTY + type-the-id + reason. Records skipped_validate_gate. Never a green release. Not a flag on agent verbs. |
| `link` | main.go, link.go | Add dependency | **kept-evidence** | Couples issues. Only `--rel blocked_by` is a valid input; `blocks` is derived automatically as the inverse. |
| `unlink` | main.go, unlink.go | Remove dependency | **kept-evidence** | Uncouples issues. Removes from blocked_by/blocks. |

### Sync Commands (sync group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `sync` | main.go, sync.go | Auto-transition closed PRs | **kept-evidence** | CI integration. Scans git for merged branches and transitions issues. |
| `push-ops` | main.go, push_ops.go | Push pending ops to _armature branch | **kept-evidence** | Publishes ops to VCS. Called before PR or manually. |
| `merged` | main.go, merged.go | Manually transition to merged | **kept-evidence** | Explicit merge record. Sets PR and branch fields. |
| `materialize` | main.go, materialize.go | Regenerate state from ops log | **kept-evidence** | Diagnostic/recovery. Rebuilds snapshot from scratch. |
| `import` | main.go, import.go | Import issues from external source | **kept-evidence** | Onboarding tool. Creates issues with source links. |

### Admin Commands (admin group)

| Command | Defined | Purpose | Status | Notes |
|---------|---------|---------|--------|-------|
| `version` | main.go:78, version.go | Print arm version | **kept-evidence** | Diagnostic. |
| `worker-init` | main.go:82, worker_init.go | Initialize worker ID | **kept-evidence** | One-time setup. Stores UUID in git config. |
| `bootstrap` | main.go:86, bootstrap.go | Deploy harness hook to project | **kept-evidence** | Setup command. Installs pre-commit or post-merge hooks. |
| `create` | main.go:189, create.go | Create new issue | **kept-evidence** | Direct issue creation (not decompose-based). |
| `reparent` | main.go:193, reparent.go | Move issue to new parent | **kept-evidence** | Hierarchy adjustment. Payload: parent. |
| `validate` | main.go:249, validate.go | Validate issue graph | **kept-evidence** | Linter. Strict by default (warnings fail the run; green is a single summary line). JSON keeps native error/warning/info buckets; Strict drives OK/exit only. Validates the whole graph (no --scope/--parent). Supports --ci (CI / make validate-graph, not make check), --strict (default true), --quiet. `--ci --strict=false` is rejected. |
| `validate doc-examples` | validate_doc_examples.go | Validate typed JSON examples in canonical documentation | **kept-evidence** | Subcommand of `validate`. Used by `make check`. |
| `render-context` | main.go, render_context.go | Render issue context | **kept-evidence** | Agent-facing. Truncates to token budget. |
| `log` | main.go, log.go | List ops log entries | **kept-evidence** | Audit/debugging. Supports filtering by issue/worker. |
| `workers` | main.go, workers.go | List active workers | **kept-evidence** | Diagnostic. Shows claimed issues per worker. |
| `sources` | main.go, sources.go | Manage source manifest | **kept-evidence** | Citation infrastructure. CRUD for source entries. Subcommands: accept-citation, add, link, stale-review, sync, verify. |
| `sources add` | sources.go | Add a source entry to the manifest | **kept-evidence** | Subcommand of `sources`. |
| `sources stale-review` | sources.go, stalereview.go | Review stale sources | **kept-evidence** | Subcommand of `sources`. |
| `sources sync` | sources.go | Sync source manifest state | **kept-evidence** | Subcommand of `sources`. |
| `sources verify` | sources.go | Verify source manifest entries | **kept-evidence** | Subcommand of `sources`. |
| `show` | main.go:231, show.go | Display issue details | **kept-evidence** | Query tool. Supports --field for extraction. |
| `list` | main.go:235, list.go | List issues | **kept-evidence** | Query tool. Supports filtering and grouping. |
| `scope-rename` | main.go:239, scope_rename.go | Rename scope glob | **kept-evidence** | Cleanup. Updates scope and decision affects. |
| `scope-delete` | main.go:243, scope_delete.go | Delete scope glob | **kept-evidence** | Cleanup. Removes from scope arrays. |
| `doctor` | main.go:247, doctor.go | Diagnose DAG health | **kept-evidence** | Validator. Checks for broken refs, cycles, orphans. |
| `worktree` | main.go:266, worktree.go | Manage managed worktrees | **kept-evidence** | Container for worktree reconciliation subcommands. |
| `worktree list` | main.go:266, worktree.go | List managed worktrees with bound issue and claim status | **kept-evidence** | Subcommand of `worktree`. Flags orphans (worktree, no live claim) and ghosts (claim recording a missing worktree). |
| `worktree gc` | main.go:266, worktree.go | Remove worktrees for merged/cancelled issues | **kept-evidence** | Subcommand of `worktree`. Supports --dry-run to preview removals. Only mutates state by removing worktrees. |
| `completion` | main.go:251, cmd_completion.go | Bash/zsh completion | **kept-evidence** | Shell integration. Cobra-generated. |
| `hook` | main.go:255, hook.go | Manage harness hooks | **kept-evidence** | Configuration. Enable/disable/debug hooks. |
| `hook run` | hook.go:31 | Run a named harness hook | **kept-evidence** | Subcommand of `hook`. |
| `gate` | main.go, gate.go | Run configured quality-gate profiles | **kept-evidence** | Container for `gate run`. Profiles come from `.armature/config.json` `gates` map; `full` is reserved as the publish profile. |
| `gate run` | gate.go | Execute a named gate profile and append evidence | **kept-evidence** | Subcommand of `gate`. Runs in the invoking checkout. Streams output to `.armature/gates/` and appends a `gate-evidence` op to the invoking worker log. Trees dirty before or after the command record `uncommitted` (not citable). |
| `tui` | main.go:259, tui.go | TUI for issue navigation | **kept-evidence** | Interactive mode. Lists and filters issues. |
| `context-history` | main.go:263, context_history.go | Scan git history for context | **kept-evidence** | Diagnostic. Helps find stable reference commits. |
| `harness-hook` | main.go:267, harness_hook.go | Harness hook runner (internal) | **kept-evidence** | Internal. Runs on pre-commit and post-merge. |
| `review` | main.go:271, review.go | Manage conformance reviews for issues | **kept-evidence** | Review infrastructure. |
| `review prepare` | review.go:33 | Prepare a review bundle for an issue | **kept-evidence** | Subcommand of `review`. Flags: --issue, --base, --head, --output. |
| `review record` | review.go:157 | Record a conformance assessment for an issue | **kept-evidence** | Subcommand of `review`. Flags: --issue, --assessment, --bundle. |
| `review commits` | review.go:185 | List delivery commits for an issue | **kept-evidence** | Subcommand of `review`. Flags: --issue, --branch (default HEAD). |
| `review validate` | review_validate.go | Read-only assessment validation with auto-fix suggestions | **kept-evidence** | Subcommand of `review`. Flags: --assessment, --bundle (required). Does not append ops. Advisory; record remains the enforcement gate. |

## Command Flags

The following flags are defined across all commands. Grouped by usage pattern.

### Universal/Root Flags (from main.go:66-69)

| Flag | Type | Default | Usage | Status | Notes |
|------|------|---------|-------|--------|-------|
| `--debug` | bool | false | Dump stack traces on error | **kept-evidence** | Diagnostic. Always available. |
| `--format` | string | human | Output format: human, json, agent | **kept-evidence** | Auto-set to agent for non-TTY. Inherited by every command except `dag context`, which defines its own command-local `--format` (see DAG/Decompose Flags below). |
| `--repo` | string | "" (current directory) | Repository path | **kept-evidence** | Allows multi-repo operation. |
| `--non-interactive` | bool | false | Skip TUI, use structured output | **kept-evidence** | Auto-set in CI. |

### Issue/Field Flags (common across commands)

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--issue` | claim, transition, note, decision, amend, show, assign, unassign, etc. | string | Target issue ID (positional alternative in some commands) | **kept-evidence** |
| `--type` | create, amend, list | string | Issue type (epic, story, feature, task, bug) | **kept-evidence** |
| `--parent` | create, reparent, list, ready | string | Parent issue ID (or filter) | **kept-evidence** |
| `--title` | create, sources add | string | Human-readable title | **kept-evidence** |
| `--scope` | create, amend | string[] | File scope globs | **kept-evidence** |
| `--dod` | create, amend | string | Definition of done | **kept-evidence** |
| `--priority` | create | string | Priority level (critical, high, medium, low) | **kept-evidence** |
| `--acceptance` | create, amend | string | Acceptance criteria as JSON | **kept-evidence** |
| `--context-file` | create, amend | string[] | Stable reference files | **kept-evidence** |
| `--id` | create | string | Explicit issue ID (auto-generated if empty) | **kept-evidence** |
| `--confidence` | create | string | Rejected. Birth is always draft; promote with dag transition --to verified. | **kept-evidence** |
| `--source` | create, import | string | Source ID/URL to link at creation | **kept-evidence** |

### Workflow Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--ttl` | claim | int | Claim TTL in minutes (default 60) | **kept-evidence** |
| `--worktree` | claim | string | Required worktree destination; a value-less form remains compatible and provisions .worktrees/<issue-id>, while an explicit value selects a new destination | **kept-evidence** |
| `--from` | claim | string | Parent worktree whose current branch and tip seed an explicit new --worktree destination | **kept-evidence** |
| `--force` | claim, merged, transition | bool | Override warnings or require confirmation | **kept-evidence** |
| `--msg` | note | string | Note message | **kept-evidence** |
| `--note-id` | note | string | Note ID for deletion | **kept-evidence** |
| `--to` | transition | string | Target status: open, in-progress, done, merged, blocked, cancelled | **kept-evidence** |
| `--skip-delivery-gate` | transition | bool | Bypass the delivery gate (clean tree, scope containment, commit reference) checked when transitioning to done; override is recorded in the transition op's payload | **kept-evidence** |
| `--to` | dag transition | string | Target confidence level: draft, verified (default verified). Distinct from `transition`'s `--to` — this one stores into `targetConfidence` and is validated against the confidence enum, not the status enum (cmd/armature/dag_transition.go). Running `dag transition --to done` is now a validation error rather than silently stamping "done" into Provenance.Confidence. Promotion to verified runs full-graph strict validate first. | **kept-evidence** |
| `--reason` | dag override-release | string | Recorded reason for a Release Override. Required. | **kept-evidence** |
| `--outcome` | transition | string | Outcome summary on completion | **kept-evidence** |
| `--branch` | transition, review commits | string | Feature branch name | **kept-evidence** |
| `--pr` | transition, merged | string | PR number or URL | **kept-evidence** |
| `--worker` | assign, ready | string | Worker ID for assignment | **kept-evidence** |
| `--topic` | decision | string | Decision topic | **kept-evidence** |
| `--choice` | decision | string | Chosen option | **kept-evidence** |
| `--rationale` | decision | string | Why this choice | **kept-evidence** |
| `--affects` | decision | string[] | Affected scope globs | **kept-evidence** |

### Synchronization/Sync Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--dry-run` | sync, dag apply, dag revert, import, doctor, worktree gc | bool | Preview without writing ops (for `worktree gc`, preview removals without removing) | **kept-evidence** |
| `--into` | sync | string | Target branch for merge checks | **kept-evidence** |

### DAG/Decompose Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--plan` | dag apply, dag revert, dag context | string | Path to plan JSON file | **kept-evidence** |
| `--example` | dag apply | bool | Print minimal plan example | **kept-evidence** |
| `--schema` | dag apply | bool | Print JSON Schema | **kept-evidence** |
| `--strict` | doctor, validate | bool | Treat warnings as errors. Default false on doctor; default true on validate (D7). | **kept-evidence** |
| `--generate-ids` | dag apply | bool | Replace plan IDs with UUIDs | **kept-evidence** |
| `--root` | dag apply | string | Override inferred root | **kept-evidence** |
| `--sources` | dag context | string | Comma-separated source IDs to include | **kept-evidence** |
| `--template` | dag context | string | Prompt template with placeholders | **kept-evidence** |
| `--output` | dag context, review prepare | string | Output file (default: stdout) | **kept-evidence** |
| `--format` | dag context | string | Command-local output format override (text or json; decompose.go:383). All other commands listed here read the inherited root persistent `--format` flag (see Universal/Root Flags above) rather than defining their own — they do not appear in this row. | **kept-evidence** |
| `--existing-dag` | dag context | bool | Include existing DAG in context | **kept-evidence** |
| `--approve-all` | dag summary | bool | Approve all pending draft items in non-interactive mode | **kept-evidence** |
| `--dep` | link, unlink | string | Dependency issue ID | **kept-evidence** |
| `--rel` | link | string | Relationship type (default blocked_by) | **kept-evidence** |
| `--source` | link, unlink | string | Source issue ID | **kept-evidence** |

### Citation/Source Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--ci` | validate | bool | Fail-closed alias used by CI / make validate-graph, not by make check. Implied by default --strict; still accepted so CI scripts keep working. Contradicts an explicit `--strict=false`. | **kept-evidence** |
| `--url` | sources add | string | URL or path of source | **kept-evidence** |
| `--type` | sources add | string | Provider type (filesystem, confluence, sharepoint) | **kept-evidence** |

### Query/Filter Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--status` | list | string | Filter by status | **kept-evidence** |
| `--terminal` | list | bool | Filter to terminal statuses (done, merged, cancelled) | **kept-evidence** |
| `--group` | list | bool | Group by status with headers | **kept-evidence** |
| `--assigned-to` | ready | string | Filter to tasks assigned to worker | **kept-evidence** |
| `--explain` | ready | bool | Diagnose why tasks aren't ready | **kept-evidence** |
| `--waves` | ready | bool | Partition ready queue into scope-disjoint dispatch waves (JSON/agent output only) | **kept-evidence** |
| `--field` | show, transition | string | Extract specific fields | **kept-evidence** |
| `--json` | workers, log | bool | Output as JSONL | **kept-evidence** |
| `--worker` | log | string | Filter ops by worker ID | **kept-evidence** |
| `--since` | log | string | Filter ops since RFC3339 or YYYY-MM-DD | **kept-evidence** |

### Diagnostic/Admin Flags

| Flag | Command(s) | Type | Notes | Status |
|------|-----------|------|-------|--------|
| `--check` | worker-init | bool | Verify existing worker ID without modifying | **kept-evidence** |
| `--repo` | worker-init, validate doc-examples | string | Command-local repository path override (worker_init.go:42, validate_doc_examples.go:24). bootstrap, doctor, harness-hook, and push-ops read the inherited root persistent `--repo` flag (see Universal/Root Flags above) rather than defining their own. | **kept-evidence** |
| `--verbose` | doctor | bool | Emit file paths and uncited issue IDs | **kept-evidence** |
| `--fix` | doctor | bool | Reconcile expired claims (claimed->open, in-progress->blocked) by appending ops; see [recovery-state-machine.md](./recovery-state-machine.md) | **kept-evidence** |
| `--quiet` | validate | bool | Suppress INFO lines on a failing run. Green output is already a single summary line. | **kept-evidence** |
| `--exclude-worker` | materialize | string | Skip ops from worker ID (diagnostic) | **kept-evidence** |
| `--global` | bootstrap | bool | Deploy to ~/.claude/ instead of .claude/ | **kept-evidence** |
| `--with-hooks` | bootstrap | bool | Also write harness hook configuration | **kept-evidence** |
| `--platform` | bootstrap | string[] | Restrict to specific platform(s) | **kept-evidence** |
| `--budget` | render-context | int | Token budget for truncation (default 4000) | **kept-evidence** |
| `--raw` | render-context | bool | Skip truncation | **kept-evidence** |
| `--at` | render-context | string | Replay context at git commit SHA | **kept-evidence** |
| `--limit` | context-history | int | Max commits to scan (default 100) | **kept-evidence** |
| `--source` | import | string | Source ID to link imported items to | **kept-evidence** |
| `--assessment` | review record, review validate | string | Assessment file or '-' for stdin | **kept-evidence** |
| `--bundle` | review record, review validate | string | Review bundle file path (required for review validate; optional for review record) | **kept-evidence** |
| `--base` | review prepare | string | Base revision for diff | **kept-evidence** |
| `--head` | review prepare | string | Head revision for diff | **kept-evidence** |
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

Values appear on the estimated_complexity field (state.go:24) and are copied from the create payload
in engine.go. An enumeration does exist downstream: internal/validate/validate.go's
`checkW6ComplexityMismatch` (~lines 388-402) interprets `small` and `large` specifically (flagging a
mismatch between complexity and scope size), so those two values are load-bearing even though nothing
enforces them at write time.

| Value | Status | Notes |
|-------|--------|-------|
| `small` | **kept-evidence** | Interpreted by checkW6ComplexityMismatch (validate.go:~390): warns if scope has >5 files. |
| `large` | **kept-evidence** | Interpreted by checkW6ComplexityMismatch (validate.go:~397): warns if scope has <2 files. |
| (other free-form string) | **kept-justified** | Field exists in state and payload but no CLI flag or decompose plan field currently sets it — nothing in cmd/ or internal/decompose/ produces `EstComplexity`; it is only ever copied through from whatever the create payload already contains. No current producer exists. Flagged here for a future story rather than fixed in this pass. |

## Relationship Types (for link/unlink)

Defined in the `rel` field of link operations (types.go:97) and link command flag (link.go). The only
accepted input value is `blocked_by`: link.go's RunE rejects any other `--rel` value before the op is
ever appended to the log, since `blocks` is a derived/output-only field (see below), never a valid
`--rel` input. This validation happens at the CLI layer, not at replay/materialize time — engine.go's
`applyLink` silently no-ops on any non-blocked_by rel it encounters during replay, preserving backward
compatibility with historical op-log entries written before this validation existed (see
`TestApplyLinkOp_LegacyNonBlockedByRelIsNoOp`).

| Type | Used By | Status | Notes |
|------|---------|--------|-------|
| `blocked_by` | link.go:55 (default), link.go RunE (rejects any other value before the op is written), engine.go applyLink (only value handled during replay), ready queue logic | **kept-evidence** | The only valid `--rel` input. Used to block ready queue eligibility. |
| `blocks` | state.go Issue.Blocks field, engine.go applyLink (auto-derived) | **kept-evidence** | Derived/output-only. Automatically populated as the inverse when a `blocked_by` link is applied to the *other* issue. Never a valid `--rel` input — `link.go`'s RunE rejects `rel=blocks` before the op is written. If a non-blocked_by rel is present in a historical op log (predating this validation), `applyLink` replays it as a no-op rather than erroring. |

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
- **Issue Fields**: 37 (33 kept-evidence, 2 kept-justified, 2 parked)
- **Op Types**: 19 (all kept-evidence)
- **CLI Commands**: 50 (all kept-evidence, 4 groups)
- **Command Flags**: ~100+ (all kept-evidence)
- **Parked Surfaces**: 2 (`assignee` and `preferred_model` fields — see Issue Fields)
- **Estimated Complexity Levels**: 2 enumerated (`small`, `large`, interpreted by validate.go), plus free-form; no current producer (CLI flag or decompose field) sets this field

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

## Censused Surfaces

This table is the authoritative list of surfaces this census covers, and the
documentation files each surface's drift check reads. `internal/validate`'s E13
vertical-slice check restates it in `censusedSurfaces` (internal/validate/coupling.go);
`TestCensusedSurfacesMatchesCensusDoc_REQ_LNGHZN_S10_T5` fails if the two drift
apart, so adding a row here without updating the map is caught at `make check`.

| Surface | Doc Files | Notes |
|---------|-----------|-------|
| `cmd/**` | `docs/commands.md`, `docs/design/surface-census.md` | CLI commands and flags. A task adding a flag owns its census row and its command documentation. |
