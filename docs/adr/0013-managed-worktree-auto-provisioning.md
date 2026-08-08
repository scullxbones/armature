# ADR 0013: Managed Worktree Auto-Provisioning with Boolean Flag

## Status

Accepted

## Principles touched

I1, I3, I4

## Context

ADR 0003 established that `arm claim` always requires `--worktree <path>` to enable reliable task governance. However, the requirement for coordinators to specify explicit worktree paths introduced friction:

1. **Coordinator burden**: Every `arm claim` invocation required the coordinator to construct a path (`/tmp/arm-task-TASK-ID`, `./.worktrees/TASK-ID`, etc.), adding complexity to task dispatch logic.

2. **Path variability**: Different coordinators and environments used different path conventions (tmp directories, project-relative paths, absolute paths), causing inconsistency and making worktree state hard to predict or debug.

3. **Session sandbox friction**: Some harness sandboxes forbid writes to `/tmp`, causing `arm claim --worktree /tmp/...` to fail silently or hang, with no fast-fail error message to alert the coordinator.

4. **Worktree lifecycle ambiguity**: With explicit paths, worktrees could be created anywhere in the filesystem. Some were left behind after session death, others conflicted with session cleanup, and diagnostic tooling (`arm doctor`) had no canonical location to scan.

The core value of ADR 0003 — that `arm claim` owns worktree creation and binding setup — remains sound. The friction lay in requiring the coordinator to specify the path.

## Decision

1. **`--worktree` becomes a boolean flag** (no positional argument).

2. **Worktree auto-provisioning at default root**: when `--worktree` is passed without a path, `arm claim` auto-provisions the worktree at `.worktrees/<issue-id>` (repo-root-relative). This establishes a single, predictable, canonical location for all worktrees within a repository.

3. **`--worktree` remains mandatory** for worker dispatch (unchanged from ADR 0003). Omitting it is an error.

4. **Doctor's missing-worktree detection** now looks for `<repo-root>/.worktrees/<issue-id>` when diagnosing orphaned or stale claims. If the directory is missing despite an active claim, `arm doctor --fix` releases the claim for re-dispatch.

## Consequences

- **Simplified coordinator logic**: `arm claim TASK-ID --worktree` is now the complete, context-free invocation. No path construction needed.

- **Canonical worktree root**: All claimed tasks in a repository are provisioned under `.worktrees/`, making worktree state predictable and scannable by tooling.

- **Sandbox-friendly**: Worktrees live under the repository root (typically writable), avoiding sandbox friction with `/tmp` writes or permission errors.

- **Consistent lifecycle**: Worktrees are nested under `.worktrees/`, which can be documented as a git-ignored directory or included in cleanup scripts consistently across all environments.

- **Tooling integration**: Harness hook and doctor can reliably find worktrees by issue ID without ambiguous path search. Session cleanup scripts can uniformly remove `.worktrees/` without manual path handling.

- **Breaking change for coordinators**: Existing coordinator scripts or prompts that pass explicit paths (`--worktree /tmp/...`) must be updated to use the boolean form (`--worktree`).

- **Documentation and skill updates required**: All embedded examples, skill references, and user-facing docs must be updated to reflect the boolean flag and auto-provisioned location (see LNGHZN-S5-T5 task).

## ADR 0003 Superseded

This ADR supersedes ADR 0003 on the specific point of worktree path specification. ADR 0003's core decision — that `arm claim` always requires `--worktree` for reliable task governance — is retained and reinforced. Only the mechanism (explicit path → boolean auto-provisioning) has evolved based on operational experience.

ADR 0003 remains in the git history as a decision record; it is not edited or removed.
