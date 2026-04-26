# E2-001-T1: Abstract Issues Directory Path Resolution via Context Struct

## Problem

All trellis commands hardcode `issuesDir := repoPath + "/.issues"`. For dual-branch mode (E2), the issues directory lives in a git worktree on a separate branch. Commands need a mode-aware path resolution mechanism.

## Decision: Git Config for Mode Detection

Mode is stored in `git config trellis.mode` rather than in a file inside `.armature/`. This avoids a chicken-and-egg problem: you can't read `.armature/config.json` to find out where `.armature/` is.

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

**Excluded commands** (don't need context, or run before `.armature/` exists): `init`, `version`, `worker-init`, `decompose-context`. These set their own `PersistentPreRunE` that returns nil to skip the root hook (Cobra runs the most-specific `PersistentPreRunE`).

- `init` and `worker-init` are excluded because they run before `.armature/` exists — `ResolveContext` step 4 (load config) would fail.
- `decompose-context` only parses a plan file and has no repo interaction.
- `version` needs no project state.

### Command Changes

**Group 1: Commands that directly construct `issuesDir`** — replace with `appCtx.IssuesDir`:

| File | Notes |
|------|-------|
| `claim.go` | Also uses `resolveWorkerAndLog` |
| `ready.go` | |
| `materialize.go` | |
| `validate.go` | |
| `transition.go` | Also uses `resolveWorkerAndLog` |
| `render_context.go` | |
| `decompose.go` | Apply and revert subcommands only |

**Group 2: Commands that only use `resolveWorkerAndLog`** — no direct `issuesDir` change needed, updated via helper:

| File |
|------|
| `create.go` |
| `note.go` |
| `heartbeat.go` |
| `link.go` |
| `decision.go` |
| `reopen.go` |
| `merged.go` |

All Group 2 commands also have a local `--repo` flag and `repoPath` variable that should be removed (they'll use `appCtx.RepoPath` via the updated helper).

### `resolveWorkerAndLog` Update

Signature changes from `resolveWorkerAndLog(repoPath string)` to `resolveWorkerAndLog()` — reads both `appCtx.RepoPath` (for `worker.GetWorkerID`) and `appCtx.IssuesDir` (for log path) from the package-level context.

```go
// Before:
func resolveWorkerAndLog(repoPath string) (string, string, error) {
    workerID, err := worker.GetWorkerID(repoPath)
    logPath := fmt.Sprintf("%s/.armature/ops/%s.log", repoPath, workerID)
}

// After:
func resolveWorkerAndLog() (string, string, error) {
    workerID, err := worker.GetWorkerID(appCtx.RepoPath)
    logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, workerID)
}
```

### `--repo` Flag Migration

The `--repo` flag moves from individual commands to a persistent flag on the root command. `PersistentPreRunE` reads it before calling `ResolveContext`.

- Group 1 and Group 2 commands: remove local `--repo` flag, use `appCtx.RepoPath`
- Excluded commands (`init`, `worker-init`): keep their own local `--repo` flag since they bypass `PersistentPreRunE` and need repo path independently

## Scope Exclusions

- No worktree creation or management (E2-001-T2)
- No dual-branch path resolution implementation (stubbed with error)
- Config file stays at `.armature/config.json` — only mode detection uses git config
- No changes to `Materialize()` signature (E2-001-T4)

## Testing Strategy

- **Unit test** `ResolveContext` with a temp git repo: verify single-branch returns correct path, dual-branch returns error stub
- **Integration tests**: existing command tests continue to pass (single-branch is default)
- **Excluded commands**: verify `init`, `version`, `worker-init`, `decompose-context` work without a valid `.armature/` directory
- **Regression**: `make test` must pass (behavior-preserving refactor; test file changes allowed only if tests invoke commands that now trigger `PersistentPreRunE`)
