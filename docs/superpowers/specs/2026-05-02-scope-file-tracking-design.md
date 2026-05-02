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

## Design

### New Op Types

Two new op types are added to `internal/ops/types.go`:

```
OpScopeRename = "scope-rename"
OpScopeDelete = "scope-delete"
```

Both are worker-attributed and timestamped. Both are added to `ValidOpTypes`.

Both ops are **cross-cutting** — they affect all issues rather than a single target. `TargetID` is written as `""` (empty string) in the log for these ops, which is valid since the ops carry their operands in the payload rather than in `TargetID`.

#### `scope-rename`

**New payload fields in `internal/ops/types.go` `Payload` struct:**
```go
OldPath string `json:"old_path,omitempty"`
NewPath string `json:"new_path,omitempty"`
```

The materializer applies `strings.ReplaceAll(entry, OldPath, NewPath)` to every scope entry across all issues. `OldPath` and `NewPath` are treated as substrings, not necessarily full paths, so a single op correctly handles both exact paths and glob patterns:

- Exact entry `cmd/trellis/main.go` with `OldPath=cmd/trellis/main.go` → exact replacement
- Glob entry `cmd/trellis/*.go` with `OldPath=cmd/trellis` → `cmd/armature/*.go`

A directory rename can therefore be expressed as one op using the directory prefix as old/new, updating all scope entries (both exact and glob) in one pass.

`applyScopeRename` records `Updated` only on issues whose scope entries actually changed. Issues with no matching entries are left untouched.

**Idempotency:** On replay, a second application of the same op finds nothing matching `OldPath` (already rewritten) and is a no-op. The op never creates scope entries — it only transforms existing ones.

**Existence guard:** Before emitting the op (from command or hook), current materialized state is checked. If `OldPath` does not appear as a substring in any issue's scope entries, no op is written.

**`OldPath == NewPath` guard:** The command rejects this with an error. No op is written.

#### `scope-delete`

**New payload field in `internal/ops/types.go` `Payload` struct:**
```go
DeletedPath string `json:"deleted_path,omitempty"`
```

The materializer removes any scope entry where `entry == DeletedPath` (exact string match only). Glob entries are not removed; a glob covering deleted files will surface via the W10 phantom scope check on the next `arm validate` for active issues.

`applyScopeDelete` records `Updated` only on issues that had the entry removed. Issues with no matching exact entry are left untouched.

**Rationale for exact-only:** Removing a glob because one of its matches disappeared requires filesystem evaluation at op-apply time, which breaks replay determinism. Glob cleanup is left to the user via `arm amend`.

**Warning on empty scope:** The `arm scope-delete` command inspects current materialized state before emitting the op. If any issue of any type (task, story, epic) whose status is not `merged`, `done`, or `cancelled` would have an empty scope after deletion, the command prints a warning listing those issues before proceeding. `StatusBlocked` is not terminal — blocked issues are warned about. The materializer's `applyScopeDelete` method is silent; warnings are surfaced at command time only, not on every replay.

### Materializer Changes (`internal/materialize/engine.go`)

`ApplyOp` gains two new cases:

```go
case ops.OpScopeRename:
    return s.applyScopeRename(op)
case ops.OpScopeDelete:
    return s.applyScopeDelete(op)
```

`applyScopeRename` iterates all issues, applies substring replacement to each scope entry, records `Updated` timestamp only on issues with at least one changed entry.

`applyScopeDelete` iterates all issues, removes exact-matching entries, records `Updated` only on issues with at least one removed entry. No warnings are produced during op application; see command section for warning behavior.

`ApplyOp`'s `error` return signature is unchanged. No `Warnings` field is added to `State`.

### New Commands

#### `arm scope-rename <old-path> <new-path>`

1. Reject if `old-path == new-path` or either is empty, with an error.
2. Load current materialized state.
3. Check whether `old-path` appears as a substring in any issue scope. If not, print a warning (`no scope entries reference <old-path>`) and exit 0 without writing an op.
4. Print a summary of affected issues (count and IDs).
5. Emit `scope-rename` op.
6. Rematerialize.

#### `arm scope-delete <path>`

1. Reject if `path` is empty, with an error.
2. Load current materialized state.
3. Check whether `path` appears as an exact entry in any issue scope. If not, warn and exit 0.
4. Warn if any non-terminal issue (status not in `merged`, `done`, `cancelled`) of any type would have an empty scope after deletion; include the issue IDs. This includes `blocked` issues.
5. Print affected issues.
6. Emit `scope-delete` op.
7. Rematerialize.

Both commands are plumbed through the existing op-write path consistent with `arm amend`, `arm note`, etc.

### Git Hook Auto-Detection

The post-commit hook is extended to detect file renames and deletions from the most recent commit.

**Renames:** Parse the output of:
```
git diff --diff-filter=R --name-status HEAD~1 HEAD
```
Output lines have the form `R<score>\t<old-path>\t<new-path>` (tab-separated, first field starts with `R`). Split on `\t`; take fields at index 1 (old path) and 2 (new path). For each rename, emit one `scope-rename` op using the exact old and new file paths — provided the existence guard passes (at least one scope entry contains the old path as a substring). No directory-prefix grouping is attempted; users with glob scope entries spanning a renamed directory should run `arm scope-rename <old-prefix> <new-prefix>` explicitly.

**Deletions:** Parse the output of:
```
git diff --diff-filter=D --name-status HEAD~1 HEAD
```
Output lines have the form `D\t<path>`. Split on `\t`; take field at index 1. For each deleted file, emit one `scope-delete` op — provided at least one issue scope contains the exact path. Files not referenced in any scope are ignored. Glob-only scope entries covering a deleted file are not detected by the hook; the narrowed W10 check will surface them on the next `arm validate`.

**Edge case:** If the commit is the initial commit (no `HEAD~1`), this step is skipped. The hook is best-effort; users who rename or delete files without committing should use the explicit commands directly.

### W10 Phantom Scope Narrowing

`checkW10PhantomScope` in `internal/validate/validate.go` is updated to skip issues of **any type** (task, story, epic) whose status is `merged`, `done`, or `cancelled`. Terminal issues had valid scope when work was completed; phantom entries there are noise.

`StatusBlocked` is intentionally **not** skipped — blocked issues are still active work items and should have valid scope.

The check retains its original purpose: flag typos and invalid globs on active work items.

### Migration

Existing stale scope entries (e.g. `cmd/trellis/` paths remaining from the Trellis → Armature rename) are resolved by running:

```
arm scope-rename cmd/trellis cmd/armature
```

No special migration command is needed — the new op is sufficient and will be recorded in the event log like any other op.

## Testing

- Unit tests for `applyScopeRename`: exact path replacement, glob pattern replacement, no-op when `OldPath` absent, idempotency on double-apply, `Updated` only set on modified issues, no-op when `OldPath == NewPath`.
- Unit tests for `applyScopeDelete`: exact removal, glob entries left intact, `Updated` only set on modified issues, unaffected issues untouched.
- Unit tests for `checkW10PhantomScope`: terminal issues of all types (`merged`, `done`, `cancelled`) skipped; `blocked` issues still checked; epics and stories with terminal status also skipped.
- Integration tests for `arm scope-rename`: empty-arg rejection, `OldPath == NewPath` rejection, existence guard, summary output, op written to log, state updated correctly.
- Integration tests for `arm scope-delete`: empty-arg rejection, existence guard, warning printed for non-terminal issues with empty-after-deletion scope, op written to log, state updated correctly.
- Integration test for hook auto-detection: simulated commit with rename and delete, correct ops emitted only for scope-referenced paths; initial-commit edge case skipped.

## Out of Scope

- Fuzzy or pattern-based glob removal on file deletion (deferred; breaks replay determinism).
- Directory-prefix grouping in the hook for glob scope entries (deferred; users run `arm scope-rename <prefix>` explicitly).
- `arm scope-rename` operating on live working-tree changes before commit (deferred; hook covers the commit path).
