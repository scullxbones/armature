# Harness Hook Integration Guide

Armature's `harness-hook` command integrates task scope and acceptance verification with external AI workers (Claude Code, Codex, Devin). The hook intercepts tool use and task completion events, enforcing task boundaries and running verification before allowing operations.

## What `arm harness-hook` Does

The `harness-hook` command is an internal integration surface called by harness-native hook mechanisms. It:

1. **Intercepts Pre-Tool Events** (e.g., file edits, shell commands)
   - Checks if paths are within the active task's scope
   - Blocks direct `git commit` commands (Armature owns commits)
   - Allows or denies the tool operation

2. **Intercepts Stop Events** (task completion)
   - Runs verification against task acceptance criteria
   - Checks that all citations have been reviewed and accepted
   - Blocks task completion if verification fails

3. **Returns Platform-Native Decisions**
   - Allow (tool proceeds, verification passed)
   - Block (tool denied, reason provided to model)
   - None (event ignored, no policy applies)

## Issue Binding Resolution Chain

The hook resolves the active task ID using a four-step resolution chain (ADR-0007),
applied in priority order. Binding identity follows the artifact being touched, not the
process touching it — this ensures each agent operates under exactly one issue binding
regardless of worktree or session nesting.

### Four-Step Resolution Chain

The hook attempts to resolve a binding in this order:

1. **tool_input.file_path walk-up** (PreToolUse/PostToolUse only)
   - For file edit/write/read events, the hook walks up the directory tree from the target
     file path until it finds a `.git` directory.
   - It then reads the `armature-issue-id` file from that `.git` directory (or from the
     git worktree's git directory if the target is inside a git worktree).
   - If a valid binding is found, resolution stops here — the event is evaluated under
     the worktree's issue.

2. **Event payload cwd walk-up** (PreToolUse/PostToolUse only)
   - If step 1 yields no binding and the harness platform reported a current working
     directory in the hook event payload, the hook walks up from that directory to find
     a `.git` directory and reads `armature-issue-id`.
   - This covers harnesses that report per-agent working directories (e.g., when an
     agent runs inside a different worktree from the session root).

3. **Session binding** (all events)
   - The hook reads `armature-issue-id` from the git dir of the repository passed via
     `--repo` (or the current directory when `--repo` is omitted) — the session's own
     worktree binding.
   - This is the fallback for Bash and Stop events, which resolve at the session level
     and are never path-resolved.

4. **ARMATURE_ISSUE_ID environment variable** (last-resort fallback)
   - If the session git dir has no binding file, the hook falls back to the
     `ARMATURE_ISSUE_ID` environment variable set at harness launch time.
   - Bindings resolved via steps 3 or 4 are both logged with `resolution_step=session`.

If none of these steps yields a binding, the write is marked as unbound (see Violation Gate below).

### Why Worktree Dispatch is Mandatory

Worktree dispatch (passing `--worktree <path>` to `arm claim`) is **required** for worker
dispatch. The binding-resolution invariant depends on it: without a bound worktree, steps 1–2
have nothing to resolve against, and the hook cannot distinguish between intended pass-throughs
and enforcement gaps. Workers claim with `arm claim TASK-ID --worktree <path>` (creating an
isolated git worktree on a task-specific branch). The task ID is written to the worktree's
git-dir `armature-issue-id` file. When the worker's hook fires, step 1 resolves the binding
from the file path being edited, confirming that each agent operates under its claimed issue.

### Binding File Locations

- **Regular git repository:** `<repo-root>/.git/armature-issue-id`
- **Git worktree:** `<repo-root>/.git/worktrees/<worktree-name>/armature-issue-id`

Every binding read (steps 1-3 above, plus the merged-issue violation gate) falls back to
the legacy `armature-task-id` file in the same git dir when `armature-issue-id` is absent,
for compatibility with worktrees claimed before the `armature-issue-id` rename (commit
`d52d78be`). `armature-issue-id` always takes precedence when both files are present.

When a task is claimed with `arm claim --worktree <path>`, the task ID is written to the
worktree's git-specific directory, ensuring it is found during step 1 when the hook runs.

## Decision Logging and Violation Gate

All binding-resolution decisions and enforcement actions are logged to the worktree's
local `armature-hook.log` file. This log is consulted during merge validation to detect
violations (unbound file writes) and enforce the violation gate.

### Decision Log Format

The hook appends entries to `<git-dir>/armature-hook.log` for all events. Each entry
includes a UTC RFC3339 timestamp and one of the following formats:

**Decision entries** (for all resolved bindings):
```
<timestamp> decision: issue_id=<ISSUE-ID> resolution_step=<STEP> event=<EVENT_KIND> tool=<TOOL_NAME> decision=<ACTION> [block_reason=<REASON>]
```

- `<timestamp>`: UTC RFC3339 (e.g., `2026-07-04T12:34:56Z`)
- `<ISSUE-ID>`: The resolved task ID (e.g., `TASK-001`)
- `<STEP>`: Resolution step that determined the binding: `file_path`, `event_cwd`, or `session`
- `<EVENT_KIND>`: Normalized hook event kind (`pre-tool-use`, `post-tool-use`, or `stop`)
- `<TOOL_NAME>`: The tool being invoked (e.g., `Edit`, `Write`, `Bash`)
- `<ACTION>`: Hook decision (`allow`, `block`, or `none`)
- `<REASON>`: (optional) Block reason or metadata, included if the action is `block`

Example decision entries:
```
2026-07-04T12:34:56Z decision: issue_id=TASK-001 resolution_step=file_path event=pre-tool-use tool=Edit decision=allow
2026-07-04T12:34:57Z decision: issue_id=TASK-001 resolution_step=file_path event=pre-tool-use tool=Edit decision=block block_reason=path is outside task scope
```

**Pass-through entries** (for unblocked events with no binding):
```
<timestamp> pass-through: <reason>
```

Pass-throughs occur when an event cannot be resolved to any binding (the session has no
active binding, and the event has no file path to resolve from), or when a transient
error occurs during binding resolution or policy evaluation. These are warnings, not
enforcement gaps.

Example pass-through entries:
```
2026-07-04T12:34:56Z pass-through: no issue binding found
2026-07-04T12:34:57Z pass-through: snapshot load failed
2026-07-04T12:34:58Z pass-through: event decode failed
```

**Violation entries** (for unbound file writes):
```
<timestamp> violation: <reason>
```

Violations represent enforcement gaps: file writes that should have been subject to
scope policy but were not (because binding resolution failed). The hook is fail-open
(does not block the write), but logs the violation for later auditing.

Example violation entries:
```
2026-07-04T12:34:56Z violation: file write with no resolved binding
```

### Violation Gate

The violation gate is a merge-time enforcement mechanism. When `arm merged --issue <TASK-ID>`
is run to mark a completed task as merged:

1. The hook checks the worktree's `armature-hook.log` for any `violation:` entries.
2. If violations are found and `--force` is **not** specified, `arm merged` exits with an error
   and **does not tear down the worktree** — preserving it as evidence for audit and remediation.
3. If violations are found and `--force` **is** specified, the task is marked merged despite violations
   (explicit operator override).
4. Pass-through entries (`pass-through:`) are warnings only and do not block the merge;
   a message is emitted to stderr.

A wave whose tasks contain `violation:` entries in their logs **must not be integrated**
(merged into the main branch) without explicit operator override (`--force`). Violations
indicate that the harness was unable to enforce scope, raising risk for the story integration.

### Fail-Open Posture

The hook operates with a fail-open posture:
- If snapshot loading fails (e.g., state files are corrupted or missing), the event is
  passed through with loud stderr warning, not blocked.
- If hook event decoding fails (e.g., the platform sends malformed JSON), the event is
  passed through with loud stderr warning, not blocked.
- If binding resolution encounters transient errors, the event is passed through with
  loud stderr warning, not blocked.

Enforcement gaps are surfaced by the **violation gate** (unbound file writes logged and
blocked at merge time), not by freezing tool use and leaving the worker stranded. This
ensures workers can continue even if temporary issues occur, while providing operators
with a clear audit trail to detect and remediate enforcement gaps before merging.

The scope check itself is purely lexical: it compares the tool-reported path string
against the declared scope globs and does not resolve symlinks. A symlink that lives
inside the worktree but points outside it (e.g. to another repo or `/etc`) is treated
as an in-scope path if its in-worktree name matches, and writes through it are not
detected as an escape. This is consistent with the hook's overall fail-open posture
(ADR-0007): the hook is a best-effort guardrail, not a sandbox, and only the violation
gate at merge time provides a hard backstop.

## Autonomic Heartbeats

The hook automatically emits rate-limited heartbeats on every `PreToolUse` event (file
edits, shell commands, etc.) for bound, non-stale claims. This ensures that active work
updates the claim's activity timestamp without requiring manual heartbeat calls.

### Fixed Debounce Interval

Heartbeats are emitted **at most once every 5 minutes** per worker+issue combination,
independent of claim TTL. The debounce interval is:

- **Fixed at 5 minutes** (constant `HeartbeatDebounceInterval` in `internal/claim`)
- **Not configurable** via repo configuration or environment variables
- **Always on** — operators cannot disable it with a config flag

This fixed interval decouples heartbeat rate from claim TTL, providing predictable
behavior: a worker can always rely on a heartbeat being emitted within a 5-minute
window of active tool use.

### Per-Worker+Issue Rate-Limit State

Heartbeat rate-limit state is stored in the **OS temporary directory** as a disposable,
per-worker+issue cache:

- **Location:** `<os.TempDir()>/armature-heartbeat-<workerID>-<issueID>.json`
- **Format:** JSON with `last_heartbeat_time_unix` field (Unix timestamp)
- **Lifecycle:** Not committed to the repository; persists on disk until the OS reclaims
  `/tmp` (e.g. on reboot). It is not tied to a process or session lifetime — a fresh
  session started within the debounce window will inherit and respect the prior
  session's rate-limit timestamp, not reset it.

This design ensures that rate-limit state:
- Does not accumulate in the worktree or repository
- Cannot cause merge conflicts

### Source Field: Auto vs. Manual

Every heartbeat op carries a `Source` field in its `Payload` to distinguish between
automatic and manual heartbeats:

- **`"source":"hook"`** (auto): Emitted by the harness hook on every 5th-minute-spaced
  `PreToolUse` event (tool use within the harness)
- **`"source":"<omitted>"`** (manual): Emitted by explicit `arm heartbeat ISSUE-ID` commands

This distinction allows operators and reviewers to:
- See which heartbeats came from active tool use vs. manual intervention
- Detect long stretches without tool use (gaps between hook-emitted heartbeats)
- Correlate heartbeat timing with other activity evidence

### Operational Guidance on Claim TTL

Operators should configure `default_ttl` (claim time-to-live, in minutes) to be **at least
3x the fixed heartbeat debounce interval (~15 minutes minimum)** to provide a safety margin:

- **Too-short TTL** (e.g., `default_ttl: 5`): Claims may expire between heartbeats if tool
  use pauses for a few minutes, risking claim theft before the next heartbeat window.
- **Safe TTL** (e.g., `default_ttl: 15` or higher): Provides a 2–3 minute buffer beyond
  the next heartbeat, tolerating brief pauses in tool use without claim loss.

Example repo-level configuration (`.armature/config.json`):

```json
{
  "default_ttl": 15
}
```

This ensures that even if a worker pauses tool use at the end of a 5-minute heartbeat
window, the claim will not expire before the next heartbeat can be emitted.

### Fail-Open Posture

Heartbeat emission uses fail-open semantics:
- If the worker ID cannot be resolved, heartbeat is skipped (not fatal)
- If the rate-limit state file cannot be read or written, heartbeat is still emitted and
  a warning is logged to stderr (rate-limit state loss causes more frequent heartbeats
  temporarily, not a claim-loss risk)
- If the heartbeat op cannot be written to the ops log, a warning is logged to stderr
  and the hook does not block tool execution

This ensures tool execution is never blocked by heartbeat machinery. Heartbeats are
advisory; enforcement comes from the claim TTL and explicit `arm claim` release.

### When to Use Manual Heartbeats

Workers should call `arm heartbeat ISSUE-ID` manually only during **long stretches of
non-tool thinking work**, when no tool calls are happening:

- **No manual heartbeat needed:** During active file editing, shell commands, and other
  tool use (hook handles it automatically)
- **Manual heartbeat recommended:** When thinking or analyzing for >5 minutes without
  calling tools (e.g., reviewing code, designing an approach, reading docs). These
  stretches produce no `PreToolUse` events, so the automatic hook cannot emit a heartbeat.

Example workflow:
```bash
# Thinking phase (no tools for 6 minutes)
# → After ~5 minutes: manually run `arm heartbeat TASK-ID`

# Then tool use resumes
# → Hook automatically emits heartbeat every 5 minutes during tool use

# Thinking phase again (no tools for 8 minutes)
# → Manually run `arm heartbeat TASK-ID` again
```

For workers operating primarily within a harness UI (Claude Code, Codex, Devin), this
is typically not a concern, since most work involves tool use. The primary case for
manual heartbeats is standalone workflows where the worker uses Armature from the
command line with long thinking pauses between commands.

## Execution Evidence Capture

In addition to scope and acceptance enforcement, harness hooks now record execution
evidence for qualified tool invocations. This allows semantic review to access
harness-captured command/output pairs (not worker-authored reasoning) as a
third, explicitly weaker evidence class. See ADR-0008 for the trust model,
disclosure posture, and why execution evidence is upgrade-only.

### What Gets Captured

For every **PostToolUse event** (`Bash` tool only) on the bound issue, the hook captures:
- **Command:** The exact command string executed
- **Exit Status:** The command's exit code
- **Truncated Output:** First 1 KB of stdout/stderr + last 1 KB (if output > 2 KB)
- **Full Output Hash:** SHA-256 hash of the complete output for integrity checking
- **Worktree HEAD SHA:** The git commit at execution time (tied to delivery provenance)
- **Timestamp:** UTC RFC3339 timestamp of execution

This is a complete-capture, no-filtering policy. Every Bash command on the bound
issue is recorded. Failure-then-success sequences remain visible instead of being
curated by the worker.

### Activity Log Format and Location

The activity log is **worktree-local, ephemeral**, stored as **JSONL** (one JSON object
per line, via `encoding/json`):

- **Location:** `<worktree-git-dir>/armature-activity.log` (e.g., `.git/armature-activity.log`
  for regular repos, `.git/worktrees/<worktree-name>/armature-activity.log` for worktrees)
- **Format:** One JSON object per line, appended chronologically, with fields
  `timestamp` (RFC3339), `command`, `exit_code` (int, meaningful only when
  `exit_code_known` is `true`), `exit_code_known` (bool), `head_sha`, `output_hash`,
  and `output_head`/`output_tail` (present only when there is output to show)
- **Entry IDs:** not stored in the file — an entry's ID is the 0-based physical line
  number it occupies in the log file (see "Entry ID Convention" below)
- **Retention:** Worktree-local only; never committed to the ops log or repository
- **Lifecycle:** Deleted when the worktree is torn down

Example log entry:
```json
{"timestamp":"2026-07-04T12:34:56Z","command":"go test ./...","exit_code":0,"exit_code_known":true,"head_sha":"abc1234...","output_hash":"def5678...","output_head":"ok  github.com/armature/examples  0.254s"}
```

### Entry ID Convention

Citations (`activity_entry_id` in a `ConformanceAssessment` Citation) reference entries
by **0-based physical line number** as a plain integer string (`"0"`, `"1"`, `"2"`, …) —
not zero-padded, not 1-based. A malformed or blank line consumes its line number but
produces no entry, so entry IDs are not guaranteed to be contiguous, and later entries'
IDs never shift because an earlier line failed to parse. This is the same convention
documented in the `armature-activity-indexer` and `armature-reviewer` skills.

### Truncation Behavior

Output is truncated content-neutrally:
- **Short output** (≤ 2 KB): kept in full, in `output_head` (`output_tail` absent/empty)
- **Long output** (> 2 KB): split into `output_head` (first 1 KB) and `output_tail` (last 1 KB);
  the middle is dropped, with only the full-output hash (`output_hash`) retained for integrity
  verification — there is no separate in-band "truncated" marker string, since the presence
  of a non-empty `output_tail` field is itself the truncation signal
- Truncation points are adjusted to the nearest UTF-8 rune boundary so multi-byte characters
  are never split across the head/tail cut

The full output hash allows the Reviewer to verify that the reported excerpt matches
the full execution. The hash is recorded in the activity log but **not shown** in the
published report (citation boundary, see docs/sensitive-environments.md).

### Kill-Switch Mechanism

Execution evidence capture is **default-on**. The only supported kill-switch is the
repo-level git config:

```bash
git config --local armature.disable-activity-logging true
```

This setting:
- Is per-repository (all worktrees of a repo share the same `--local` config)
- Persists across sessions
- Takes effect immediately on the next hook invocation

There is deliberately **no environment-variable override**. An env var would be
settable by the worker process mid-session (`export …=1; run failing test; unset`),
letting the worker curate failure-then-success sequences out of the log — exactly the
selection bias this capture policy exists to prevent (see "What Gets Captured" above).
Disabling capture is a repo-level Definition-of-Done decision, not something a running
session should be able to toggle unilaterally.

### Fail-Open Posture

The hook fails open on any capture error:
- If worktree HEAD cannot be resolved, capture is skipped with stderr warning
- If the log file cannot be opened or written, capture is skipped with stderr warning
- The hook never blocks a tool invocation due to capture failure

This ensures workers are never stranded due to activity logging errors. Warnings
are emitted to stderr for operator visibility.

### Teardown-After-Record Ordering Constraint

**The activity log is worktree-local.** Worktree teardown must happen **ONLY AFTER**
`arm review record`, never before. This is a hard constraint because:

1. The activity log lives in the worktree's git directory
2. `arm review record` re-verifies the activity log digest before finalizing attestation,
   but **only when the assessment cites activity evidence** — if the assessment contains
   no activity citations, this digest check is skipped
3. Tearing down the worktree (deleting it) destroys the log before it can be verified

**Correct ordering:**
```
1. Worker completes and transitions task to done
2. Coordinator runs `arm review prepare` (constructs bundle from delivery range)
3. Coordinator dispatches Reviewer
4. Reviewer evaluates and emits result
5. Coordinator runs `arm review record` (verifies digest, creates attestation)
6. Coordinator runs `arm merged --issue <task-id>` (tears down worktree)
```

Violating this order (e.g., tearing down before record) will cause `arm review record`
to fail with a digest mismatch **when the assessment cites activity evidence**, since the
log has been deleted. If the assessment contains no activity citations, the digest check
is skipped and record will not fail on this basis — but the ordering constraint should
still be followed, since whether activity citations are present is not known in advance
at teardown time.

## Environment Variables

When launching an external harness, set these variables in the harness environment:

### `ARMATURE_ISSUE_ID` (fallback only)
The active Armature task ID for the worker. Only required when NOT using `arm claim --worktree`.
When a worktree binding file is present, this variable is ignored.

**Example:**
```bash
export ARMATURE_ISSUE_ID=TASK-001
```

### `ARMATURE_HOOK_PLATFORM` (required)
The harness platform type. Controls hook input/output encoding.

**Valid values:** `claude`, `codex`, `devin`

**Example:**
```bash
export ARMATURE_HOOK_PLATFORM=claude
```

### Environment propagation and subagents

Hook handlers do not get task-specific variables injected per event — they inherit the
**launch environment of the harness process**. Consequences:

- Variables exported *inside* a session (e.g. via a shell tool call) do not reach hook
  handlers; each tool call and each hook run is a fresh process. Only variables set before
  the harness starts are visible to `arm harness-hook`.
- Subagents spawned by the harness (Claude Code Task/Agent tool, etc.) run inside the same
  harness process environment, so their tool calls fire the same hooks with the same
  launch-time variables. You cannot give a subagent a different `ARMATURE_ISSUE_ID` via env.
- This is why the worktree binding file is preferred: it derives the task ID from *where the
  hook runs* (the event payload's `cwd`/git dir), not from process environment. Per-task
  isolation falls out of per-task worktrees with zero env plumbing.
- If transient per-invocation data is needed, put it in a file keyed by cwd or session
  (like the binding file), or parse it from the hook's stdin JSON payload (`session_id`,
  `cwd`, `tool_input`) — not from environment variables.

Note for Claude Code specifically: hook commands receive the event as **JSON on stdin**
(fields like `hook_event_name`, `tool_name`, `tool_input`, `cwd`, `session_id`) and
`CLAUDE_PROJECT_DIR` in the environment. There is no `CLAUDE_TOOL_INPUT` variable.

## Claude Code Setup

Claude Code uses `.claude/settings.json` for hook configuration.

### 1. Configure `settings.json`

Add the following to `.claude/settings.json` (create the file if it doesn't exist):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "arm harness-hook"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "arm harness-hook"
          }
        ]
      }
    ]
  }
}
```

### 2. Set Environment Variables

When launching Claude Code, set:

```bash
export ARMATURE_ISSUE_ID=TASK-001
export ARMATURE_HOOK_PLATFORM=claude
claude <project-directory>
```

### 3. Verification

To verify the configuration is active, attempt to edit a file outside task scope:

```bash
# In Claude Code:
# 1. Claim a task with limited scope (e.g., src/auth/login.go)
# 2. Try to edit a different file (e.g., README.md)
# Expected: Hook blocks the edit with "path is outside task scope" message
```

## Codex Setup

Codex uses `codex.toml` for hook configuration.

### 1. Configure `codex.toml`

Create or edit `codex.toml` in your project root:

```toml
[hooks]
pre_tool_use = "arm harness-hook"
stop = "arm harness-hook"
```

### 2. Set Environment Variables

When launching Codex, set:

```bash
export ARMATURE_ISSUE_ID=TASK-001
export ARMATURE_HOOK_PLATFORM=codex
codex <project-directory>
```

### 3. Verification

To verify the configuration is active, inspect Codex logs during tool use:

```bash
# In Codex:
# 1. Claim a task with scope ["src/auth/*.go"]
# 2. Attempt to edit src/auth/login.go
# Expected: Hook approves (within scope)
#
# 3. Attempt to edit docs/README.md
# Expected: Hook blocks (outside scope) with "path is outside task scope" reason
```

## Devin CLI Setup

Devin uses `.devin/hooks.json` for hook configuration.

### 1. Configure `.devin/hooks.json`

Create or edit `.devin/hooks.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "edit|exec",
        "command": "arm harness-hook"
      }
    ],
    "Stop": [
      {
        "command": "arm harness-hook"
      }
    ]
  }
}
```

### 2. Set Environment Variables

When launching Devin, set:

```bash
export ARMATURE_ISSUE_ID=TASK-001
export ARMATURE_HOOK_PLATFORM=devin
devin <project-directory>
```

### 3. Verification

To verify the configuration is active, review Devin's execution logs:

```bash
# In Devin:
# 1. Claim a task with scope ["cmd/arm/*.go"]
# 2. Execute a shell command that reads within scope
# Expected: Hook approves (within scope)
#
# 3. Attempt to edit external/external.go
# Expected: Hook blocks with "path is outside task scope" reason
```

## Task Scope and Acceptance Verification

### Task Scope Enforcement

The hook reads the task's `scope` field (a list of file path globs or exact paths) from `.armature/state/issues/<task-id>.json`. Before a file edit or read, the hook checks:

- **Exact path match:** If the scope contains `src/auth/login.go` and the tool targets `src/auth/login.go`, it's allowed.
- **Glob match:** If the scope contains `src/auth/*.go` and the tool targets `src/auth/login.go`, it's allowed.
- **Outside scope:** If the tool targets `docs/README.md` and no scope entry matches, the operation is blocked.

### Acceptance Criteria Verification

When the harness stops (model completes task), the hook runs verification:

1. **Check acceptance criteria:** The task's `acceptance` field contains JSON-encoded acceptance rules. The verification service evaluates them.
2. **Check citation state:** If the task has `source_links`, all linked sources must be accepted (recorded via `arm sources accept-citation`).
3. **Block if failed:** If any criterion or citation is not met, the hook blocks completion with a reason message.

### Example: Acceptance Criteria

Task spec (rendered by `arm render-context`):

```
# Task: Write login API endpoint

## Scope
- src/auth/*.go

## Acceptance Criteria
- Endpoint returns JWT on valid credentials
- Endpoint returns 401 on invalid credentials
- Request/response are logged without credentials
```

When the harness stops:

1. Verification service scans each acceptance criterion for machine-verifiable keywords (e.g. `go test`, `make check`, `arm validate`). At least one such criterion must be present; purely human-review criteria are flagged but not individually verified against the implementation.
2. If the keyword check passes, completion is allowed.
3. If no machine-verifiable criterion exists, completion is blocked with a message prompting you to add one.

## Common Issues and Troubleshooting

### Hook passes through with "no task binding found"

**Cause:** Neither the worktree binding file (`<git-dir>/armature-issue-id`) nor the
`ARMATURE_ISSUE_ID` environment variable is present when the harness starts.

**Fix (preferred):** Claim the task with `--worktree` before launching the harness:
```bash
arm claim TASK-001 --worktree ./task-001-work
# then launch harness from the worktree directory
cd ./task-001-work
ARMATURE_HOOK_PLATFORM=claude claude .
```

**Fix (fallback):** Set the environment variable before launching the harness:
```bash
export ARMATURE_ISSUE_ID=TASK-001
export ARMATURE_HOOK_PLATFORM=claude
# then launch harness
```

### "task <task-id> not found"

**Cause:** The task ID does not exist, or `.armature/state/issues/<task-id>.json` is missing.

**Fix:** 
1. Verify the task exists: `arm show <task-id>`
2. Verify Armature state is materialized: `arm materialize`
3. Check that `.armature/state/` is readable from the harness cwd

### Hook blocks legitimate edits with "path is outside task scope"

**Cause:** The task scope is too restrictive, or the scope file globs don't match the target file.

**Fix:**
1. Check the task scope: `arm show <task-id> --field scope`
2. Verify glob patterns: `src/auth/*.go` matches `src/auth/login.go` but not `src/login.go`
3. If scope is wrong, amend the task: `arm amend <task-id> --scope "src/**/*.go"`

### "Verification failed" blocks task completion

**Cause:** Acceptance criteria or citations were not satisfied.

**Fix:**
1. Review which criteria failed in the error message
2. Update implementation to satisfy the failing criteria
3. For citation failures, run `arm sources accept-citation <task-id>` to accept all sources

## Manual Configuration Without an Installer

Armature does not ship a harness hook installer. Configuration is manual:

1. **Identify your harness platform** (Claude Code, Codex, or Devin).
2. **Copy the JSON/TOML snippet** from the relevant section above into your config file.
3. **Set `ARMATURE_ISSUE_ID` and `ARMATURE_HOOK_PLATFORM`** when launching the harness.
4. **Test** by claiming a task and attempting to edit in-scope and out-of-scope files.

## Hook Event Types

The hook receives event JSON from the harness. Common event types:

### PreToolUse
Fired before a file edit or shell command.

Example (Claude Code):
```json
{
  "hook_event_name": "PreToolUse",
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "src/auth/login.go"
  }
}
```

### Stop
Fired when the harness is stopping (task completion).

Example (Codex):
```json
{
  "hook_event_name": "stop"
}
```

## Scope Violation Visibility

Out-of-scope operations are logged with a violation marker in `armature-hook.log`, even when the
hook blocks or passes through the operation. This ensures operators can detect scope escapes and
review evidence of enforcement gaps.

### Scope Violation Logging

Whenever scope checking detects that paths are outside a task's declared scope, the hook:
1. Logs a violation entry with the out-of-scope paths (as part of the decision log entry)
2. Blocks the operation with a clear message
3. Records the paths for later audit and review

Example violation log entry:
```
2026-07-04T12:34:57Z decision: issue_id=TASK-001 resolution_step=file_path event=pre-tool-use tool=Edit decision=block block_reason=path(s) outside task scope: cmd/main.go; allowed scope: internal/auth/
```

### Post-Task Artifact Check (D8)

The doctor (`arm doctor`) includes a D8 check that flags out-of-scope artifacts discovered
**after a task is complete**, on disk in the main worktree. This catches stray binaries and
build artifacts that may have escaped scope enforcement during task execution.

**D8 Scope:**
- Checks active (claimed/in-progress) tasks and recently-completed (done/merged within 30 minutes) tasks
- Flags any files on disk that are outside the task's declared scope glob
- Excludes general main-worktree hygiene (documentation, CI config, build caches)

**Example D8 violation:**
```
D8: Out-of-scope artifacts detected for active or recently-completed tasks
  TASK-001: cmd/main.go
```

If D8 reports violations, investigate whether the artifacts should have been in-scope
(task scope too narrow) or are genuine stray files (need cleanup before merge).

## How Hooks Integrate with Armature Workflow

1. **Coordinator runs `arm ready`** to find unblocked tasks.
2. **Coordinator runs `arm claim <task-id> --worktree <path>`** to reserve a task and create a git worktree. The task ID is written to `<worktree-git-dir>/armature-issue-id`.
3. **Coordinator launches harness** from the worktree directory with `ARMATURE_HOOK_PLATFORM` set. `ARMATURE_ISSUE_ID` is optional when a worktree binding file exists.
4. **Model within harness requests tools** (file edits, shell commands).
5. **Pre-tool hook fires** → `arm harness-hook` reads task binding from file → checks scope → allow/block returned. Scope violations are logged.
6. **Model completes work** and requests harness to stop.
7. **Stop hook fires** → `arm harness-hook` runs verification → allow/block returned.
8. **Coordinator runs `arm transition --to done`** to record completion.
9. **Coordinator runs `arm doctor`** to check repository health, including D8 for out-of-scope artifacts in the completed task.
10. **Coordinator runs `arm merged --issue <task-id>`** to tear down the worktree and record the merge. A warning is emitted if the hook log contains pass-through entries.

## Threat Model

This section documents the hook's enforcement guarantees and limitations across platforms.

### Enforcement Model

The harness hook enforces task scope and acceptance criteria through a **best-effort guardrail** model:

- **When hooks are configured and active** in the harness platform, the hook **mechanically blocks** violating tool invocations (scope violations, acceptance failures) by returning a deny decision to the harness.
- **When hooks are disabled or bypassed**, enforcement is advisory only — the worker can invoke operations that would otherwise be blocked.
- **The hook is never a sandbox.** The fail-open posture (pass-through on error) and symlink-bypass coverage mean enforcement gaps are possible and are surfaced via violation logging, not by preventing operations.
- **All enforcement is recorded** to the worktree-local hook log for later audit, visible during merge validation.

### Platform-Specific Guarantees

#### Claude Code: Advisory (Configurable)

**Enforcement:** When hooks are active in `.claude/settings.json`, the hook **mechanically enforces** scope and acceptance checks.

**Limitation:** Enforcement is **advisory** — a worker can disable or modify hooks by editing `.claude/settings.json` or by invoking operations outside the Claude Code tool interface. Armature provides no mechanism to prevent hook modification.

**Tool classes mediated:**
- **Edit tools:** `Edit`, `Write`, `MultiEdit` (file edits via Claude Code)
- **Shell tools:** `Bash` (shell commands)

**Scope enforcement:** Lexical path checking against task scope globs; symlink-inside-worktree-pointing-outside is treated as in-scope (fail-open posture, see docs/harness-hook.md#Fail-Open Posture).

**Pass-through conditions:**
- No issue binding found (session has no `ARMATURE_ISSUE_ID` or worktree binding file)
- Binding resolution encounters transient errors (file I/O failures, corrupted state)
- Hook event decoding fails (malformed JSON from harness)
- Snapshot loading fails (task state unavailable)

**Recorded evidence:**
- Decision log entries with timestamp, issue ID, resolution step, event kind, tool name, decision (allow/block), and block reason
- Activity log entries (PostToolUse only) with command, exit code, output hash, truncated output head/tail, worktree HEAD commit
- Violation entries for unbound file writes detected at merge time

#### Codex: Advisory (Configurable)

**Enforcement:** When hooks are active in `.codex/config.toml`, the hook **mechanically enforces** scope and acceptance checks.

**Limitation:** Enforcement is **advisory** — a worker can disable or modify hooks by editing `.codex/config.toml` or by invoking operations outside the Codex tool interface. Armature provides no mechanism to prevent hook modification.

**Tool classes mediated:**
- **Edit tools:** `apply_patch`, `Edit`, `Write`
- **Shell tools:** `shell`, `local_shell`, `Bash`

**Scope enforcement:** Lexical path checking (same as Claude Code).

**Pass-through conditions:** Same as Claude Code.

**Recorded evidence:** Same as Claude Code.

#### Devin: Advisory (Configurable)

**Enforcement:** When hooks are active in `.devin/hooks.json`, the hook **mechanically enforces** scope and acceptance checks.

**Limitation:** Enforcement is **advisory** — a worker can disable or modify hooks by editing `.devin/hooks.json` or by invoking operations outside the Devin tool interface. Armature provides no mechanism to prevent hook modification.

**Tool classes mediated:**
- **Edit tools:** `edit`
- **Shell tools:** `exec`

**Scope enforcement:** Lexical path checking (same as Claude Code).

**Pass-through conditions:** Same as Claude Code.

**Recorded evidence:** Same as Claude Code.

### Pass-Through Logging and Violation Detection

**Pass-through entries** (`pass-through:` lines in hook log) are warnings, not enforcement failures. They indicate events that could not be evaluated due to binding resolution or decoding errors and were allowed to proceed. These are fail-open: the hook prefers to let work continue rather than block a worker on transient failures.

**Violation entries** (`violation:` lines in hook log) indicate file writes that should have been subject to scope enforcement but were not — the binding could not be resolved, so no scope check occurred. Violations are surfaced at merge time by the **violation gate** (see docs/harness-hook.md#Violation Gate): `arm merged --issue <task-id>` will fail if violation entries are present unless `--force` is specified.

A wave whose tasks contain violation entries **must not be integrated** without explicit operator override. Violations raise risk for story integration.

### Sensitive Environment Considerations

For sensitive environments (customer infrastructure, air-gapped systems, compliance-critical contexts) containing credentials, tokens, API keys, proprietary data, or PII:

**Execution evidence is recorded by default.** See docs/sensitive-environments.md for:
- How to disable execution evidence capture (repo-level `git config armature.disable-activity-logging`)
- Citation boundary (what appears in published reports vs. what operators see)
- Teardown-after-record ordering constraints

### Summary

| Platform | Enforcement | Mediated Tools (Edit / Shell) | Pass-Through | Violations at Merge |
|----------|------------|---------------------------|---------------|------------------|
| Claude Code | Advisory (per hook config) | Edit, Write, MultiEdit / Bash | Logged, warnings only | Block merge unless --force |
| Codex | Advisory (per hook config) | apply_patch, Edit, Write / shell, local_shell, Bash | Logged, warnings only | Block merge unless --force |
| Devin | Advisory (per hook config) | edit / exec | Logged, warnings only | Block merge unless --force |

All platforms share the same fail-open posture, pass-through conditions, and violation logging. Differences lie in which tool names and event types each platform's harness natively supports.

## See Also

- [Commands Reference](commands.md#harness-hook) — `arm harness-hook` command documentation
- [Configuration Reference](configuration.md#hooks) — Hook configuration in `.armature/config.json`
- [Harness Hooks Guardrail Refactor Design](docs/superpowers/specs/2026-05-26-harness-hooks-guardrail-refactor-design.md) — Technical design details
- [Sensitive Environments and Execution Evidence Disclosure](docs/sensitive-environments.md) — Pass-through conditions, capture configuration, teardown ordering
