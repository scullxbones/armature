# Coordinator DX Improvements — Source Document

Origin: post-implementation reflection on story-1777766982 (Skill Optimization).

## Problems Identified

### 1. `arm ready --explain` does not suggest `arm merged`

When a task is blocked by a blocker that is `done` but not `merged`, `arm ready --explain`
outputs "blocker(s) not merged: X" with no actionable hint. The coordinator must already
know to run `arm merged --issue X`. The fix: detect when a blocker is `done` (not missing,
not cancelled) and append "run: arm merged --issue X" to the reason string.

### 2. `make check` does not enforce DoD constraints on skill bodies

The acceptance criterion for SKOPT-2A required that `make install` not appear in SKILL.md
bodies, but no automated check enforced this. Workers self-reported done, and the violation
was caught only at audit time. A `make validate-skills` target should run grep assertions
and be wired into `make check`.

### 3. `deploy-skills` requires manual extension per new subdir type

The Makefile `deploy-skills` target has a separate `if [ -d scripts ]` block and a separate
`if [ -d references ]` block. Any new subdir type requires another manual block. Replace
with `cp -r` of the entire skill source directory.

### 4. Coordinator skill does not document background agent Bash limitation

Background agents dispatched via the Agent tool do not inherit the parent session's Bash
permissions. Agents that require shell commands block silently waiting for approvals that
never arrive. The coordinator skill should document this limitation and the recommended
workaround (direct implementation, or manual worktrees with foreground agents).

### 5. Coordinator skill does not include worktree cleanup step

The "After Workers Return" checklist has no cleanup step. Worktrees from parallel waves
accumulate silently. Add `git worktree remove` to the integration checklist.
