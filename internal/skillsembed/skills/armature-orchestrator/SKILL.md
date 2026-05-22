---
name: armature-orchestrator
description: >
  Use when running Armature's default runtime execution loop. Runs
  `arm worker run` to drain ready work deterministically, handles operational
  escalations, and uses `arm orchestrate --issue` as single-task fallback.
  Requires arm on PATH.
compatibility: Designed for Claude Code and Gemini CLI. Requires arm on PATH.
---

# Armature Orchestrator

This is the default execution path for task delivery in Armature.

Use this loop:
1. Run deterministic runtime execution with `arm worker run`
2. Let the runtime pull ready work and orchestrate tasks
3. Repeat when needed until the queue is empty

## Prerequisites

1. Confirm `arm` is available on PATH.
2. Run `arm doctor` and resolve health errors before dispatch.
3. Confirm your harness binary is installed (`claude`, `codex`, or `devin`) and runnable.
4. Validate sandbox prerequisites for your platform before first run.
5. Ensure project verification tooling (`make check` or equivalent) is installed.

Recommended preflight checks:

```bash
arm doctor
arm worker run --max-tasks 1 --dry-run
```

## Default Runtime Loop

```bash
arm worker run
```

If runtime exits `final_state=idle`, the queue is drained for now.

Use single-task fallback when you need targeted control:

```bash
arm orchestrate --issue TASK-ID --dry-run
```

## Multi-Orchestrator Scaling

Run multiple runtime processes in parallel. Each process independently:

```bash
arm worker run
```

Claim collisions are expected. If a claim is lost, immediately poll `arm ready`
again and continue.

## Escalation Handling

When runtime or orchestration escalates:

1. Inspect issue details: `arm show TASK-ID`
2. Review outcome notes and verification failures
3. Fix root cause (scope, acceptance criteria, test setup, harness/model)
4. Re-run `arm worker run` or use targeted fallback `arm orchestrate --issue TASK-ID`

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
