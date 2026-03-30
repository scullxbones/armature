# Trellis Command Reference

`trls` is a git-native work orchestration tool. This document provides a complete reference for every `trls` subcommand.

## Global Flags

The following flags are available for all commands:

- `--debug`: Dump debug diagnostics on error.
- `--format string`: Output format: `human`, `json`, `agent` (default "human").
- `--repo string`: Repository path (default: current directory).

---

## accept-citation

Accept a citation for an issue with a recorded rationale.

**Synopsis:**
`trls accept-citation [issue-id] [flags]`

**Flags:**
- `--ci`: Bypass interactive prompt (non-interactive/CI mode).
- `--issue string`: Issue ID to accept citation for.
- `--rationale string`: Rationale for accepting the citation (>=3 words).

**Example:**
```bash
trls accept-citation E5-S4-T3 --rationale "Documentation is complete and reviewed."
```

---

## amend

Amend fields on an existing issue.

**Synopsis:**
`trls amend [issue-id] [flags]`

**Flags:**
- `--acceptance string`: Acceptance criteria as JSON array.
- `--dod string`: Definition of done.
- `--issue string`: Issue ID to amend.
- `--scope strings`: File scope globs.
- `--type string`: New type (epic, story, task).

**Example:**
```bash
trls amend TASK-001 --type story --dod "Feature is fully tested and documented."
```

---

## assign

Assign an issue to a worker.

**Synopsis:**
`trls assign [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to assign.
- `--worker string`: Worker ID to assign to.

**Example:**
```bash
trls assign TASK-001 --worker "brian"
```

---

## claim

Claim a ready task.

**Synopsis:**
`trls claim [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to claim.
- `--ttl int`: Claim TTL in minutes (default 60).

**Example:**
```bash
trls claim TASK-001 --ttl 120
```

---

## confirm

Promote an inferred node from draft to verified confidence.

**Synopsis:**
`trls confirm <node-id> [flags]`

**Example:**
```bash
trls confirm STORY-001
```

---

## context-history

Show commits where an issue's context changed.

**Synopsis:**
`trls context-history [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--limit int`: Maximum number of commits to scan (default 100).

**Example:**
```bash
trls context-history --issue TASK-001 --limit 50
```

---

## create

Create a new work item.

**Synopsis:**
`trls create [flags]`

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
trls create --title "Implement user login" --type story --parent EPIC-001
```

---

## dag-summary

Interactive TUI for reviewing and signing off DAG items.

**Synopsis:**
`trls dag-summary [flags]`

**Flags:**
- `--issue string`: Root issue ID of the subtree to review (default: all draft nodes).

---

## dag-transition

Promote all draft nodes in a subtree to verified.

**Synopsis:**
`trls dag-transition [flags]`

**Flags:**
- `--issue string`: Root issue ID of the subtree to promote.
- `--to string`: Target confidence level (default: `verified`).

---

## decision

Record an architectural decision.

**Synopsis:**
`trls decision [issue-id] [flags]`

**Flags:**
- `--affects strings`: Affected scope globs.
- `--choice string`: Chosen option.
- `--issue string`: Issue ID.
- `--rationale string`: Why this choice.
- `--topic string`: Decision topic.

**Example:**
```bash
trls decision TASK-001 --topic "Database Choice" --choice "PostgreSQL" --rationale "Industry standard and supports JSONB."
```

---

## decompose-apply

Apply a decomposition plan to the issue graph.

**Synopsis:**
`trls decompose-apply [flags]`

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
trls decompose-apply --plan plan.json
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
`trls decompose-context [flags]`

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
`trls decompose-revert [flags]`

**Flags:**
- `--plan string`: Path to plan JSON file.

---

## doctor

Run repository health checks (D1-D6).

**Synopsis:**
`trls doctor [flags]`

**Flags:**
- `--strict`: Promote warnings to errors.

---

## heartbeat

Send heartbeat for an active claim.

**Synopsis:**
`trls heartbeat [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID.

---

## import

Import issues from a CSV or JSON file.

**Synopsis:**
`trls import <file> [flags]`

**Flags:**
- `--dry-run`: Show what would be imported without writing ops.
- `--source string`: Source ID to link imported items to.

---

## init

Initialize Trellis in the current repository.

**Synopsis:**
`trls init [flags]`

**Flags:**
- `--dual-branch`: Initialize in dual-branch mode (issues stored on separate `_trellis` branch).

---

## link

Add a dependency link between issues.

**Synopsis:**
`trls link [flags]`

**Flags:**
- `--dep string`: Dependency issue ID.
- `--rel string`: Relationship type (default "blocked_by").
- `--source string`: Source issue ID.

---

## list

List issues with optional `--type` and `--parent` filters.

**Synopsis:**
`trls list [flags]`

**Flags:**
- `--parent string`: Filter by parent issue ID.
- `--type string`: Filter by issue type (task, story, feature, bug).

**Example:**
```bash
trls list --type story --parent EPIC-001
```

---

## log

Show the audit log of ops.

**Synopsis:**
`trls log [flags]`

**Flags:**
- `--issue string`: Filter by issue ID.
- `--json`: Output as JSONL.
- `--since string`: Filter entries since this time (RFC3339 or YYYY-MM-DD).
- `--worker string`: Filter by worker ID.

---

## materialize

Replay op logs and update materialized state files.

**Synopsis:**
`trls materialize [flags]`

**Flags:**
- `--exclude-worker string`: Skip all ops from this worker ID.

---

## merged

Mark a done issue as merged.

**Synopsis:**
`trls merged [flags]`

**Flags:**
- `--issue string`: Issue ID.
- `--pr string`: PR number or URL.

---

## note

Add a note to an issue.

**Synopsis:**
`trls note [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID.
- `--msg string`: Note message.

**Example:**
```bash
trls note TASK-001 --msg "Started implementation after architectural review."
```

---

## ready

Show tasks ready to be claimed.

**Synopsis:**
`trls ready [flags]`

**Flags:**
- `--parent string`: Filter to descendants of this issue ID.
- `--worker string`: Worker ID for assignment-aware sorting.

---

## render-context

Render assembled context for an issue.

**Synopsis:**
`trls render-context [issue-id] [flags]`

**Flags:**
- `--at string`: Replay context as of this git commit SHA.
- `--budget int`: Token budget (default 4000).
- `--issue string`: Issue ID.
- `--raw`: Skip truncation.

---

## reopen

Reopen a done or blocked issue.

**Synopsis:**
`trls reopen [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to reopen.

---

## show

Show a human-readable summary of a single issue.

**Synopsis:**
`trls show [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to show.

---

## source-link

Link an issue to a source entry in the manifest.

**Synopsis:**
`trls source-link [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to link.
- `--source-id string`: UUID of the source entry in the manifest.

---

## sources

Manage external knowledge sources.

**Synopsis:**
`trls sources [command]`

**Available Subcommands:**
- `add`: Add a new source to the manifest.
- `sync`: Fetch and cache content for all sources.
- `verify`: Verify cached content matches stored fingerprints.

**Example:**
```bash
trls sources add --url "https://example.com/docs" --type filesystem
```

---

## stale-review

Review sources whose cached content has changed since last sync.

**Synopsis:**
`trls stale-review [flags]`

---

## status

Show issues grouped by status.

**Synopsis:**
`trls status [flags]`

---

## sync

Detect merged branches and auto-transition done issues to merged.

**Synopsis:**
`trls sync [flags]`

**Flags:**
- `--into string`: Target branch to check merges against (default: current branch).

---

## transition

Transition an issue to a new status.

**Synopsis:**
`trls transition [issue-id] [flags]`

**Flags:**
- `--branch string`: Feature branch name.
- `--issue string`: Issue ID.
- `--outcome string`: Outcome description.
- `--pr string`: PR number.
- `--to string`: Target status.

**Example:**
```bash
trls transition TASK-001 --to in_progress --branch feature/login
```

---

## tui

Interactive kanban board with auto-refresh.

**Synopsis:**
`trls tui [flags]`

---

## unassign

Remove worker assignment from an issue.

**Synopsis:**
`trls unassign [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to unassign.

---

## validate

Validate the issue graph for consistency.

**Synopsis:**
`trls validate [flags]`

**Flags:**
- `--ci`: Exit non-zero if errors found.
- `--scope string`: Validate only the subtree rooted at this node ID.
- `--strict`: Treat warnings as errors.

---

## version

Print `trls` version.

**Synopsis:**
`trls version [flags]`

---

## worker-init

Generate or check worker identity.

**Synopsis:**
`trls worker-init [flags]`

**Flags:**
- `--check`: Verify existing worker ID without modifying state.

---

## workers

Show worker activity status.

**Synopsis:**
`trls workers [flags]`

**Flags:**
- `--json`: Output as JSONL.
