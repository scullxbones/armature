# E2-001-T1: Context Struct Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace hardcoded `.issues` path resolution with a `Context` struct resolved once via `PersistentPreRunE`, using `git config trellis.mode` for mode detection.

**Architecture:** New `Context` struct in `internal/config/` resolved by root command's `PersistentPreRunE`. Excluded commands (`init`, `version`, `worker-init`, `decompose-context`) define their own no-op `PersistentPreRunE`. All other commands read `appCtx.IssuesDir` and `appCtx.RepoPath` instead of computing paths locally.

**Tech Stack:** Go, Cobra (PersistentPreRunE), git config

---

## Chunk 1: Context struct + tests

### Task 1: Create `ResolveContext` with tests

**Files:**
- Create: `internal/config/context.go`
- Create: `internal/config/context_test.go`

- [ ] **Step 1: Write failing test for single-branch resolution**

In `internal/config/context_test.go`:

```go
package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestResolveContext_SingleBranch_Default(t *testing.T) {
	repo := initTestRepo(t)

	// Create .armature/config.json so LoadConfig works
	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", ctx.Mode)
	assert.Equal(t, filepath.Join(repo, ".issues"), ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
	assert.Equal(t, "go", ctx.Config.ProjectType)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestResolveContext_SingleBranch_Default -v`
Expected: FAIL — `ResolveContext` not defined

- [ ] **Step 3: Implement `ResolveContext`**

In `internal/config/context.go`:

```go
package config

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context holds resolved paths and config for the current trellis session.
type Context struct {
	RepoPath  string // resolved repo root
	IssuesDir string // path to issues directory
	Mode      string // "single-branch" or "dual-branch"
	Config    Config // loaded from IssuesDir/config.json
}

// ResolveContext reads git config for mode and resolves the issues directory path.
func ResolveContext(repoPath string) (*Context, error) {
	mode, err := readGitConfigMode(repoPath)
	if err != nil {
		return nil, fmt.Errorf("read trellis mode: %w", err)
	}

	var issuesDir string
	switch mode {
	case "single-branch":
		issuesDir = filepath.Join(repoPath, ".issues")
	case "dual-branch":
		return nil, errors.New("dual-branch mode not yet implemented")
	default:
		return nil, fmt.Errorf("unknown trellis mode: %q", mode)
	}

	cfg, err := LoadConfig(filepath.Join(issuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &Context{
		RepoPath:  repoPath,
		IssuesDir: issuesDir,
		Mode:      mode,
		Config:    cfg,
	}, nil
}

// readGitConfigMode reads trellis.mode from git config. Returns "single-branch" if unset.
func readGitConfigMode(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "config", "trellis.mode")
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 means key not set — default to single-branch
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "single-branch", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestResolveContext_SingleBranch_Default -v`
Expected: PASS

- [ ] **Step 5: Write test for dual-branch stub error**

Add to `internal/config/context_test.go`:

```go
func TestResolveContext_DualBranch_ReturnsError(t *testing.T) {
	repo := initTestRepo(t)

	// Set dual-branch mode in git config
	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "dual-branch")
	require.NoError(t, cmd.Run())

	_, err := ResolveContext(repo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestResolveContext_DualBranch -v`
Expected: PASS (the stub error is already in the implementation)

- [ ] **Step 7: Write test for explicit single-branch git config**

Add to `internal/config/context_test.go`:

```go
func TestResolveContext_SingleBranch_Explicit(t *testing.T) {
	repo := initTestRepo(t)

	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "single-branch")
	require.NoError(t, cmd.Run())

	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), DefaultConfig("go")))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "single-branch", ctx.Mode)
}
```

- [ ] **Step 8: Run all context tests**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/config/context.go internal/config/context_test.go
git commit -m "feat: add ResolveContext for mode-aware issues dir resolution"
```

---

## Chunk 2: Atomic --repo migration + PersistentPreRunE + all command updates

> **IMPORTANT:** The `--repo` persistent flag on root and removal of local `--repo` flags from all commands must happen atomically. Cobra does not allow a local flag with the same name as an inherited persistent flag. All changes in this chunk must be applied together before running tests.

### Task 2: Root command + helpers + excluded commands + all command updates

**Files:**
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/helpers.go`
- Modify: `cmd/trellis/version.go`
- Modify: `cmd/trellis/init.go` (excluded — keeps local `--repo`, adds no-op PreRunE)
- Modify: `cmd/trellis/worker_init.go` (excluded — keeps local `--repo`, adds no-op PreRunE)
- Modify: `cmd/trellis/decompose.go` (decompose-context excluded; apply/revert updated)
- Modify: `cmd/trellis/claim.go`
- Modify: `cmd/trellis/ready.go`
- Modify: `cmd/trellis/materialize.go`
- Modify: `cmd/trellis/validate.go`
- Modify: `cmd/trellis/transition.go`
- Modify: `cmd/trellis/render_context.go`
- Modify: `cmd/trellis/create.go`
- Modify: `cmd/trellis/note.go`
- Modify: `cmd/trellis/heartbeat.go`
- Modify: `cmd/trellis/link.go`
- Modify: `cmd/trellis/decision.go`
- Modify: `cmd/trellis/reopen.go`
- Modify: `cmd/trellis/merged.go`

- [ ] **Step 1: Update `main.go` — add `appCtx`, `PersistentPreRunE`, persistent `--repo`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/scullxbones/armature/internal/config"
	"github.com/spf13/cobra"
)

var Version = "dev"

var appCtx *config.Context

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "arm",
		Short: "Armature — git-native work orchestration",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			repoPath, _ := cmd.Flags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}
			ctx, err := config.ResolveContext(repoPath)
			if err != nil {
				return err
			}
			appCtx = ctx
			return nil
		},
	}

	root.PersistentFlags().Bool("debug", false, "dump debug diagnostics on error")
	root.PersistentFlags().String("format", "human", "output format: human, json, agent")
	root.PersistentFlags().String("repo", "", "repository path (default: current directory)")

	// ... AddCommand calls unchanged ...
	return root
}
```

- [ ] **Step 2: Update `helpers.go` — change `resolveWorkerAndLog` signature**

```go
func resolveWorkerAndLog() (string, string, error) {
	workerID, err := worker.GetWorkerID(appCtx.RepoPath)
	if err != nil {
		return "", "", fmt.Errorf("worker not initialized: %w", err)
	}
	logPath := fmt.Sprintf("%s/ops/%s.log", appCtx.IssuesDir, workerID)
	return workerID, logPath, nil
}
```

- [ ] **Step 3: Add no-op `PersistentPreRunE` to excluded commands**

**`version.go`** — add `PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },` to the command struct.

**`init.go`** — add same no-op `PersistentPreRunE`. Keep its local `--repo` flag (Cobra local flag shadows persistent flag of same name — `init` reads via its `&repoPath` binding which resolves the local flag).

**`worker_init.go`** — add same no-op `PersistentPreRunE`. Keep its local `--repo` flag (same shadowing behavior).

**`decompose.go`** — add no-op `PersistentPreRunE` to `newDecomposeContextCmd` only.

- [ ] **Step 4: Update all Group 2 commands (only use `resolveWorkerAndLog`)**

For each file, apply the same pattern: remove `repoPath` var, remove `if repoPath == ""` block, change `resolveWorkerAndLog(repoPath)` to `resolveWorkerAndLog()`, remove `cmd.Flags().StringVar(&repoPath, "repo", ...)` line.

Files: `create.go`, `note.go`, `heartbeat.go`, `link.go`, `decision.go`, `reopen.go`, `merged.go`

- [ ] **Step 5: Update all Group 1 commands (direct `issuesDir` construction)**

For each file: remove `repoPath` var, remove `if repoPath == ""` block, replace `issuesDir := repoPath + "/.issues"` with `issuesDir := appCtx.IssuesDir`, remove `cmd.Flags().StringVar(&repoPath, "repo", ...)` line.

**`claim.go`** — also change `resolveWorkerAndLog(repoPath)` to `resolveWorkerAndLog()`.

**`transition.go`** — also change `resolveWorkerAndLog(repoPath)` to `resolveWorkerAndLog()`. Replace `config.LoadConfig` conditional with `appCtx.Config`:

```go
// Before:
cfg, cfgErr := config.LoadConfig(filepath.Join(issuesDir, "config.json"))
if cfgErr == nil {

// After:
cfg := appCtx.Config
{
```

Note: this changes semantics from "skip hooks if config missing" to "hooks always run." This is correct because `ResolveContext` already validated config; if it was missing, we wouldn't reach this code.

**`render_context.go`** — straightforward replacement.

**`ready.go`** — straightforward replacement.

**`materialize.go`** — straightforward replacement.

**`validate.go`** — straightforward replacement.

**`decompose.go` (apply + revert only)** — replace `issuesDir` and change `worker.GetWorkerID(repoPath)` to `worker.GetWorkerID(appCtx.RepoPath)`. `decompose-context` is unchanged.

- [ ] **Step 6: Run all tests**

Run: `go test ./cmd/trellis/ -v`
Expected: All PASS

- [ ] **Step 7: Run full test suite**

Run: `make test`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add cmd/trellis/
git commit -m "feat: add PersistentPreRunE context resolution, migrate --repo to root, update all commands to use appCtx"
```

---

## Chunk 3: Final verification

### Task 3: Full regression + build

- [ ] **Step 1: Run full test suite**

Run: `make test`
Expected: All PASS

- [ ] **Step 2: Build CLI**

Run: `make build`
Expected: Binary builds successfully

- [ ] **Step 3: Smoke test with built binary**

```bash
cd $(mktemp -d)
git init && git commit --allow-empty -m init
/path/to/arm init
/path/to/arm create --title "Smoke test" --type task --id SMOKE-1
/path/to/arm ready
/path/to/arm materialize
/path/to/arm version
```

Expected: All commands work. `version` works even outside a trellis repo.

- [ ] **Step 4: Verify dual-branch mode returns clear error**

```bash
git config trellis.mode dual-branch
/path/to/arm ready
```

Expected: Error message containing "not yet implemented"

- [ ] **Step 5: Final commit (if any fixups needed)**

```bash
git add -A && git commit -m "fix: address smoke test issues"
```

Only if Step 3 or 4 revealed issues.
