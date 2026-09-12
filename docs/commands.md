# Armature Command Reference

`arm` is a git-native work orchestration tool. This document provides a complete reference for every `arm` subcommand.

## Global Flags

The following flags are available for all commands:

- `--debug`: Dump debug diagnostics on error.
- `--format string`: Output format: `human`, `json`, `agent` (default "human").
- `--repo string`: Repository path (default: current directory).
- `--non-interactive`: Skip TUI and emit structured output (auto-set when --format=agent or non-TTY).

Unknown or inapplicable flags fail by name at exit 2 (`USAGE`) and list the valid flags for that command.

Root-only (not inherited by subcommands): `--version`, `-v`, and `-V` are the same version query as `arm version`.

With no subcommand, a TTY still prints the command listing. A non-TTY prints the ready queue when `--repo` (or cwd) is an Armature repository. Snapshot warnings from that home view go to stderr, the same as `arm ready`. Absence of an Armature layout (no `armature.ops-worktree-path`) is a successful empty `{count, issues, help}` envelope whose `help` names the missing layout. Config missing, malformed, or unreadable — and other context-resolution failures — remain non-zero Command Failures. Unknown root arguments (for example `arm orchestrate`) are unknown-command errors.

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
It regenerates ops scaffolding on every run, so a later ops `FetchAndRebase` is not
blocked by dirty tracked scaffolding. Only `.gitignore` and `ops/SCHEMA` are committed
on `_armature`, and only when they differ from HEAD. Both propagate monotonically:
each carries a scaffolding-version header, and a clone rewrites them only when its
own `arm` is at least that new, so an older binary reads them rather than committing
a downgrade. Bump `ops.ScaffoldingVersion` whenever either generator's output changes.

Hook templates are local-only, regenerated from the running binary on every bootstrap
and read only by the bootstrap that wrote them. Gate logs and reviewer assessment JSON
under `.armature/gates/` and `.armature/review/` are local-only too. All three are
gitignored, and copies committed by earlier versions are untracked on the next
bootstrap, keeping the local files.

**Flags:**
- `--global`: Deploy to `~/.claude/` instead of `.claude/` (local).
- `--platform string`: Restrict to specific platform(s) (can be repeated; default: all verified platforms).
- `--with-hooks`: Also write harness hook configuration.
- `--repo string`: Repository path (default: current directory).

**Examples:**
```bash
# Initialize Armature in the current repo
arm bootstrap

# Deploy skills globally
arm bootstrap --global

# Deploy only Claude Code platform harness config with hooks
arm bootstrap --platform claude --with-hooks
```

---

## claim

Claim a ready task and associate it with a git worktree.

**Synopsis:**
`arm claim [issue-id] [flags]`

**Flags:**
- `--force`: Override scope overlap warning and proceed with claim.
- `--from string`: Seed an explicit new `--worktree <path>` from the current branch and tip of this parent worktree.
- `--issue string`: Issue ID to claim.
- `--ttl int`: Claim TTL in minutes (default 60).
- `--worktree [new-path]`: Enable worktree provisioning (required). Without a value, provisions at `.worktrees/<issue-id>`; with an explicit path, provisions there instead. Both forms create a binding-managed worktree and a derived branch (`task/<id>`, `fix/<id>`, or `feat/<id>`).

**Example:**
```bash
arm claim TASK-001 --worktree
arm claim TASK-001 --worktree ./child --from ./parent
arm claim TASK-001 --ttl 120 --worktree

# Create a sub-task worktree from the current story worktree
arm claim TASK-002 --worktree /path/to/new-task-worktree --from /path/to/story-worktree
```

---

## completion

Generate shell completion script.

**Synopsis:**
`arm completion [bash|zsh|fish|powershell]`

---

## confirm

Promote an inferred node from draft to verified confidence.

Plan release requires a strict-green `arm validate` of the whole graph.
Happy-path failures name the Graph Finding and withdraw-the-draft
(`arm dag revert` / `arm transition --to cancelled`).

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
- `--limit int`: Maximum number of commits to scan. Default is complete history (`0`). A positive `--limit` is a disclosed query boundary; `--limit 0` (and other nonpositive values) is unbounded.

Structured output (`--format json`, `--format agent`) is one Agent Output Contract envelope: `{count, commits, help}`. Without `--limit`, or with a nonpositive `--limit`, the command scans complete history and does not disclose a `limit` adjunct. A positive `--limit N` bounds the scan; `count` equals the matching commits inside that window, and the envelope discloses `limit` plus a help line that the result is bounded.

**Example:**
```bash
arm context-history --issue TASK-001 --limit 50
```

---

## context-report

Price fixture-measured structured stdout for main-path CLI commands.

**Synopsis:**
`arm context-report`

**Description:**
Measures json/agent payload bytes for `list`, `ready`, `show`,
`render-context`, and `review` against the fixture graph embedded in the
binary. `list`, `ready`, and `show` are compact AOC envelopes from the live
writers. Also prices a fixture `render-context` bundle. Estimated tokens use
bytes/4 (integer division), matching the `token_budget` convention. `--repo`
is not required.
Non-TTY invocations without `--format` follow the shared TTY auto-detect
(agent/JSON). `make context-report` passes `--format human` explicitly.

**Example:**
```bash
arm context-report --format json
make context-report
```

---

## create

Create a new work item.

**Synopsis:**
`arm create [flags]`

**Flags:**
- `--confidence string`: Ignored. Birth is always draft.
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

## dag

Manage the Directed Acyclic Graph (DAG) of issues.

**Synopsis:**
`arm dag [command]`

**Subcommands:**

### dag apply

Apply a decomposition plan to the issue graph.

Every issue in the plan must include a `source` (source entry ID). Apply is
source-atomic: each create is emitted with its source-link in the same batch.
Writes that would introduce a Graph Finding on a targeted issue are refused.
`--dry-run` is how a planner iterates.

`--schema` and `--example` remain Artifact Output (the plan schema and a sample
plan, written verbatim). Ordinary apply and `--dry-run` are agent-facing:
structured output is `{count, issues, help}` with each row's `action` set to
`created` or `would_create`. Human output is unchanged.

**Synopsis:**
`arm dag apply [flags]`

**Flags:**
- `--dry-run`: Validate and preview what would be created without writing ops.
- `--example`: Print a minimal valid example plan JSON and exit.
- `--generate-ids`: Replace plan IDs with system-generated UUIDs.
- `--plan string`: Path to plan JSON file.
- `--root string`: Override inferred root: attach top-level plan issues to this existing issue ID.
- `--schema`: Print a JSON Schema document describing the plan format and exit.

**Example:**
```bash
arm dag apply --plan plan.json
arm dag apply --plan plan.json --dry-run
```

**Example Plan JSON:**
```json artifact_type=plan
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

### dag context

Build decomposition context with template interpolation.

**Synopsis:**
`arm dag context [flags]`

**Flags:**
- `--existing-dag`: Include existing DAG issues in context.
- `--format string`: Output format: `text` or `json` (default "text").
- `--output string`: Write output to file instead of stdout.
- `--plan string`: Path to plan JSON file.
- `--sources string`: Comma-separated source IDs to include.
- `--template string`: Prompt template with placeholders.

---

### dag revert

Revert a decomposition plan from the issue graph.

**Synopsis:**
`arm dag revert [flags]`

**Flags:**
- `--plan string`: Path to plan JSON file.

---

### dag summary

Interactive TUI for reviewing and signing off DAG items.

**Synopsis:**
`arm dag summary [flags]`

**Flags:**
- `--approve-all`: Auto-approve all pending draft items in agent mode.
- `--issue string`: Root issue ID of the subtree to review (default: all draft nodes).

---

### dag transition

Promote all draft nodes in a subtree to verified.

**Synopsis:**
`arm dag transition [flags]`

Promotion to `verified` (plan release) requires a strict-green `arm validate` of
the whole graph so a planner cannot release a dirty plan. Demotion to `draft`
is not gated. Happy-path failures name the Graph Finding and withdraw-the-draft
(`arm dag revert` / `arm transition --to cancelled`).

**Flags:**
- `--issue string`: Root issue ID of the subtree to promote.
- `--to string`: Target confidence level (default: `verified`).

---

### dag override-release

Record a human Release Override for a draft subtree. Requires a controlling
terminal, an interactive type-the-id, and a recorded `--reason`. The op sets
`skipped_validate_gate` plus the reason. Success is never "green."

**Synopsis:**
`arm dag override-release <issue-id> --reason <text>`

**Flags:**
- `--reason string`: Recorded reason for the override (required).

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

## doctor

Run repository health checks (D1-D10).

**Synopsis:**
`arm doctor [flags]`

**Flags:**
- `--strict`: Promote warnings to errors.
- `--verbose`: Show file:line context for D3 violations; name uncited issue IDs for D6.
- `--fix`: Deterministically reconcile expired claims per the claim-liveness matrix in
  [docs/design/recovery-state-machine.md](./design/recovery-state-machine.md): a `claimed`
  issue with an expired TTL is released back to `open`; an `in-progress` issue with an
  expired TTL is transitioned to `blocked` pending manual investigation (the worker may
  have left in-flight work). Each fix is appended as new ops — the append-only op log is
  never rewritten. Re-running `--fix` after applying it is a no-op, since the affected
  issues are no longer claimed/in-progress with an expired claim.
- `--dry-run`: With `--fix`, list the planned fixes without writing any ops.

**Examples:**
```bash
arm doctor --fix --dry-run   # preview planned reconciliation
arm doctor --fix             # apply it
```

**Doctor Checks:**
See [Validation & Doctor Codes Reference](./validation-codes.md) for complete documentation of all doctor checks (D1–D6), their triggers, and remediation steps.

**D9 — Unrecognized managed worktrees.** A worktree living under the managed
`.worktrees/` root that carries no issue binding is reported as a health problem
(warning). Because it is a warning, plain `arm doctor` still exits 0 but flags it,
and `arm doctor --strict` promotes it to an error and exits non-zero — so an agent
cannot proceed past an unrecognized managed worktree. This enforces the anomaly
rather than merely printing it. Note that `arm worktree list` deliberately keeps
exit code 0 on the same anomaly: it is the routine inventory command that exists to
report it, so it must not fail on what it is designed to surface. Remediate by
binding the worktree to its issue or removing the stray checkout.

**D10 — Config health.** `.armature/config.json` must decode strictly (unknown
fields rejected by name) and every present field must be in range. A missing
file fails open. Malformed JSON, unknown keys, a zero `low_stakes_push_threshold`,
an empty hook/gate executable, or a `default_ttl` that would overflow claim
staleness arithmetic are errors, so `arm doctor` exits non-zero.

---

## gate

Run a repository-configured quality-gate profile and record wrapper-observed evidence.

**Synopsis:**
`arm gate run <profile>`

**Subcommands:**

### gate run

Execute the command declared for `<profile>` in the blob at `HEAD:gates.json` of the invoking checkout (`--repo` or the current directory — not the parent repo `ResolveContext` walks to, not the ops worktree, not `.armature/config.json`, and not the worktree file, which skip-worktree can hide). Stream stdout/stderr to a log file under `.armature/gates/`, and append a `gate-evidence` op to the invoking worker's own log (`profile`, `command`, `head_sha`, `start`, `end`, `exit`, `output_hash`, `output_head`, `output_tail`, `log_path`). Profile name `full` is reserved as the publish profile. A working tree that is dirty before the command, or that the command dirties, still executes but records the run as `uncommitted` (not citable). Repos with no `HEAD:gates.json` (or an empty map) get a clear error — armature does not infer make or Go. A porcelain `!!` path is exempt only when `git check-ignore -v --no-index` names a source that is a tracked file at HEAD (typically a committed `.gitignore`). `.git/info/exclude`, `core.excludesFile`, and XDG/global gitignore never appear in `Delivery.Diff` and mark the run `uncommitted`. The dirty check recurses into populated submodules with `--ignore-submodules=none --untracked-files=all`. A gitlink directory that has files but no `.git` is dirty. skip-worktree and assume-unchanged (`git ls-files -v` tags `S` or lowercase) are walked in the superproject and populated submodules. The gate git client pins `--work-tree` / `--git-dir` from the filesystem `.git` so `GIT_WORK_TREE`, `GIT_DIR`, and `core.worktree` cannot point the check at a clean export.

**Synopsis:**
`arm gate run <profile>`

**Example:**
```bash
arm gate run full
arm gate run fast
```

---

## heartbeat

Send heartbeat for an active claim.

**Synopsis:**
`arm heartbeat [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID.

---

## harness-hook

Internal hook entrypoint used by harness-native guardrails.

**Synopsis:**
`arm harness-hook`

**Behavior:**
- Selects the platform adapter from `ARMATURE_HOOK_PLATFORM`.
- Decodes hook event JSON from stdin.
- Resolves the issue binding via the ADR-0007 4-step chain, most specific first.
- Returns a platform-native allow or block decision; all internal failures fail open (pass through with a stderr warning).

**Environment:**
- `ARMATURE_HOOK_PLATFORM` (required): one of `claude`, `codex`, or `devin`.
- `ARMATURE_ISSUE_ID` (optional): last-resort binding fallback.

See [Harness Hook Integration Guide](./harness-hook.md) for the full resolution and logging model.

---

## import

Import issues from a CSV or JSON file.

**Synopsis:**
`arm import <file> [flags]`

**Flags:**
- `--dry-run`: Show what would be imported without writing ops.
- `--source string`: Source ID to link imported items to.

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

List issues with optional filters. Structured output (`--format json`, `--format agent`, and the non-TTY default) is one Agent Output Contract envelope object on stdout: `{count, issues, help}`. It is never a bare array.

Default `issues[]` rows have exactly four string keys: `id`, `type`, `status`, `title`. `outcome` is omitted entirely (not truncated). Use `arm show <id>` for outcome, scope, and acceptance. `count` is the true length of `issues` and is not capped.

`--group` is honoured in structured output. `issues` stays the flat complete list. A `groups` adjunct lists status buckets in workflow order, each `{status, ids}`. Human format still prints `=== STATUS ===` section headers.

An empty match is still the envelope: `{count: 0, "issues": [], "help": [...]}` with exit 0.

**Synopsis:**
`arm list [flags]`

**Flags:**
- `--group`: Group issues by status. Human format uses `=== STATUS ===` headers sorted by workflow priority. Structured output adds a `groups` adjunct and does not drop or cap `issues`.
- `--parent string`: Filter by parent issue ID.
- `--status string`: Filter by status: `open`, `in-progress`, `done`, `merged`, `cancelled`, `blocked`.
- `--type string`: Filter by issue type: `task`, `story`, `feature`, `bug`.

**Examples:**
```bash
arm list --status done
arm list --status open --parent STORY-001
arm list --group
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

## ready

Show tasks ready to be claimed. Structured output (`--format json`, `--format agent`, and the non-TTY default) is one Agent Output Contract envelope object on stdout: `{count, issues, help}`. It is never a bare array, never `null`, and never a second JSON value on stderr.

`issues[]` rows include `id`, `type`, `status`, and `title`, plus ready-specific fields already useful to agents (`priority`, `scope`, `parent`, `estimated_complexity`, `requires_confirmation`, `assigned_worker` when set). `outcome` is omitted. `count` equals `len(issues)` and is not capped. Ready-queue rows are `open` issues only.

`--waves` stays on the same envelope. `issues` is still the flat complete list. A `waves` adjunct lists id groups, each inner array one scope-disjoint wave. Rows are not duplicated into the adjunct (N2). Human format is unchanged and does not print waves.

An empty ready queue is still the envelope: `{count: 0, "issues": [], "help": [...]}` with exit 0. `help` names why the queue is empty and still points at `arm show`. When `--parent` or `--assigned-to` filtered the queue to nothing, `help[0]` names that filter instead of blaming blockers or claims. When the empty queue has expired claims and no filter, `help[0]` names the `expired_claims` adjunct.

**Synopsis:**
`arm ready [flags]`

**Flags:**
- `--parent string`: Filter to descendants of this issue ID.
- `--worker string`: Worker ID for assignment-aware sorting.
- `--assigned-to string`: Filter to tasks assigned to this worker ID.
- `--explain`: Diagnose why open tasks are not in the ready queue. Structured output stays on the same envelope: `issues[]` rows are `{id, type, status, title, reason}` for excluded open tasks. Empty diagnosis is `{count: 0, "issues": [], "help": [...]}`. Human format stays `ID: reason` lines.
- `--waves`: Partition ready entries into scope-disjoint waves. Structured output adds a `waves` adjunct of issue-id groups and does not drop or cap `issues`.

**Description:**
The `--waves` flag partitions ready-eligible entries using a greedy first-fit algorithm that respects priority boundaries:
- Priority tier is a hard boundary: items from different priority tiers are never placed in the same wave.
- Within a tier, items are ordered by scope-conflict degree (how many other ready items share scope with them), placing high-conflict items first for better partitioning.
- Ancestor/descendant pairs are excluded from the same wave (e.g., a parent story and its child task cannot be in the same wave).
- Overlap detection uses glob-pattern matching on file scopes.

**Structured output:**
```json
{
  "count": 2,
  "issues": [
    {"id": "TASK-001", "type": "task", "status": "open", "title": "First", "priority": "high", "scope": ["src/a/**"]},
    {"id": "TASK-002", "type": "task", "status": "open", "title": "Second", "priority": "high", "scope": ["src/b/**"]}
  ],
  "waves": [
    ["TASK-001", "TASK-002"]
  ],
  "expired_claims": [],
  "help": [
    "dispatch one wave at a time",
    "arm show <id> for outcome, scope, and acceptance"
  ]
}
```
`waves` is present only when `--waves` is set. Each inner array is a wave of issue ids that can be claimed in parallel without scope conflicts. Full rows stay in `issues`.

**Expired claims:** issues in `claimed` or `in-progress` status whose claim TTL has
lapsed without renewal are never part of the ready queue itself (only `open` issues
are). They are not folded into `issues`.
- Text output: an "Expired claims (TTL lapsed without renewal):" section is always
  printed after the ready list (even when the ready list itself is non-empty).
- `--format json` / `--format agent`: expired claims are an `expired_claims` adjunct
  on the stdout envelope (`id`, `title`, `status`, `claimed_by`, timestamps, TTL).
  Structured stderr is not a results channel for that data. An agent that runs
  `2>/dev/null` still sees them. `help` notes the adjunct when it is non-empty.

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

## review

Manage conformance reviews for issues.

**Synopsis:**
`arm review [command]`

**Subcommands:**

### review commits

List delivery commits for an issue across all conventional-commit types.

**Synopsis:**
`arm review commits <issue-id> [flags]`

**Flags:**
- `--issue string`: Issue ID (alternative to positional argument).
- `--branch string`: Branch to scan for commits (default `HEAD`).
- `--format string`: Output format.

Structured output (`--format json`, `--format agent`) is `{count, commits, help}`.

**Example:**
```bash
arm review commits TASK-001
arm review commits --issue TASK-001 --format json
```

---

### review prepare

Create a semantic review bundle for an issue from its issue contract and delivery diff.

**Synopsis:**
`arm review prepare [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--base string`: Base git SHA (required).
- `--head string`: Head git SHA (required).
- `--format string`: Output format: `json`, `agent` (default "json").
- `--output string`: Write the ReviewBundle artifact to a file.

Stdout without `--output` is Artifact Output (the ReviewBundle schema, verbatim).
`--output <file>` writes that artifact to disk and prints an agent-facing envelope
`{count, bundles, help}` describing the file.

**Example:**
```bash
arm review prepare --issue TASK-001 --base abc123 --head def456
arm review prepare --issue TASK-001 --base abc123 --head def456 --output bundle.json
```

---

### review record

Record a semantic conformance assessment for a completed task.

**Synopsis:**
`arm review record [flags]`

**Flags:**
- `--issue string`: Issue ID (required).
- `--assessment string`: Path to assessment JSON file (required).

Structured output (`--format json`, `--format agent`) is `{count, assessments, help}` with `status` `recorded` or `duplicate`.

**Example:**
```bash
arm review record --issue TASK-001 --assessment assessment.json
```

---

### review validate

Validate a conformance assessment against a review bundle without recording it. Runs the same schema, criterion-ID, fingerprint, coverage, and citation line-bounds checks as `review record`, and prints an auto-fix suggestion for every failure. No ops are appended; `review record` remains the enforcement gate.

**Synopsis:**
`arm review validate [flags]`

**Flags:**
- `--assessment string`: Assessment file path or `-` for stdin (required).
- `--bundle string`: Review bundle file path (required).

**Output:**
- Human: `Assessment is valid`, or `Assessment is invalid:` plus each failure and its suggestion.
- JSON / agent: `{"valid":true}` or `{"valid":false,"failures":[{"message":"...","suggestion":"...","fixable":true}]}`. `fixable` is true when rewriting the assessment can apply the suggestion.

**Exit codes:**
- `0` — assessment is valid.
- `1` — assessment is invalid (advisory; not a Command Failure).

**Example:**
```bash
arm review validate --assessment assessment.json --bundle bundle.json
arm review validate --assessment assessment.json --bundle bundle.json --format json
```

---

## scope-delete

Remove an exact file path from all issue scopes.

**Synopsis:**
`arm scope-delete <path>`

**Example:**
```bash
arm scope-delete cmd/armature/main.go
```

---

## scope-rename

Rename a scope path across all issues using substring matching.

**Synopsis:**
`arm scope-rename <old-path> <new-path>`

**Examples:**
```bash
arm scope-rename cmd/trellis/main.go cmd/armature/main.go
arm scope-rename cmd/trellis cmd/armature
```

---

## stats

Derived metrics from the ops log.

**Synopsis:**
`arm stats [flags]`

**Description:**
The `--cost` view sums recorded token counts into per-story and per-wave
dollar estimates. Counts come from optional `input_tokens` / `output_tokens`
on transition outcome payloads and assessment attestations (G1.1). Zero or
omitted means unset.

Dollars are USD per million tokens. Built-in rates (not a billing guarantee):

- `default` / `claude-sonnet-4-5`: $3.00 input / $15.00 output
- `claude-haiku-4-5`: $1.00 input / $5.00 output
- `claude-opus-4-5`: $15.00 input / $75.00 output
- `gpt-4.1`: $2.00 input / $8.00 output
- `gpt-4.1-mini`: $0.40 input / $1.60 out

Override with `--rates` (JSON `{"models":{"name":{"input_usd_per_mtok":3,"output_usd_per_mtok":15}}}`)
or `.armature/cost-rates.json`. Unlisted models keep built-in rates. Outcome
records resolve model as payload `preferred_model`, then the issue's
`preferred_model`, then `default`. Assessment records use attestation
`model_identity` or `default`; they do not inherit the issue's preferred model.

Waves are greedy first-fit groups of issues that recorded usage and do not share
overlapping scope. Stories roll up descendant usage. Spend uses the same captured
validated op set as snapshot materialization. Idempotent assessment attestations
(same issue and `result_fingerprint`) are counted once.

Without `--cost`, the command prints a hint. Latency and rework analytics are
not implemented yet.

**Flags:**
- `--cost`: Aggregate recorded token counts into dollar estimates.
- `--rates string`: JSON rate table path.

**Example:**
```bash
arm stats --cost
arm stats --cost --format json
arm stats --cost --rates .armature/cost-rates.json
```

---

## show

Show a human-readable summary of one or more issues. Structured output (`--format json`, `--format agent`, and the non-TTY default) is one Agent Output Contract envelope object on stdout: `{count, issues, help}`. It is never a bare issue object and never a top-level array.

`issues[]` is the detail view. A found issue has `count` 1. Several positional IDs share one envelope whose `count` equals the number of issues shown. A missing ID is an error, not an empty envelope. Rows keep the existing issue fields (`id`, `type`, `status`, `title`, plus outcome, scope, notes, and the rest of the detail schema).

Large text fields (`outcome`, `definition_of_done`) are truncated by default at 512 bytes. Truncation keeps a prefix, states the total size in a `truncated` adjunct (`field`, `shown_bytes`, `total_bytes`), and puts `--full` in `help[0]`. `--full` returns complete fields and omits the adjunct. `--full` is offered in `help` only when truncation actually happened.

`--field` stays a scalar extractor (N1.5). Even with `--format json` or `--format agent`, stdout is the requested values as plain text, one per line, never the envelope. Command substitution such as `$(arm show ID --field claimed_by)` captures a bare value.

Human TTY output still uses `RenderIssue` plus a `Spend-to-date` line. Structured and `--field` output do not load spend.

**Synopsis:**
`arm show [issue-id ...] [flags]`

**Flags:**
- `--field string`: Extract field values as plain text, one per line (e.g. `status` or `status,outcome,title`). Never an envelope.
- `--full`: Return complete large text fields in structured output.
- `--issue string`: Issue ID to show.

**Structured output:**
```json
{
  "count": 1,
  "issues": [
    {"id": "TASK-001", "type": "task", "status": "open", "title": "Example"}
  ],
  "help": [
    "arm show <id> --field <name> extracts a scalar value, never an envelope"
  ]
}
```

Coordinator remedia-cycle recovery reads notes from `.issues[0].notes`, not a top-level `.notes` parser.

---

## sources

Manage external knowledge sources.

**Synopsis:**
`arm sources [command]`

**Subcommands:**

### sources accept-citation

Accept a citation for an issue with a recorded rationale.

**Synopsis:**
`arm sources accept-citation [issue-id] [flags]`

**Flags:**
- `--ci`: Bypass interactive prompt (non-interactive/CI mode).
- `--issue string`: Issue ID to accept citation for (repeatable).
- `--rationale string`: Rationale for accepting the citation (>=3 words).
- `--force`: Skip confirmation prompt and proceed (alias for --ci).

**Example:**
```bash
arm sources accept-citation E5-S4-T3 --rationale "Documentation is complete and reviewed."
```

---

### sources add

Add a new source to the manifest.

**Synopsis:**
`arm sources add [flags]`

**Flags:**
- `--url string`: URL or path of the source (required).
- `--type string`: Provider type: filesystem, confluence, sharepoint (required).
- `--title string`: Optional title for the source.

**Example:**
```bash
arm sources add --url "https://example.com/docs" --type filesystem
```

---

### sources link

Link one or more issues to a source entry in the manifest.

**Synopsis:**
`arm sources link [issue-id] [flags]`

**Flags:**
- `--issue string`: Issue ID to link (repeatable).
- `--source-id string`: UUID of the source entry in the manifest (required).

**Example:**
```bash
arm sources link TASK-001 --source-id abc-def-123
```

---

### sources stale-review

Review sources whose cached content has changed since last sync.

**Synopsis:**
`arm sources stale-review [flags]`

**Description:**
Detects sources with changed fingerprints and presents an interactive review for confirming or flagging changes.

---

### sources sync

Fetch and cache content for all sources.

**Synopsis:**
`arm sources sync`

---

### sources verify

Verify cached content matches stored fingerprints.

**Synopsis:**
`arm sources verify`

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
- `--skip-delivery-gate`: Skip the delivery gate check only when transitioning to `done`; it is rejected for other states. The transition op records `Payload.SkippedDeliveryGate` (`skipped_delivery_gate` in the op log) as the audit flag. Supply `--outcome` with the reason for the override. See [Delivery Gate](use-cases.md#the-delivery-gate).

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

## unlink

Remove a dependency link between issues.

**Synopsis:**
`arm unlink [flags]`

**Flags:**
- `--source string`: Source issue ID.
- `--dep string`: Dependency issue ID.

---

## validate

Validate the issue graph and documentation.

**Synopsis:**
`arm validate [flags]`

Validation is **strict by default**: warnings fail the run, a green run prints a
single summary line (`OK: no issues found` plus coverage when present), and any
error or (under `--strict`) warning exits non-zero. Findings stay in their
native buckets: JSON `warnings` still lists W-codes when strict. Structured
output is `{count, findings, help}` with `errors`, `warnings`, and `infos` as
adjuncts. Snapshot/materialization warnings (rejected or malformed ops) are
included in `findings` and `warnings` so `count: 0` cannot mean a clean graph
when ops were excluded; default-strict validate fails in that case. `--ci` is the
CI alias for the same fail-closed contract (`make validate-graph`); it is not
part of the per-task `make check` publish gate. `--ci --strict=false` is
rejected as contradictory. There are no waivers and no scoping flags; the
whole graph is validated or not at all. Rules that fire on intentional
states are fixed or deleted.

**Subcommands:**

**Flags:**
- `--ci`: Exit non-zero if errors or (implied) warnings found. Used by CI / `make validate-graph`. Contradicts `--strict=false`.
- `--strict`: Treat warnings as failures (default `true`; pass `--strict=false` to keep warnings as warnings and print INFOs).
- `--quiet`: Suppress INFO lines on a failing run.

**Validation Codes:**
See [Validation & Doctor Codes Reference](./validation-codes.md) for complete documentation of all error codes and warnings.

---

### validate doc-examples

Validate typed JSON examples in canonical documentation.

**Synopsis:**
`arm validate doc-examples [flags]`

**Flags:**
- `--repo string`: Repository root (default ".").

**Description:**
Hidden command used by `make check` to validate JSON examples in documentation are well-typed and valid.

---

## version

Print `arm` version. Root `--version`, `-v`, and `-V` are the same query.

Human format prints the existing bare line `arm version <string>` and exits 0 when `--format human` is explicit, or when the invocation is an interactive TTY default.

Structured format (`--format json`, `--format agent`, `--non-interactive` implying structured unless `--format human` is explicit, or a non-TTY default) is one Agent Output Contract envelope `{count, versions, help}` with `count` 1 and a single row whose `version` member is the version string.

**Synopsis:**
`arm version`

**Structured output:**
```json
{
  "count": 1,
  "versions": [{"version": "dev"}],
  "help": ["arm version reports the build identity of this binary"]
}
```

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

Structured output (`--format json`, `--format agent`, or `--json`) is one Agent
Output Contract envelope `{count, workers, help}`. It is never JSONL.

**Flags:**
- `--json`: Output as JSON envelope (legacy alias of `--format json`).

---

## worktree

Manage managed worktrees and reconcile them against claim state.

**Synopsis:**
`arm worktree [command]`

**Subcommands:**

### worktree list

List managed worktrees classified as bound, orphan, ghost, gc-ready, unrecognized, or ambiguous.

Structured output is `{count, worktrees, help}` with class buckets as adjuncts (`bound`, `orphans`, `ghosts`, `gc_ready`, `unrecognized`, `ambiguous`).

### worktree gc

Remove worktrees for merged or cancelled issues. `--dry-run` previews removals.

Structured output is `{count, worktrees, help}` with each row's `action` set to `removed`, `would_remove`, `skipped`, `failed`, or `ambiguous`.

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
# See the concrete, copyable examples above for command syntax.
```
