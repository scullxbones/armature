---
date: 2026-06-30
agent: claude
area: workflow
task: DF-S5
tags: [arm-claim, scope-overlap]
---

# arm claim flags scope overlap against the parent story itself, every time

## User Goal

Coordinating DF-S5 (11 tasks touching 10+ distinct, non-overlapping files) required
claiming every single ready task into its own worktree.

## Observed

Every `arm claim <task> --worktree <path>` call — regardless of which file the task
touched — failed with:

```
Error: cannot claim <task>: scope overlap with DF-S5 (Dogfood Findings Remediation (2026-06-30)) — use --force to override
```

This happened for all 11 tasks across 4 dispatch waves, even when two tasks in the
same wave touched completely different files (e.g. DF-S5-T4 touching
`armature-planner/SKILL.md` vs DF-S5-T7 touching `armature-auditor/SKILL.md`). The
overlap is reported against the **parent story record**, not against another
concurrently-claimed task.

## Impact

Every claim required `--force`, turning a safety check into a rubber stamp. Because
`--force` was needed 11/11 times, it trained the operator (coordinator) to reach for
`--force` reflexively rather than treating the warning as meaningful signal. A real
overlap between two sibling tasks would likely go unnoticed under this pattern.

## Evidence

```
$ arm claim DF-S5-T5 --ttl 90 --worktree ./.worktrees/df-s5-t5
Error: cannot claim DF-S5-T5: scope overlap with DF-S5 (Dogfood Findings Remediation (2026-06-30)) — use --force to override
```
Repeated identically for T8, T9, T4, T7, T2, T6, T3, T10, task-1782866629 — 10 of 11
non-first claims in the story hit this.

## Suggested Follow-Up

Scope-overlap detection should compare a task's scope files against other
*currently-claimed sibling tasks'* scope files, not against the parent story's own
(usually empty or umbrella) scope field. If the story itself legitimately has no
file-level scope, it should be excluded from the overlap check entirely.
