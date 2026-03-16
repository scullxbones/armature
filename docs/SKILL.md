# Trellis AI Worker Interface

This document describes the Trellis task management interface for AI agents. Trellis uses an append-only operation log (op log) stored in git to track tasks, with materialized state derived from replaying those ops.

## Invoking `trls`

The `trls` binary is bundled with this skill at `scripts/trls` (relative to the skill root). Use the relative path when running commands:

```
scripts/trls <command> [flags]
```

All examples in this document use the bare name `trls` for readability — substitute `scripts/trls` when invoking from an agent context.

## Overview

Trellis is a lightweight, git-native task tracking system designed for use in automated workflows. Key properties:

- **Append-only op log**: All state changes are recorded as immutable JSONL entries. State is derived by replaying the log.
- **Materialized state**: The current view of issues and claims is computed from the op log at read time.
- **Solo-workflow / single-branch mode**: In the default mode, all ops are committed directly to the working branch. There is no separate issues branch. When a task transitions to `done`, it is automatically treated as `merged`.

---

## Worker Initialization

Before performing any work, an AI agent must register itself as a worker:

```
trls worker-init
```

This generates a unique worker ID and stores it in git config:

```
trellis.worker-id = <uuid>
```

The worker ID is used to attribute ops and claims. Run this once per agent instance.

---

## Finding Ready Work

```
trls ready
```

Lists tasks that are currently actionable. A task is ready if:

- State is `open`
- Not currently claimed by another worker (or claim has expired)
- Not blocked by any incomplete dependencies
- Parent issue (if any) is `in-progress`, or there is no parent

The output lists issue IDs and titles. Pick an issue from this list to work on.

---

## Claiming a Task

```
trls claim --issue ISSUE-ID [--ttl 3600]
```

Claims the specified issue for this worker. `--ttl` sets the claim duration in seconds (default: 3600). While claimed, other workers will not see this issue in `trls ready`.

Claims expire automatically. Use `trls heartbeat` to keep the claim alive during long-running work.

---

## Context Assembly

```
trls render-context --issue ISSUE-ID [--budget 4000]
```

Assembles context relevant to the issue: the issue description, definition of done, parent/child relationships, linked decisions, and recent notes. `--budget` limits output to approximately that many tokens.

Read this output before beginning work on an issue.

---

## Doing Work

While working on a claimed issue, record progress using the following commands.

### Progress Note

```
trls note --issue ISSUE-ID --msg "message"
```

Records a free-form progress note. Use this to log what you are doing, what you found, or intermediate results.

### Decision

```
trls decision --issue ISSUE-ID --topic "X" --choice "Y" --rationale "Z"
```

Records a decision made during the work. Use this any time you make a non-trivial choice about approach, design, or implementation.

### Heartbeat

```
trls heartbeat --issue ISSUE-ID
```

Refreshes the claim TTL. Call this at most once per minute while doing long-running work. Failing to heartbeat will cause the claim to expire, making the issue visible to other workers.

**Rate limit**: maximum 1 heartbeat per minute per issue.

---

## Completing Work

```
trls transition --issue ISSUE-ID --to done --outcome "what was accomplished"
```

Marks the issue as done and records the outcome. In single-branch mode, done issues are automatically treated as merged.

Other valid target states:
- `--to cancelled` — work was abandoned
- `--to blocked` — work cannot proceed due to an external dependency

---

## Creating Sub-Issues

```
trls create --title "X" --type task --parent PARENT-ID
```

Creates a new issue as a child of an existing issue. Valid types: `task`, `feature`, `bug`.

**Rate limit**: maximum 500 total issues per repository.

---

## Decomposing Plans

```
trls decompose-apply --plan plan.json
```

Bulk-creates issues from a plan file (JSONL format version 1). This is useful for loading a structured set of issues at once, preserving parent/blocked_by relationships.

See `docs/plan-post-bootstrap.json` for an example plan file.

---

## Validation

```
trls validate [--ci]
```

Validates the op log and materialized state for consistency. Use `--ci` for machine-readable output suitable for CI pipelines. Run this after any bulk operation.

---

## Op Log Format

The op log is a JSONL file where each line is a JSON array with positional fields:

```
[op_type, target_id, timestamp, worker_id, payload]
```

| Position | Field       | Type   | Description                              |
|----------|-------------|--------|------------------------------------------|
| 0        | `op_type`   | string | Operation type (e.g., `create`, `claim`) |
| 1        | `target_id` | string | Issue ID this op applies to              |
| 2        | `timestamp` | string | RFC3339 timestamp                        |
| 3        | `worker_id` | string | Worker that wrote this op                |
| 4        | `payload`   | object | Op-specific data                         |

The log is append-only. Never modify or delete existing entries.

---

## Issue States

Issues follow this state machine:

```
open → in-progress → done
                   → cancelled
                   → blocked → in-progress
```

In single-branch mode, `done` issues are automatically transitioned to `merged` (no separate merge step required).

**State descriptions:**
- `open`: Issue exists and is ready to be worked
- `in-progress`: Issue is actively being worked (claimed)
- `done`: Work is complete
- `cancelled`: Work was abandoned
- `blocked`: Work cannot proceed pending external resolution
- `merged`: Work is complete and integrated (auto in single-branch mode)

---

## Rate Limits

| Operation   | Limit                    |
|-------------|--------------------------|
| Heartbeat   | 1 per minute per issue   |
| Create      | 500 total per repository |

Exceeding these limits will result in an error. Design your workflows to stay within these bounds.

---

## Quick Reference

```
scripts/trls worker-init                                          # register this agent
scripts/trls ready                                               # find work to do
scripts/trls claim --issue ID [--ttl 3600]                       # claim an issue
scripts/trls render-context --issue ID [--budget 4000]           # read context
scripts/trls note --issue ID --msg "..."                         # log progress
scripts/trls decision --issue ID --topic "X" --choice "Y" \
                      --rationale "Z"                            # record a decision
scripts/trls heartbeat --issue ID                                # keep claim alive
scripts/trls transition --issue ID --to done --outcome "..."     # complete work
scripts/trls create --title "X" --type task --parent ID          # create sub-issue
scripts/trls decompose-apply --plan plan.json                    # bulk load issues
scripts/trls validate [--ci]                                     # validate state
```
