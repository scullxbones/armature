# E2-001-T1: Abstract Issues Directory Path Resolution via Context Struct

## Problem

All trellis commands hardcode `issuesDir := repoPath + "/.issues"`. For dual-branch mode (E2), the issues directory lives in a git worktree on a separate branch. Commands need a mode-aware path resolution mechanism.

## Decision: Git Config for Mode Detection

Mode is stored in `git config trellis.mode` rather than in a file inside `.issues/`. This avoids a chicken-and-egg problem: you can't read `.issues/config.json` to find out where `.issues/` is.

- Unset or `"single-branch"` → default behavior
- `"dual-branch"` → worktree-based path resolution

## Design

### New: `internal/config/context.go`

```go
type Context struct {
    RepoPath  string // resolved repo root (default ".")
    IssuesDir string // ".issues" (single-branch) or worktree path (dual-branch)
    Mode      string // "single-branch" or "dual-branch"
    Config    Config // loaded from IssuesDir/config.json
}

func ResolveContext(repoPath string) (*Context, error)
```

**Resolution steps:**
1. Run `git config trellis.mode` — default to `"single-branch"` if exit code 1
2. Single-branch: `IssuesDir = repoPath + "/.issues"`
3. Dual-branch: stub returning `errors.New("dual-branch not yet implemented")` (T2 fills this in)
4. Load `Config` from `IssuesDir + "/config.json"`
5. Return `*Context`

### Root Command: `PersistentPreRunE`

A `PersistentPreRunE` on the root command calls `ResolveContext` and stores the result in a package-level `var appCtx *config.Context`.

**Excluded commands** (don't need context): `init`, `version`, `worker-init`, `completion`. These set their own `PersistentPreRunE` that returns nil to skip the root hook (Cobra runs the most-specific `PersistentPreRunE`).

### Command Changes

Each command that currently computes `issuesDir` switches to reading `appCtx.IssuesDir`:

| File | Current pattern | New pattern |
|------|----------------|-------------|
| `claim.go` | `issuesDir := repoPath + "/.issues"` | `appCtx.IssuesDir` |
| `create.go` | same | same |
| `transition.go` | same | same |
| `note.go` | same | same |
| `heartbeat.go` | same | same |
| `ready.go` | same | same |
| `materialize.go` | same | same |
| `link.go` | same | same |
| `decision.go` | same | same |
| `reopen.go` | same | same |
| `merged.go` | same | same |
| `validate.go` | same | same |
| `render_context.go` | same (if exists) | same |
| `decompose_*.go` | same | same |

### `resolveWorkerAndLog` Update

```go
// Before:
logPath := fmt.Sprintf("%s/.issues/ops/%s.log", repoPath, workerID)

// After:
logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, workerID)
```

### `--repo` Flag

The `--repo` flag remains on the root command (moved from individual commands). `PersistentPreRunE` reads it before calling `ResolveContext`.

## Scope Exclusions

- No worktree creation or management (E2-001-T2)
- No dual-branch path resolution implementation (stubbed with error)
- Config file stays at `.issues/config.json` — only mode detection uses git config
- No changes to `Materialize()` signature (E2-001-T4)

## Testing Strategy

- **Unit test** `ResolveContext` with a temp git repo: verify single-branch returns correct path, dual-branch returns error stub
- **Integration tests**: existing command tests continue to pass (single-branch is default)
- **Regression**: `make test` must pass with zero changes to test files (behavior-preserving refactor)
