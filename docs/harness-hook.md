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

## Task Binding

The hook discovers the active task ID through two mechanisms, tried in order:

### 1. Worktree binding file (preferred)
When a task is claimed with `arm claim --worktree <path>`, the task ID is written to
`<worktree-git-dir>/armature-task-id` (e.g., `<parent>/.git/worktrees/<name>/armature-task-id`).
The hook reads this file automatically when invoked inside the worktree — no environment variable is needed.

This is the recommended approach: claim the task with `--worktree`, launch the harness from that
worktree directory, and set only `ARMATURE_HOOK_PLATFORM`.

### 2. `ARMATURE_TASK_ID` environment variable (fallback)
If the binding file is absent (e.g., the worktree was not created via `arm claim --worktree`),
the hook falls back to the `ARMATURE_TASK_ID` environment variable.

## Environment Variables

When launching an external harness, set these variables in the harness environment:

### `ARMATURE_TASK_ID` (fallback only)
The active Armature task ID for the worker. Only required when NOT using `arm claim --worktree`.
When a worktree binding file is present, this variable is ignored.

**Example:**
```bash
export ARMATURE_TASK_ID=TASK-001
```

### `ARMATURE_HOOK_PLATFORM` (required)
The harness platform type. Controls hook input/output encoding.

**Valid values:** `claude`, `codex`, `devin`

**Example:**
```bash
export ARMATURE_HOOK_PLATFORM=claude
```

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
export ARMATURE_TASK_ID=TASK-001
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
export ARMATURE_TASK_ID=TASK-001
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
export ARMATURE_TASK_ID=TASK-001
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
2. **Check citation state:** If the task has `source_links`, all linked sources must be accepted (recorded via `arm accept-citation`).
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

**Cause:** Neither the worktree binding file (`<git-dir>/armature-task-id`) nor the
`ARMATURE_TASK_ID` environment variable is present when the harness starts.

**Fix (preferred):** Claim the task with `--worktree` before launching the harness:
```bash
arm claim TASK-001 --worktree ./task-001-work
# then launch harness from the worktree directory
cd ./task-001-work
ARMATURE_HOOK_PLATFORM=claude claude .
```

**Fix (fallback):** Set the environment variable before launching the harness:
```bash
export ARMATURE_TASK_ID=TASK-001
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
3. For citation failures, run `arm accept-citation <task-id>` to accept all sources

## Manual Configuration Without an Installer

Armature does not ship a harness hook installer. Configuration is manual:

1. **Identify your harness platform** (Claude Code, Codex, or Devin).
2. **Copy the JSON/TOML snippet** from the relevant section above into your config file.
3. **Set `ARMATURE_TASK_ID` and `ARMATURE_HOOK_PLATFORM`** when launching the harness.
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

## How Hooks Integrate with Armature Workflow

1. **Coordinator runs `arm ready`** to find unblocked tasks.
2. **Coordinator runs `arm claim <task-id> --worktree <path>`** to reserve a task and create a git worktree. The task ID is written to `<worktree-git-dir>/armature-task-id`.
3. **Coordinator launches harness** from the worktree directory with `ARMATURE_HOOK_PLATFORM` set. `ARMATURE_TASK_ID` is optional when a worktree binding file exists.
4. **Model within harness requests tools** (file edits, shell commands).
5. **Pre-tool hook fires** → `arm harness-hook` reads task binding from file → checks scope → allow/block returned.
6. **Model completes work** and requests harness to stop.
7. **Stop hook fires** → `arm harness-hook` runs verification → allow/block returned.
8. **Coordinator runs `arm transition --to done`** to record completion.
9. **Coordinator runs `arm merged --issue <task-id>`** to tear down the worktree and record the merge. A warning is emitted if the hook log contains pass-through entries.

## See Also

- [Commands Reference](commands.md#harness-hook) — `arm harness-hook` command documentation
- [Configuration Reference](configuration.md#hooks) — Hook configuration in `.armature/config.json`
- [Harness Hooks Guardrail Refactor Design](docs/superpowers/specs/2026-05-26-harness-hooks-guardrail-refactor-design.md) — Technical design details
