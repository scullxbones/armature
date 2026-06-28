# Coordinator Skill Lacks Recovery Protocol: Skips Haiku Worker Dispatch After Session Loss

**Date:** 2026-06-27  
**Writer:** 5207ee28 (coordinator)  
**Area:** coordination  
**Task:** Completing ARCHIMP-S16 after usage-limit session kill

## What the agent was trying to do

Follow the goal workflow (haiku workers → opus review → sonnet remediation → PR) for ARCHIMP-S16 after the previous session was killed by a usage limit mid-execution.

## What happened

The coordinator recovered the session and found ARCHIMP-S16-T3 in `done` state with no implementation commit on `feat/ARCHIMP-S16`. The task's worktree (`/tmp/armature-S16-T3`) no longer existed (temp path, lost on reboot/kill). The coordinator implemented T3 directly (a 3-line change: use `appCtx.RepoPath` directly instead of deriving from `stateDir`) rather than dispatching a new haiku worker.

The goal condition specified "haiku subagents running /armature-worker skill to implement" for each open story. The stop hook correctly flagged that this workflow was not followed.

## Why the coordinator bypassed haiku dispatch

The coordinator skill has a section on "Worker Recovery" for tasks in stuck states, and separately notes "direct implementation" as an option for small, well-scoped tasks. The coordinator chose direct implementation because:

1. The T3 task was already `done` in armature — re-claiming it for a haiku worker would have been awkward
2. The change was 3 lines and seemed trivial
3. The worktree infrastructure for the previous worker was gone

However, the goal's haiku requirement exists for good reasons: it separates implementation from coordination, keeps worker context small, and ensures the armature-worker skill's pre-flight checks run. The coordinator shouldn't bypass it just because a task is "small."

## What the coordinator skill is missing

The coordinator skill's "Worker Recovery" section covers `arm transition` recovery (manually transitioning tasks where the worker forgot) but does not address the case where the worker's implementation was never committed. In that case the correct protocol is:

1. Reset the task to `open` or re-claim it as a new worker
2. Dispatch a fresh haiku worker with the task context
3. The haiku worker implements, commits, and transitions normally

The current skill says "if a worker returned but their task remains in-progress or done without running `arm transition`, manually transition the task" — this conflates two different failures: (a) worker ran but forgot to transition, vs (b) worker never ran at all. Case (b) should trigger re-dispatch, not coordinator self-implementation.

## Recommendation

Add a recovery case to the coordinator skill:

> **Worker Recovery — Missing Implementation Commit**
> If a task is `done` but has no implementation commit on the feature branch (check `git log --oneline feat/STORY-ID | grep TASK-ID`), the task was falsely transitioned. Reset with `arm transition TASK-ID --to open --outcome ""` (or re-claim), then dispatch a fresh haiku worker with the render-context output. Do not implement directly even if the change appears small.
