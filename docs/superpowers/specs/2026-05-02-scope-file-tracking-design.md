# Scope File Tracking: Rename and Delete

**Date:** 2026-05-02
**Status:** Approved for implementation

## Problem

Issue scopes contain file paths and glob patterns. When files are renamed or deleted during normal refactoring, scope entries become stale. The existing W10 phantom scope check surfaces these as `INFO: phantom scope:` lines on every `arm validate` run, creating noise that obscures the check's original purpose: catching typos or invalid globs on active work items.

There are three distinct phantom scope cases that the current check conflates:

1. **Typo / never-existed** — the original intent; scope was invalid at creation time
2. **File renamed/moved** — the file exists elsewhere; the scope entry needs updating
3. **File deleted** — the file is intentionally gone; the scope entry should be removed

Cases 2 and 3 require event-sourced tracking so that materialized state remains correct on replay.

## Aggregate Model

The issue is the aggregate root. Every existing op targets a specific issue via `TargetID`. The log is per-worker and append-only; materialization merges all worker logs by timestamp at read time. There is no workspace aggregate.

`scope-rename` and `scope-delete` follow this model: each op targets one issue (`TargetID = issueID`). When a file rename or deletion affects N issues, N ops are emitted — one per affected issue — all at the same timestamp from the same worker. This preserves per-issue history and is consistent with the existing single-writer contract.

Ops from the same batch command or hook share a timestamp and worker ID, which is sufficient to correlate them as a group without a dedicated causality field.

## Design

### New Op Types

Two new op types are added to `internal/ops/types.go`:

```
OpScopeRename = "scope-rename"
OpScopeDelete = "scope-delete"
```

Both are worker-attributed, timestamped, and target a specific issue via `TargetID`. Both are added to `ValidOpTypes`.

**Implementation ordering constraint:** The `default` branch in `ApplyOp` returns an error for unknown op types. Both new cases must be added to `ApplyOp` in the same commit — adding one without the other will cause replay to hard-error on any log containing the missing type.

#### `scope-rename`

**New payload fields in `internal/ops/types.go` `Payload` struct** (add under a `// scope-rename` comment group, consistent with the existing per-op-type comment pattern in that file):
```go
// scope-rename
OldPath string `json:"old_path,omitempty"`
NewPath string `json:"new_path,omitempty"`
```

The materializer applies `strings.ReplaceAll(entry, OldPath, NewPath)` to every scope entry on the targeted issue. `OldPath` and `NewPath` are treated as substrings, not necessarily full paths, so a single op correctly handles both exact paths and glob patterns:

- Exact entry `cmd/trellis/main.go` with `OldPath=cmd/trellis/main.go` → exact replacement
- Glob entry `cmd/trellis/*.go` with `OldPath=cmd/trellis` → `cmd/armature/*.go`

`applyScopeRename` updates `Updated` only if at least one scope entry changed. If `OldPath` does not appear in the issue's scope, the op is a no-op and `Updated` is unchanged.

**Idempotency:** On replay, a second application of the same op finds nothing matching `OldPath` (already rewritten) and is a no-op. The op never creates scope entries — it only transforms existing ones.

**`OldPath == NewPath` guard:** The command rejects this with an error. No op is written.

#### `scope-delete`

**New payload field in `internal/ops/types.go` `Payload` struct** (add under a `// scope-delete` comment group):
```go
// scope-delete
DeletedPath string `json:"deleted_path,omitempty"`
```

The materializer removes any scope entry on the targeted issue where `entry == DeletedPath` (exact string match only). Glob entries are not removed; a glob covering deleted files will surface via the W10 phantom scope check on the next `arm validate` for active issues.

`applyScopeDelete` updates `Updated` only if at least one entry was removed. If `DeletedPath` does not appear as an exact entry, the op is a no-op and `Updated` is unchanged.

**Rationale for exact-only:** Removing a glob because one of its matches disappeared requires filesystem evaluation at op-apply time, which breaks replay determinism. Glob cleanup is left to the user via `arm amend`.

**Warning on empty scope:** The `arm scope-delete` command inspects current materialized state before emitting ops. If any non-terminal issue (status not in `merged`, `done`, `cancelled`) would have an empty scope after deletion — including `blocked` issues — the command prints a warning listing those issue IDs before proceeding. The materializer's `applyScopeDelete` is silent; warnings are surfaced at command time only, not on every replay.

### Materializer Changes (`internal/materialize/engine.go`)

`ApplyOp` gains two new cases:

```go
case ops.OpScopeRename:
    return s.applyScopeRename(op)
case ops.OpScopeDelete:
    return s.applyScopeDelete(op)
```

Both methods look up `s.Issues[op.TargetID]`. If the issue is not found, they return `nil`. Note: the codebase has two patterns — `applyClaim`, `applyTransition`, `applyLink`, and `applyUnlink` hard-error on unknown issues; `applyNote`, `applyDecision`, `applyAssign`, and others return `nil`. The `nil` pattern is correct here because `scope-rename` and `scope-delete` ops are emitted in bulk and may outlive individual issues in a long-lived log; hard-erroring on a missing issue would break full replay if an issue was later cancelled or removed. They operate only on the targeted issue — no cross-issue scanning.

`ApplyOp`'s `error` return signature is unchanged. No `Warnings` field is added to `State`.

### New Commands

Both commands load current materialized state, identify affected issues, emit one op per affected issue at the same timestamp, then rematerialize.

#### `arm scope-rename <old-path> <new-path>`

1. Reject if `old-path == new-path` or either is empty, with an error.
2. Load current materialized state.
3. Find all issues whose scope contains `old-path` as a substring. If none, print a warning (`no scope entries reference <old-path>`) and exit 0 without writing any ops.
4. Print a summary of affected issues (count and IDs).
5. Emit one `scope-rename` op per affected issue, all at the same timestamp.
6. Rematerialize.

#### `arm scope-delete <path>`

1. Reject if `path` is empty, with an error.
2. Load current materialized state.
3. Find all issues whose scope contains `path` as an exact entry. If none, print a warning (`no scope entries contain <path>`) and exit 0 without writing any ops.
4. Warn if any of the affected non-terminal issues would have an empty scope after deletion; include the issue IDs. This includes `blocked` issues. The command proceeds regardless — this is a warning, not a hard stop.
5. Print affected issues.
6. Emit one `scope-delete` op per affected issue, all at the same timestamp.
7. Rematerialize.

Both commands are plumbed through the existing op-write path consistent with `arm amend`, `arm note`, etc.

### Git Hook Auto-Detection

The post-commit hook is extended to detect file renames and deletions from the most recent commit.

**Renames:** Parse the output of:
```
git diff --diff-filter=R --name-status HEAD~1 HEAD
```
Output lines are tab-separated; the first field starts with `R` (e.g. `R` or `R100` depending on Git version and rename-detection configuration). Use `strings.HasPrefix(field, "R")` to identify rename lines — do not assume a fixed `R<score>` format. Take fields at index 1 (old path) and 2 (new path). For each rename, find all issues whose scope contains the old path as a substring. Emit one `scope-rename` op per affected issue. If no issues reference the old path, skip — no ops written.

No directory-prefix grouping is attempted; users with glob scope entries spanning a renamed directory should run `arm scope-rename <old-prefix> <new-prefix>` explicitly.

**Deletions:** Parse the output of:
```
git diff --diff-filter=D --name-status HEAD~1 HEAD
```
Output lines have the form `D\t<path>`. Split on `\t`; take field at index 1. For each deleted file, find all issues whose scope contains the exact path. Emit one `scope-delete` op per affected issue. Files not referenced in any scope are ignored. Glob-only scope entries covering a deleted file are not detected by the hook; the narrowed W10 check will surface them on the next `arm validate`.

**Edge cases:** If the repo has fewer than two commits (i.e. `HEAD~1` does not exist, including an empty repo where `HEAD` itself is unborn), this step is skipped entirely. If either `git diff` invocation exits non-zero for any other reason (detached HEAD, shallow clone, etc.), the hook skips scope detection for that invocation and exits cleanly — hook failures must not block commits. The `HEAD~1 HEAD` range is intentionally simple; it will produce incorrect results for amended commits (where `HEAD~1` is the pre-amend parent, not the previous commit state) and for commits applied during a rebase. These are known limitations of the best-effort hook; users in those workflows should run `arm scope-rename` or `arm scope-delete` explicitly. The hook is best-effort; users who rename or delete files without committing should use the explicit commands directly.

### W10 Phantom Scope Narrowing

**This is a change to the existing implementation.** `checkW10PhantomScope` currently iterates all issues unconditionally; this spec adds a status filter.

`checkW10PhantomScope` in `internal/validate/validate.go` is updated to skip issues of **any type** (task, story, epic) whose status is `merged`, `done`, or `cancelled`. Terminal issues had valid scope when work was completed; phantom entries there are noise.

`StatusBlocked` is intentionally **not** skipped — blocked issues are still active work items and should have valid scope.

Note: routing to `infos` is enforced at the **call site** in `Validate()` (`infos = append(infos, checkW10PhantomScope(...)...)`), not inside the function. The function's internal variable is named `warns` but that is an implementation detail. Do not change the call-site routing; the return value must continue to be appended to `infos`.

The check retains its original purpose: flag typos and invalid globs on active work items.

### Migration

Existing stale scope entries (e.g. `cmd/trellis/` paths remaining from the Trellis → Armature rename) are resolved by running:

```
arm scope-rename cmd/trellis cmd/armature
```

This emits one `scope-rename` op per affected issue. No special migration command is needed.

## Testing

- Unit tests for `applyScopeRename`: exact path replacement, glob pattern replacement, no-op when `OldPath` absent (`Updated` unchanged), idempotency on double-apply, unknown `TargetID` tolerated.
- Unit tests for `applyScopeDelete`: exact removal, glob entries left intact, no-op when `DeletedPath` absent (`Updated` unchanged), unknown `TargetID` tolerated.
- Unit tests for `checkW10PhantomScope`: terminal issues of all types (`merged`, `done`, `cancelled`) skipped; `blocked` issues still checked; epics and stories with terminal status also skipped.
- Integration tests for `arm scope-rename`: empty-arg rejection, `OldPath == NewPath` rejection, existence guard, N ops emitted for N affected issues at same timestamp, state updated correctly.
- Integration tests for `arm scope-delete`: empty-arg rejection, existence guard, warning printed for non-terminal issues with empty-after-deletion scope, N ops emitted for N affected issues, state updated correctly.
- Integration test for hook auto-detection: simulated commit with rename and delete, correct per-issue ops emitted only for scope-referenced paths; initial-commit edge case skipped.

## Interaction with `arm amend`

`applyAmend` replaces the issue's scope list wholesale when `len(op.Payload.Scope) > 0`. If a user runs `arm amend` with a scope argument after `scope-rename` or `scope-delete` ops have been applied, the amended scope list overwrites the previously renamed/deleted entries. This is existing behaviour and is not changed by this spec. Users should treat `arm amend --scope` as an authoritative override, and use `arm scope-rename` / `arm scope-delete` for refactoring-driven corrections.

## Out of Scope

- Fuzzy or pattern-based glob removal on file deletion (deferred; breaks replay determinism).
- Directory-prefix grouping in the hook for glob scope entries (deferred; users run `arm scope-rename <prefix>` explicitly).
- `arm scope-rename` operating on live working-tree changes before commit (deferred; hook covers the commit path).
