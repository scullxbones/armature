---
name: armature-orchestrator
description: >
  Use when running Armature's default pull-model execution loop. Polls ready
  work, runs `arm orchestrate`, handles retries/escalations, and scales out via
  concurrent orchestrator processes. Requires arm on PATH.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Orchestrator

This is the default execution path for task delivery in Armature.

Use this loop:
1. Pull a ready task with `arm ready`
2. Run deterministic orchestration with `arm orchestrate --issue ID`
3. Repeat until the queue is empty

## Prerequisites

1. Confirm `arm` is available on PATH.
2. Run `arm doctor` and resolve health errors before dispatch.
3. Confirm your harness binary is installed (`claude`, `codex`, or `devin`) and runnable.
4. Validate sandbox prerequisites for your platform before first run.
5. Ensure project verification tooling (`make check` or equivalent) is installed.

Recommended preflight checks:

```bash
arm doctor
arm orchestrate --issue TASK-ID --dry-run
```

## Single-Orchestrator Loop

```bash
arm ready
arm orchestrate --issue TASK-ID
```

If orchestration succeeds, repeat with the next ready task.

If orchestration fails before dispatch, use dry-run to inspect state:

```bash
arm orchestrate --issue TASK-ID --dry-run
```

## Multi-Orchestrator Scaling

Run multiple orchestrator processes in parallel. Each process independently:

```bash
arm ready
arm orchestrate --issue TASK-ID
```

Claim collisions are expected. If a claim is lost, immediately poll `arm ready`
again and continue.

## Escalation Handling

When orchestration escalates:

1. Inspect issue details: `arm show TASK-ID`
2. Review outcome notes and verification failures
3. Fix root cause (scope, acceptance criteria, test setup, harness/model)
4. Re-run `arm orchestrate --issue TASK-ID`

Use explicit model override when needed:

```bash
arm orchestrate --issue TASK-ID --model <model-id>
```

## Operator Checklist

After each wave (or periodically):

```bash
arm list --group
arm validate
make check
```

Goal: keep the ready queue draining while maintaining deterministic verification.

## When To Use Manual Worker Flow

Use manual worker flow only when orchestration cannot proceed due to environment
or policy constraints requiring human-directed execution. In those cases, switch
to the `armature-worker` skill.
