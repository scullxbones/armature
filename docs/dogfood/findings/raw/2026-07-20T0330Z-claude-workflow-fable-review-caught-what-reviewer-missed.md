---
date: 2026-07-20
agent: claude
area: workflow
task: TOPTIER-S4 coordination
tags: [code-review, fable, defense-in-depth, gate-effectiveness]
---

# Holistic fable /code-review caught a critical bug both the per-task armature-reviewer and auditor missed

## User Goal

Per the user's instructions, after all TOPTIER-S4 tasks were merged and reviewed
(armature-reviewer, task-scoped) and the story was audited (armature-auditor,
structural/citation checks) and transitioned to `done`, a fable subagent was
dispatched to run a holistic `/code-review` over the whole story's diff before
opening the PR — an extra, non-task-scoped pass.

## Observed

TOPTIER-S4-T2's `internal/doctor/fix.go` added a "missing worktree" remediation:
`arm doctor --fix` releases/blocks a claimed or in-progress issue whose task worktree
branch no longer exists. The armature-reviewer had assessed T2 twice (yellow both
times) and never flagged this. The armature-auditor checked citation coverage, source
freshness, outcome quality, and repo health — none of which would surface a runtime
correctness bug. Only the fable holistic pass caught it: `GitWorktreeBranches`
silently swallowed every git failure (not-a-repo, transient error, bad `repoPath`) and
returned an empty map with a nil error; `PlanFixes` then read that empty map as "no
issue anywhere has a live worktree," which would misfire the missing-worktree check
against *every* currently claimed/in-progress issue in the graph from a single
transient git error — precisely the mass-false-positive failure mode the story text
explicitly said it was declining to risk for a different (declined) feature. The
`PlanFixes` doc comment itself claimed the check was "skipped when repoPath is empty
or not a git repo," which was also false.

## Impact

This is exactly the kind of defect that task-scoped semantic review (which trusts the
diff against its own stated acceptance criteria) and structural auditing (citation
coverage, outcome quality) are not designed to catch: a subtle error-handling gap
whose blast radius only becomes visible when reasoning about the code's behavior
under failure, holistically, against the story's own stated safety intent — the
armature-reviewer's transcript never mentions `GitWorktreeBranches` or its error
path at all across two review passes. The fix (propagate the real error; skip the
whole missing-worktree pass, not just the current branch's lookup, on any failure)
took under 10 minutes once flagged, but would have shipped `arm doctor --fix` with a
latent bug capable of mass-reopening or mass-blocking active work on a routine git
hiccup, in a command explicitly designed to be safe to run unattended.

## Evidence

- Fable review's finding #1 (verbatim in transcript): "`GitWorktreeBranches`
  error-swallowing turns 'couldn't determine' into 'confirmed no worktree,' risking a
  fleet-wide false-positive misfire in `arm doctor --fix`" — citing
  `internal/adapters/shell.go:148-164` and `internal/doctor/fix.go:101,109`.
- Two prior `armature-reviewer` assessments recorded via `arm review record` for
  TOPTIER-S4-T2 (bundle ids `991711ad...` and `b8cd4910...` contract fingerprint) —
  neither mentions the error-handling path at all.
- `arm doctor --strict` and `arm sources verify` (auditor steps) are structurally
  orthogonal to this class of bug and correctly did not catch it either — this is not
  a criticism of those gates, it's evidence the gates are complementary, not
  redundant.
- Fix landed as commit `359c0d6b` on `feat/TOPTIER-S4`, with a new regression test
  (`TestPlanFixes_GitFailure_SkipsMissingWorktreeCheckEntirely`) that fails against
  the pre-fix code.

## Suggested Follow-Up

Keep the holistic fable pass as a standard step in the coordinator flow — this
session is direct evidence it finds a different class of bug than the per-task
reviewer and the auditor, not overlapping coverage. Consider whether the
armature-reviewer's own prompt should explicitly ask "does this change's error
handling match its own safety claims in comments/docs?" as a targeted heuristic,
since that's precisely the gap here — the code's own doc comment described a safer
behavior than the implementation delivered, and no reviewer pass cross-checked the
two.
