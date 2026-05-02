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

#### `scope-rename`

**Payload fields:** `OldPath string`, `NewPath string`

The materializer applies `strings.ReplaceAll(entry, OldPath, NewPath)` to every scope entry across all issues. `OldPath` and `NewPath` are treated as substrings, not necessarily full paths, so a single op correctly handles both exact paths and glob patterns:

- Exact entry `cmd/trellis/main.go` with `OldPath=cmd/trellis/main.go` → exact replacement
- Glob entry `cmd/trellis/*.go` with `OldPath=cmd/trellis` → `cmd/armature/*.go`

A directory rename can therefore be expressed as one op using the directory prefix as old/new, updating all scope entries (both exact and glob) in one pass.

**Idempotency:** On replay, a second application of the same op finds nothing matching `OldPath` (already rewritten) and is a no-op. The op never creates scope entries — it only transforms existing ones.

**Existence guard:** Before emitting the op (from command or hook), current materialized state is checked. If `OldPath` does not appear as a substring in any issue's scope entries, no op is written.

#### `scope-delete`

**Payload fields:** `DeletedPath string`

The materializer removes any scope entry where `entry == DeletedPath` (exact string match only). Glob entries are not removed; a glob covering deleted files will surface via the W10 phantom scope check on the next `arm validate`.

**Rationale for exact-only:** Removing a glob because one of its matches disappeared requires filesystem evaluation at op-apply time, which breaks replay determinism. Glob cleanup is left to the user via `arm amend`.

**Warning on empty scope:** After applying the removal, if any issue whose status is not `merged`, `done`, or `cancelled` has an empty scope as a result, `ApplyOp` returns a warning. The E6 validation check will independently flag the empty scope on the next `arm validate` run.

### Materializer Changes (`internal/materialize/engine.go`)

`ApplyOp` gains two new cases:

```go
case ops.OpScopeRename:
    return s.applyScopeRename(op)
case ops.OpScopeDelete:
    return s.applyScopeDelete(op)
```

`applyScopeRename` iterates all issues, applies substring replacement to each scope entry, records `Updated` timestamp on modified issues.

`applyScopeDelete` iterates all issues, removes exact-matching entries, records `Updated` on modified issues, returns a warning string (not an error) if any active issue's scope becomes empty.

### New Commands

#### `arm scope-rename <old-path> <new-path>`

1. Load current materialized state.
2. Check whether `old-path` appears as a substring in any issue scope. If not, print a warning (`no scope entries reference <old-path>`) and exit 0 without writing an op.
3. Print a summary of affected issues (count and IDs).
4. Emit `scope-rename` op.
5. Rematerialize and write updated state.

#### `arm scope-delete <path>`

1. Load current materialized state.
2. Check whether `path` appears as an exact entry in any issue scope. If not, warn and exit 0.
3. Print affected issues. Warn explicitly if any active task will have an empty scope after deletion.
4. Emit `scope-delete` op.
5. Rematerialize and write updated state.

Both commands are plumbed through the existing op-write path consistent with `arm amend`, `arm note`, etc.

### Git Hook Auto-Detection

The post-commit hook is extended to detect file renames and deletions from the most recent commit:

```
git diff --diff-filter=R --name-status HEAD~1 HEAD  # renames
git diff --diff-filter=D --name-status HEAD~1 HEAD  # deletions
```

**Renames:** Individual file renames are grouped by common directory-prefix change. For each distinct prefix pair `(oldPrefix, newPrefix)`, one `scope-rename` op is emitted — provided the existence guard passes (at least one scope entry references `oldPrefix`). This grouping ensures glob entries are updated by a single directory-level op rather than N per-file ops.

**Deletions:** For each deleted file, one `scope-delete` op is emitted — provided at least one issue scope contains the exact path. Files not referenced in any scope are ignored.

**Edge case:** If the commit is the initial commit (no `HEAD~1`), this step is skipped. The hook is best-effort; users who rename or delete files without committing should use the explicit commands directly.

### W10 Phantom Scope Narrowing

`checkW10PhantomScope` in `internal/validate/validate.go` is updated to skip issues whose status is `merged`, `done`, or `cancelled`. Terminal issues had valid scope when work was completed; phantom entries there are noise.

The check retains its original purpose: flag typos and invalid globs on active work items.

### Migration

Existing stale scope entries (e.g. `cmd/trellis/` paths remaining from the Trellis → Armature rename) are resolved by running `arm scope-rename` for each stale prefix. No special migration command is needed — the new ops are sufficient and will be recorded in the event log like any other op.

## Testing

- Unit tests for `applyScopeRename`: exact path replacement, glob pattern replacement, no-op when `OldPath` absent, idempotency on double-apply.
- Unit tests for `applyScopeDelete`: exact removal, glob entries left intact, warning when active task scope empties.
- Unit tests for `checkW10PhantomScope`: terminal issues skipped, active issues still checked.
- Integration tests for `arm scope-rename` and `arm scope-delete` commands: existence guard, summary output, op written to log, state updated correctly.
- Integration test for hook auto-detection: simulated commit with rename and delete, correct ops emitted.

## Out of Scope

- Fuzzy or pattern-based glob removal on file deletion (deferred; breaks replay determinism).
- `arm scope-rename` operating on live working-tree changes before commit (deferred; hook covers the commit path).
