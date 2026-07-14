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

Claim a ready task and associate it with a git worktree.

**Synopsis:**
`arm claim [issue-id] [flags]`

**Flags:**
- `--force`: Override scope overlap warning and proceed with claim.
- `--issue string`: Issue ID to claim.
- `--ttl int`: Claim TTL in minutes (default 60).
- `--worktree string`: Path to task worktree (required). Creates a new git worktree and a derived branch (`task/<id>`, `fix/<id>`, or `feat/<id>`) if the path does not exist. Writes the task ID to `<worktree-git-dir>/armature-issue-id` so the harness hook can read it without an environment variable.

**Example:**
```bash
arm claim TASK-001 --worktree ./task-001-work
arm claim TASK-001 --ttl 120 --worktree ./task-001-work
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
- `--verbose`: Show file:line context for D3 violations; name uncited issue IDs for D6.

**Doctor Checks:**
See [Validation & Doctor Codes Reference](./validation-codes.md) for complete documentation of all doctor checks (D1–D6), their triggers, and remediation steps.

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

## bootstrap

Bootstrap Armature: initialize repository and deploy harness artifacts.

**Synopsis:**
`arm bootstrap [flags]`

**Description:**
Initializes a repository for Armature coordination and optionally deploys harness artifacts
(skills, plugin metadata, harness hook configs).

By default, artifacts deploy to `.claude/` (local). Use `--global` to deploy to `~/.claude/` instead.
Use `--with-hooks` to also write harness hook configuration (both require `--platform` support).
Use `--platform` to restrict bootstrap to specific platforms (can be repeated); default is all verified platforms.

The command is idempotent: running it multiple times has the same effect as running it once.

**Flags:**
- `--global`: Deploy to `~/.claude/` instead of `.claude/` (local).
- `--platform string`: Restrict to specific platform(s) (can be repeated; default: all verified platforms).
- `--with-hooks`: Also write harness hook configuration.
- `--repo string`: Repository path (default: current directory).

**Default Behavior:**
When run without flags, `arm bootstrap` performs repo setup (`.armature/` directory structure, ops logs,
git hooks, worker identity) and deploys bundled skills to `.claude/skills/` locally.

**Examples:**
```bash
# Initialize Armature in the current repo
arm bootstrap

# Deploy skills globally
arm bootstrap --global

# Deploy only Claude Code platform harness config with hooks
arm bootstrap --platform claude --with-hooks

# Specify repo path explicitly
arm bootstrap --repo /path/to/project
```

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

Mark a done issue as merged after its branch or PR has landed on the main branch.

For task, bug, and feature issues that were claimed with `--worktree`, this command also:
1. Tears down the associated git worktree (if still present).
2. Warns to stderr if the worktree's `armature-hook.log` contains pass-through entries
   (hooks that fired without an active task binding).

**Synopsis:**
`arm merged [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--pr string`: PR number or URL.

**Example:**
```bash
arm merged --issue TASK-001
arm merged --issue TASK-001 --pr 42
```

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

## harness-hook

Internal hook entrypoint used by harness-native guardrails.

**Synopsis:**
`arm harness-hook`

**Behavior:**
- Selects the platform adapter from `ARMATURE_HOOK_PLATFORM`.
- Decodes hook event JSON from stdin.
- Resolves the issue binding via the ADR-0007 4-step chain, most specific first:
  1. `tool_input.file_path` — walk up from the target file to the containing worktree's git dir and its `armature-issue-id` file;
  2. event-payload `cwd` — same walk-up from the per-agent working directory reported by the harness;
  3. hook process cwd — the session's own worktree binding (`<git-dir>/armature-issue-id`);
  4. `ARMATURE_ISSUE_ID` environment variable — last-resort fallback.
- Resolves task scope, acceptance, and citation policy from Armature state.
- Returns a platform-native allow or block decision; all internal failures fail open (pass through with a stderr warning).

**Environment:**
- `ARMATURE_HOOK_PLATFORM` (required): one of `claude`, `codex`, or `devin`.
- `ARMATURE_ISSUE_ID` (optional): last-resort binding fallback (step 4 above) for harnesses without worktree support.

See [Harness Hook Integration Guide](./harness-hook.md) for the full resolution and logging model.

**Notes:**
- This command is an internal integration surface, not a user-facing queue runner.
- Hook configuration is generated by the platform adapters under `internal/harnesshook`.
- External workers launch their harness normally and call `arm harness-hook` through the harness's native hook mechanism.

**See Also:**
See [Harness Hook Integration Guide](./harness-hook.md) for complete setup instructions for Claude Code, Codex, and Devin, including example configurations and troubleshooting.

---

## ready

Show tasks ready to be claimed.

**Synopsis:**
`arm ready [flags]`

**Flags:**
- `--parent string`: Filter to descendants of this issue ID.
- `--worker string`: Worker ID for assignment-aware sorting.

**Queue Inspection:**
Use `ready` to find unblocked tasks before claiming:

1. `arm ready`
2. `arm claim <issue-id>`
3. `arm render-context <issue-id>`
4. Launch the external worker or harness outside Armature

Claim collisions are expected under concurrency; losing workers simply call `arm ready` again.

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

## review prepare

Create a semantic review bundle for an issue from its issue contract and delivery diff.

**Synopsis:**
`arm review prepare [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--base string`: Base git SHA (required); typically the commit at wave start.
- `--head string`: Head git SHA (required); typically the commit after workers complete.
- `--format string`: Output format: `json`, `agent` (default "json").

**Description:**
Captures the issue's acceptance criteria, scope, and the unified diff between `--base` and `--head`. Outputs a `ReviewBundle` JSON object containing the issue contract and delivery diff. This bundle is passed to the `armature-reviewer` skill for semantic conformance assessment.

**Example:**
```bash
arm review prepare --issue TASK-001 --base abc123 --head def456
```

---

## review record

Record a semantic conformance assessment for a completed task.

**Synopsis:**
`arm review record [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--assessment string`: Path to assessment JSON file (required).

**Description:**
Persists the output of semantic review (a `ConformanceAssessment`) to the issue's audit log. The assessment includes a structured rating (green/yellow/red) and detailed findings. This operation links the reviewer's judgment to the task and updates its review status.

**Example:**
```bash
arm review record --issue TASK-001 --assessment assessment.json
```

---

## review commits

List delivery commits for an issue across all conventional-commit types.

**Synopsis:**
`arm review commits <issue-id> [flags]`

**Flags:**
- `--issue string`: Issue ID (alternative to positional argument).
- `--branch string`: Branch to scan for commits (default `HEAD`). Point this at `task/TASK-ID` or
  a story branch to find a task's commits before merge, e.g. from a worktree whose parent repo
  has a different branch checked out.

**Description:**
Discovers and lists all commits on the given branch (default: the currently checked-out branch)
that reference the issue ID in their conventional-commit-style scope (e.g., `feat(ISSUE-ID): ...`,
`fix(ISSUE-ID): ...`, etc.). Breaking-change syntax (`feat(ISSUE-ID)!: ...`) is also matched.

This command scans commit messages across all type prefixes (feat, fix, refactor, test, docs, chore),
replacing the coordinator skill's feat-only grep pseudocode which silently dropped other commit types.

**Example:**
```bash
arm review commits TASK-001
arm review commits --issue TASK-001 --format json
arm review commits TASK-001 --branch task/TASK-001
```

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

**Sandbox Note (Codex/agent sessions):**
- In some sandboxed sessions, `arm transition` may fail with:
  `Unable to create .../.git/worktrees/.../index.lock: Read-only file system`.
- This is a sandbox lockfile restriction on nested git writes, not an issue-graph bug.
- Re-run the same command with elevated approval so git can write worktree locks.
- If this happens repeatedly, approve the command prefix:
  `go run ./cmd/armature transition`
  so future transitions work without re-troubleshooting.

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
- `--quiet`: Suppress INFO lines; still print COVERAGE and OK lines.

**Validation Codes:**
See [Validation & Doctor Codes Reference](./validation-codes.md) for complete documentation of all error codes (E2–E10, E12), warnings (W1–W8, W10–W11), and their fixes.

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

---

## Environment Variables

### ARM_LOG_SLOT

**Type:** string

**Behavior:**
Appends `~<slot>` to the worker UUID, forming the log filename. For example, a worker with UUID `abc123` and `ARM_LOG_SLOT=2` writes to `ops/abc123~2.log` instead of `ops/abc123.log`.

**Purpose:**
Enables parallel agent dispatch while preserving the single-writer-per-log invariant (MRDT). Each concurrent process must have a distinct slot to avoid log conflicts when multiple workers with the same identity run simultaneously.

**Usage:**
Export before invoking any `arm` command:

```bash
export ARM_LOG_SLOT=<n>
arm <command> [flags]
```

**Example (two-agent parallel dispatch):**
```bash
# Agent 1: slot 0
export ARM_LOG_SLOT=0
arm claim TASK-001 &

# Agent 2: slot 1
export ARM_LOG_SLOT=1
arm claim TASK-002 &

wait
```

In this scenario, both agents operate with the same worker ID but write to separate log files (`ops/worker-id~0.log` and `ops/worker-id~1.log`), preventing conflicts.
