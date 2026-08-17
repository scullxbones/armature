# Theme: Session Recovery Gaps

## Summary

When coordinating sessions are abruptly terminated (e.g., via usage limits), the lack of clear diagnostic/recovery commands and lost temporary worktrees makes reconstructing the repository/task state highly manual, leading coordinators to bypass standard worker dispatch protocols.

A second restart shape has taken over S10: the human keeps merging PRs, nothing runs `arm merged`, and the next session inherits three disagreeing records (handoff, `arm list`, GitHub). That tax is now curated under [i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md); the restart finding stays here because the first hour is still reconstruct, not implementation.

## Evidence

- [Session recovery branch divergence](../../raw/2026-06-27T1954Z-5207ee28-coordination-session-recovery-branch-divergence.md) - Rebuilding state after a session limit was hit required multiple manual git commands because the active task worktree was lost.
- [Recovery skips worker dispatch](../../raw/2026-06-27T2030Z-5207ee28-coordination-recovery-skips-worker-dispatch.md) - The coordinator skill lacked recovery protocols for missing implementation commits, causing the coordinator to implement a task directly instead of spawning a new worker.
- [Worker left task in `claimed` state despite reporting success](../../raw/2026-06-28T2200Z-claude-workflow-worker-left-task-claimed.md) - Haiku worker returned with summary saying "task transitioned to done" but DAG showed task still `claimed`. Required manual `arm transition` + retry of `arm merged`.
- [Human merge protocol leaves arm and the handoff behind GitHub](../../raw/2026-08-15T2036Z-5207ee28-coordination-manual-merge-protocol-stalls-session-restart.md) — Handoff said wait for #103; GitHub already had six S10 PRs merged; `arm list` still had T2/T3/T6/T7 as `done`. Whole restart was reconstruct + `--force` ×4.

## Candidate Follow-Ups

- Add a clear "Worker Recovery — Missing Implementation Commit" protocol to the coordinator skill to guide resetting and re-dispatching workers instead of self-implementing.
- Save worktree paths in the armature task claim record to avoid ambiguity about where work occurred.
- Re-evaluate using `/tmp` for worktrees that need to persist across usage limits/system restarts, or build cleanup and re-dispatch routines when worktree directories are missing.
- Stop treating `/tmp` handoffs as a source of truth; `arm ready --explain` already diagnosed the S10 restart. See [i6-promotion-agent-owned](../i6-promotion-agent-owned/README.md) for the close-the-loop product fix.
