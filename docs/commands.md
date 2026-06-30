# Armature Command Reference

`arm` is a git-native work orchestration tool. This document provides a complete reference for every `arm` subcommand.

## Global Flags

The following flags are available for all commands:

- `--debug`: Dump debug diagnostics on error.
- `--format string`: Output format: `human`, `json`, `agent` (default "human").
- `--repo string`: Repository path (default: current directory).

---

## accept-citation

Accept a citation for an issue with a recorded rationale.

**Synopsis:**
`arm accept-citation [issue-id] [flags]`

**Flags:**
- `--ci`: Bypass interactive prompt (non-interactive/CI mode).
- `--issue string`: Issue ID to accept citation for.
- `--rationale string`: Rationale for accepting the citation (>=3 words).

**Example:**
```bash
arm accept-citation E5-S4-T3 --rationale "Documentation is complete and reviewed."
```

---

## amend

Amend fields on an existing issue.

**Synopsis:**
`arm amend [issue-id] [flags]`

**Flags:**
- `--acceptance string`: Acceptance criteria as JSON array.
- `--dod string`: Definition of done.
- `--issue string`: Issue ID to amend.
- `--scope strings`: File scope globs.
- `--type string`: New type (epic, story, task).

**Example:**
```bash
arm amend TASK-001 --type story --dod "Feature is fully tested and documented."
```

---

## assign

Assign an issue to a worker.

**Synopsis:**
`arm assign [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to assign.
- `--worker string`: Worker ID to assign to.

**Example:**
```bash
arm assign TASK-001 --worker "brian"
```

---

## claim

Claim a ready task.

**Synopsis:**
`arm claim [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to claim.
- `--ttl int`: Claim TTL in minutes (default 60).

**Example:**
```bash
arm claim TASK-001 --ttl 120
```

---

## confirm

Promote an inferred node from draft to verified confidence.

**Synopsis:**
`arm confirm <node-id> [flags]`

**Example:**
```bash
arm confirm STORY-001
```

---

## context-history

Show commits where an issue's context changed.

**Synopsis:**
`arm context-history [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--limit int`: Maximum number of commits to scan (default 100).

**Example:**
```bash
arm context-history --issue TASK-001 --limit 50
```

---

## create

Create a new work item.

**Synopsis:**
`arm create [flags]`

**Flags:**
- `--confidence string`: Confidence level: `draft` or `verified` (default `verified`).
- `--dod string`: Definition of done.
- `--id string`: Explicit ID (auto-generated if empty).
- `--parent string`: Parent node ID.
- `--priority string`: Priority: `critical`, `high`, `medium`, `low`.
- `--scope strings`: File scope globs.
- `--title string`: Item title.
- `--type string`: Item type: `epic`, `story`, `task` (default "task").

**Example:**
```bash
arm create --title "Implement user login" --type story --parent EPIC-001
```

---

## dag-summary

Interactive TUI for reviewing and signing off DAG items.

**Synopsis:**
`arm dag-summary [flags]`

**Flags:**
- `--issue string`: Root issue ID of the subtree to review (default: all draft nodes).

---

## dag-transition

Promote all draft nodes in a subtree to verified.

**Synopsis:**
`arm dag-transition [flags]`

**Flags:**
- `--issue string`: Root issue ID of the subtree to promote.
- `--to string`: Target confidence level (default: `verified`).

---

## decision

Record an architectural decision.

**Synopsis:**
`arm decision [issue-id] [flags]`

**Flags:**
- `--affects strings`: Affected scope globs.
- `--choice string`: Chosen option.
- `--issue string`: Issue ID.
- `--rationale string`: Why this choice.
- `--topic string`: Decision topic.

**Example:**
```bash
arm decision TASK-001 --topic "Database Choice" --choice "PostgreSQL" --rationale "Industry standard and supports JSONB."
```

---

## decompose-apply

Apply a decomposition plan to the issue graph.

**Synopsis:**
`arm decompose-apply [flags]`

**Flags:**
- `--dry-run`: Validate and preview what would be created without writing ops.
- `--example`: Print a minimal valid example plan JSON and exit.
- `--generate-ids`: Replace plan IDs with system-generated UUIDs.
- `--plan string`: Path to plan JSON file.
- `--root string`: Override inferred root: attach top-level plan issues to this existing issue ID.
- `--schema`: Print a JSON Schema document describing the plan format and exit.
- `--strict`: Treat advisory warnings as errors.

**Example:**
```bash
arm decompose-apply --plan plan.json
```

**Example Plan JSON (`--example` output):**
```json
{
  "version": 1,
  "title": "Example Decomposition Plan",
  "issues": [
    {
      "id": "STORY-001",
      "title": "User authentication story",
      "type": "story",
      "scope": "",
      "priority": "",
      "dod": "",
      "parent": "",
      "blocked_by": null,
      "notes": null
    },
    {
      "id": "TASK-001",
      "title": "Implement login endpoint",
      "type": "task",
      "scope": "",
      "priority": "high",
      "dod": "Login endpoint returns JWT on valid credentials",
      "parent": "STORY-001",
      "blocked_by": [],
      "notes": null
    },
    {
      "id": "TASK-002",
      "title": "Write login integration tests",
      "type": "task",
      "scope": "",
      "priority": "medium",
      "dod": "Integration tests cover happy path and error cases",
      "parent": "STORY-001",
      "blocked_by": [
        "TASK-001"
      ],
      "notes": null
    }
  ]
}
```

---

## decompose-context

Build decomposition context with template interpolation.

**Synopsis:**
`arm decompose-context [flags]`

**Flags:**
- `--existing-dag`: Include existing DAG issues in context.
- `--format string`: Output format: `text` or `json` (default "text").
- `--output string`: Write output to file instead of stdout.
- `--plan string`: Path to plan JSON file.
- `--sources string`: Comma-separated source IDs to include.
- `--template string`: Prompt template with placeholders.

---

## decompose-revert

Revert a decomposition plan from the issue graph.

**Synopsis:**
`arm decompose-revert [flags]`

**Flags:**
- `--plan string`: Path to plan JSON file.

---

## doctor

Run repository health checks (D1-D6).

**Synopsis:**
`arm doctor [flags]`

**Flags:**
- `--strict`: Promote warnings to errors.

---

## heartbeat

Send heartbeat for an active claim.

**Synopsis:**
`arm heartbeat [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID.

---

## import

Import issues from a CSV or JSON file.

**Synopsis:**
`arm import <file> [flags]`

**Flags:**
- `--dry-run`: Show what would be imported without writing ops.
- `--source string`: Source ID to link imported items to.

---

## init

Initialize Armature in the current repository.

**Synopsis:**
`arm init [flags]`

**Flags:**
- `--dual-branch`: Initialize in dual-branch mode (issues stored on separate `_armature` branch).

---

## link

Add a dependency link between issues.

**Synopsis:**
`arm link [flags]`

**Flags:**
- `--dep string`: Dependency issue ID.
- `--rel string`: Relationship type (default "blocked_by").
- `--source string`: Source issue ID.

---

## list

List issues with optional filters. In non-TTY environments (agent context) the output is a JSON array automatically.

**Synopsis:**
`arm list [flags]`

**Flags:**
- `--group`: Group issues under `=== STATUS ===` section headers sorted by workflow priority (human output only).
- `--parent string`: Filter by parent issue ID.
- `--status string`: Filter by status: `open`, `in-progress`, `done`, `merged`, `cancelled`, `blocked`.
- `--type string`: Filter by issue type: `task`, `story`, `feature`, `bug`.

**Examples:**
```bash
# Flat list — in agent context this is JSON automatically
arm list --status done
arm list --status open --parent STORY-001

# Grouped human overview
arm list --group
arm list --group --parent EPIC-001
```

---

## log

Show the audit log of ops.

**Synopsis:**
`arm log [flags]`

**Flags:**
- `--issue string`: Filter by issue ID.
- `--json`: Output as JSONL.
- `--since string`: Filter entries since this time (RFC3339 or YYYY-MM-DD).
- `--worker string`: Filter by worker ID.

---

## materialize

Replay op logs and update materialized state files.

**Synopsis:**
`arm materialize [flags]`

**Flags:**
- `--exclude-worker string`: Skip all ops from this worker ID.

---

## merged

Mark a done issue as merged.

**Synopsis:**
`arm merged [flags]`

**Flags:**
- `--issue string`: Issue ID.
- `--pr string`: PR number or URL.

---

## note

Add a note to an issue.

**Synopsis:**
`arm note [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID.
- `--msg string`: Note message.

**Example:**
```bash
arm note TASK-001 --msg "Started implementation after architectural review."
```

---

## ready

Show tasks ready to be claimed.

**Synopsis:**
`arm ready [flags]`

**Flags:**
- `--parent string`: Filter to descendants of this issue ID.
- `--worker string`: Worker ID for assignment-aware sorting.

---

## render-context

Render assembled context for an issue.

**Synopsis:**
`arm render-context [issue-id] [flags]`

**Flags:**
- `--at string`: Replay context as of this git commit SHA.
- `--budget int`: Token budget (default 4000).
- `--issue string`: Issue ID.
- `--raw`: Skip truncation.

---

## reopen

Reopen a done or blocked issue.

**Synopsis:**
`arm reopen [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to reopen.

---

## show

Show a human-readable summary of one or more issues.

**Synopsis:**
`arm show [issue-id ...] [flags]`

**Flags:**
- `--field string`: Extract a single field value (e.g. `status`, `title`).
- `--issue string`: Issue ID to show.

---

## source-link

Link an issue to a source entry in the manifest.

**Synopsis:**
`arm source-link [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to link.
- `--source-id string`: UUID of the source entry in the manifest.

---

## sources

Manage external knowledge sources.

**Synopsis:**
`arm sources [command]`

**Available Subcommands:**
- `add`: Add a new source to the manifest.
- `sync`: Fetch and cache content for all sources.
- `verify`: Verify cached content matches stored fingerprints.

**Example:**
```bash
arm sources add --url "https://example.com/docs" --type filesystem
```

---

## scope-delete

Remove an exact file path from all issue scopes.

**Synopsis:**
`arm scope-delete <path>`

**Behaviour:**
- Rejects an empty `path` argument with an error.
- If no issue has an exact scope entry matching `path`, prints a warning and exits 0 without writing any ops.
- If any non-terminal issue (status not in `merged`, `done`, `cancelled`) would be left with an empty scope after deletion, prints a warning listing those issue IDs; the command proceeds regardless.
- Only exact-string entries are removed; glob entries that happen to cover the deleted file are left intact (use `arm amend --scope` to update them manually).
- Emits one `scope-delete` op per affected issue, all at the same timestamp, then rematerializes.

**Example:**
```bash
arm scope-delete cmd/trellis/main.go
```

---

## scope-rename

Rename a scope path across all issues using substring matching.

**Synopsis:**
`arm scope-rename <old-path> <new-path>`

**Behaviour:**
- Rejects an empty `old-path` or `new-path` argument with an error.
- Rejects identical `old-path` and `new-path` with an error.
- If no issue has a scope entry containing `old-path` as a substring, prints a warning and exits 0 without writing any ops.
- Prints a summary of affected issues (count and IDs) before writing ops.
- `old-path` is a substring match, so a directory prefix correctly updates both exact paths and glob patterns in a single op (e.g. `old-path=cmd/trellis` rewrites `cmd/trellis/main.go` and `cmd/trellis/*.go`).
- Emits one `scope-rename` op per affected issue, all at the same timestamp, then rematerializes.
- Idempotent: a second application finds nothing matching `old-path` and is a no-op.

**Examples:**
```bash
# Rename a single file
arm scope-rename cmd/trellis/main.go cmd/armature/main.go

# Rename a directory prefix (updates exact paths and globs)
arm scope-rename cmd/trellis cmd/armature
```

---

## stale-review

Review sources whose cached content has changed since last sync.

**Synopsis:**
`arm stale-review [flags]`

---

## sync

Detect merged branches and auto-transition done issues to merged.

**Synopsis:**
`arm sync [flags]`

**Flags:**
- `--into string`: Target branch to check merges against (default: current branch).

---

## transition

Transition an issue to a new status.

**Synopsis:**
`arm transition [issue-id] [flags]`

**Flags:**
- `--branch string`: Feature branch name.
- `--issue string`: Issue ID.
- `--outcome string`: Outcome description.
- `--pr string`: PR number.
- `--to string`: Target status.

**Example:**
```bash
arm transition TASK-001 --to in-progress --branch feature/login
```

---

## tui

Interactive kanban board with auto-refresh.

**Synopsis:**
`arm tui [flags]`

---

## unassign

Remove worker assignment from an issue.

**Synopsis:**
`arm unassign [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to unassign.

---

## validate

Validate the issue graph for consistency.

**Synopsis:**
`arm validate [flags]`

**Flags:**
- `--ci`: Exit non-zero if errors found.
- `--scope string`: Validate only the subtree rooted at this node ID.
- `--strict`: Treat warnings as errors.

---

## version

Print `arm` version.

**Synopsis:**
`arm version [flags]`

---

## worker-init

Generate or check worker identity.

**Synopsis:**
`arm worker-init [flags]`

**Flags:**
- `--check`: Verify existing worker ID without modifying state.

---

## workers

Show worker activity status.

**Synopsis:**
`arm workers [flags]`

**Flags:**
- `--json`: Output as JSONL.
