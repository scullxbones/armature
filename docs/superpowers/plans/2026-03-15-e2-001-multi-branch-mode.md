# E2-001: Implement Multi-Branch Mode — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `trellis.mode=dual-branch` functional end-to-end: init creates an orphan `_trellis` branch + `.trellis/` worktree, context resolution reads the worktree path, and all materialize calls derive `singleBranch` from the mode rather than hardcoding `true`.

**Architecture:** Git config `trellis.mode` already drives mode selection; the Context struct (`internal/config/context.go`) already resolves `IssuesDir` for single-branch. This plan implements the dual-branch case: create an orphan `_trellis` branch at init, mount it as a linked worktree at `.trellis/`, and store the worktree path in `trellis.ops-worktree-path` git config so `ResolveContext` can find it. All hardcoded `singleBranch=true` calls become `appCtx.Mode == "single-branch"`.

**Tech Stack:** Go, `os/exec` for git commands, testify

---

## Status Note

T1 (Context struct) and T3 (route reads/writes through appCtx) are **already implemented**. The `internal/config/context.go` file has `Context`, `ResolveContext`, and `readGitConfigMode`. `cmd/trellis/helpers.go` already uses `appCtx.IssuesDir`. This plan covers T2, T4, T5, and T6.

**Test pattern in this repo** (`cmd/trellis/main_test.go`): all integration tests use `newRootCmd()`, `cmd.SetOut(buf)`, `cmd.SetArgs(...)`, `cmd.Execute()`. There is no `runCmd` helper — do not invent one. The `run(t, dir, "git", ...)` helper runs external commands. `initTempRepo(t)` creates a temp git repo without an initial commit.

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `internal/git/git_test.go` | Tests for new git methods |
| Modify | `internal/git/git.go` | Add `CreateOrphanBranch`, `AddWorktree`, `ReadGitConfig`, `SetGitConfig` |
| Modify | `internal/config/context.go` | Implement dual-branch case using `trellis.ops-worktree-path` |
| Modify | `internal/config/context_test.go` | Add dual-branch resolution tests (replaces "returns error" test) |
| Modify | `cmd/trellis/init.go` | Add `--dual-branch` flag, create orphan branch + worktree |
| Modify | `cmd/trellis/claim.go` | `Materialize(issuesDir, appCtx.Mode == "single-branch")` |
| Modify | `cmd/trellis/materialize.go` | Same |
| Modify | `cmd/trellis/ready.go` | Same |
| Modify | `cmd/trellis/render_context.go` | Same |
| Modify | `cmd/trellis/main_test.go` | Integration test for `init --dual-branch` |

---

## Chunk 1: Git Worktree Methods (T2)

### Task 1: Add git.Client methods for orphan branches and worktrees

**Files:**
- Modify: `internal/git/git.go`
- Create: `internal/git/git_test.go`

- [ ] **Step 1: Write failing tests for new git methods**

Create `internal/git/git_test.go`:

```go
package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("init")
	gitRun("config", "user.email", "test@test.com")
	gitRun("config", "user.name", "Test")
	gitRun("config", "commit.gpgsign", "false")
	gitRun("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestCreateOrphanBranch(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	err := c.CreateOrphanBranch("_trellis")
	require.NoError(t, err)

	// Verify branch exists
	cmd := exec.Command("git", "-C", repo, "branch", "--list", "_trellis")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "_trellis")

	// Verify we are still on the original branch (not _trellis)
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_trellis\n", string(branchOut))
}

func TestCreateOrphanBranch_Idempotent(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_trellis"))
	// Second call should not error; branch already exists so it returns nil immediately
	err := c.CreateOrphanBranch("_trellis")
	assert.NoError(t, err)

	// Still on original branch
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	assert.NotEqual(t, "_trellis\n", string(branchOut))
}

func TestAddWorktree(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_trellis"))

	worktreePath := filepath.Join(repo, ".trellis")
	err := c.AddWorktree("_trellis", worktreePath)
	require.NoError(t, err)

	// Verify worktree directory exists
	info, err := os.Stat(worktreePath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSetAndReadGitConfig(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	err := c.SetGitConfig("trellis.ops-worktree-path", "/some/path")
	require.NoError(t, err)

	val, err := c.ReadGitConfig("trellis.ops-worktree-path")
	require.NoError(t, err)
	assert.Equal(t, "/some/path", val)
}

func TestReadGitConfig_Unset(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	_, err := c.ReadGitConfig("trellis.nonexistent")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/brian/development/trellis && go test ./internal/git/... -v
```
Expected: FAIL — `(*Client).CreateOrphanBranch undefined` (methods don't exist yet)

- [ ] **Step 3: Add methods to `internal/git/git.go`**

Add `"os"`, `"path/filepath"`, `"strings"` to the imports in `internal/git/git.go`, then append the following methods:

```go
// CreateOrphanBranch creates an orphan branch (no parent commits) with a single empty commit.
// If the branch already exists, this is a no-op. Always returns to the original branch.
func (c *Client) CreateOrphanBranch(branch string) error {
	// Check if branch already exists — idempotent fast-path
	check := exec.Command("git", "-C", c.repoPath, "rev-parse", "--verify", branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Capture current branch name so we can return to it explicitly
	headCmd := exec.Command("git", "-C", c.repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	priorBranch := strings.TrimSpace(string(headOut))

	// Create orphan branch and make an empty initial commit
	cmds := [][]string{
		{"checkout", "--orphan", branch},
		{"rm", "-rf", "--quiet", "."},
		{"commit", "--allow-empty", "-m", "chore: init trellis issues branch"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", append([]string{"-C", c.repoPath}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w\n%s", args, err, out)
		}
	}

	// Return to the original branch by name (not `checkout -` which may fail on fresh repos)
	restore := exec.Command("git", "-C", c.repoPath, "checkout", priorBranch)
	if out, err := restore.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", priorBranch, err, out)
	}
	return nil
}

// AddWorktree adds a linked worktree for an existing branch at the given path.
// If the worktree already exists at that path (has a .git file), this is a no-op.
func (c *Client) AddWorktree(branch, path string) error {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return nil // already a worktree
	}
	cmd := exec.Command("git", "-C", c.repoPath, "worktree", "add", path, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

// SetGitConfig sets a local git config key to value.
func (c *Client) SetGitConfig(key, value string) error {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "--local", key, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config set %s: %w\n%s", key, err, out)
	}
	return nil
}

// ReadGitConfig reads a local git config key. Returns error if unset.
func (c *Client) ReadGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "--local", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/brian/development/trellis && go test ./internal/git/... -v
```
Expected: PASS all tests

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add orphan branch, worktree, and git config helpers"
```

---

## Chunk 2: Derive singleBranch from Mode (T6)

### Task 2: Replace hardcoded `singleBranch=true` with `appCtx.Mode == "single-branch"`

**Files:**
- Modify: `cmd/trellis/claim.go:23`
- Modify: `cmd/trellis/materialize.go:15`
- Modify: `cmd/trellis/ready.go:19`
- Modify: `cmd/trellis/render_context.go:27`

- [ ] **Step 1: Write a regression test that confirms single-branch behavior is unchanged**

In `cmd/trellis/main_test.go`, add:

```go
func TestMaterialize_SingleBranchMode_AfterModeRefactor(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init repo
	cmd1 := newRootCmd()
	cmd1.SetOut(new(bytes.Buffer))
	cmd1.SetArgs([]string{"init", "--repo", repo})
	require.NoError(t, cmd1.Execute())

	// Materialize should still work
	buf := new(bytes.Buffer)
	cmd2 := newRootCmd()
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"materialize", "--repo", repo})
	require.NoError(t, cmd2.Execute())
	assert.Contains(t, buf.String(), "Materialized")
}
```

- [ ] **Step 2: Run test to confirm it passes (baseline)**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestMaterialize_SingleBranchMode_AfterModeRefactor -v
```
Expected: PASS (confirms existing behavior before the change)

- [ ] **Step 3: Replace hardcoded `true` in all four call sites**

In `cmd/trellis/claim.go:23`, change:
```go
if _, err := materialize.Materialize(issuesDir, true); err != nil {
```
to:
```go
if _, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch"); err != nil {
```

In `cmd/trellis/materialize.go:15`, change:
```go
result, err := materialize.Materialize(appCtx.IssuesDir, true)
```
to:
```go
result, err := materialize.Materialize(appCtx.IssuesDir, appCtx.Mode == "single-branch")
```

In `cmd/trellis/ready.go:19`, change:
```go
if _, err := materialize.Materialize(issuesDir, true); err != nil {
```
to:
```go
if _, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch"); err != nil {
```

In `cmd/trellis/render_context.go:27`, change:
```go
_, err := materialize.Materialize(issuesDir, true)
```
to:
```go
_, err := materialize.Materialize(issuesDir, appCtx.Mode == "single-branch")
```

- [ ] **Step 4: Run all tests to verify single-branch behavior is unchanged**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -v
```
Expected: PASS — `appCtx.Mode == "single-branch"` evaluates to `true` in single-branch mode, behavior identical

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/claim.go cmd/trellis/materialize.go cmd/trellis/ready.go cmd/trellis/render_context.go cmd/trellis/main_test.go
git commit -m "feat(mode): derive singleBranch flag from appCtx.Mode instead of hardcoding true"
```

---

## Chunk 3: Init Dual-Branch Flag + Context Resolution (T5 + T4)

### Task 3: Implement dual-branch context resolution

**Files:**
- Modify: `internal/config/context.go:26-34` (dual-branch case)
- Modify: `internal/config/context_test.go` (replace "returns error" test with real tests)

- [ ] **Step 1: Write failing tests for dual-branch context resolution**

In `internal/config/context_test.go`, replace `TestResolveContext_DualBranch_ReturnsError` with two new tests:

```go
func TestResolveContext_DualBranch(t *testing.T) {
	repo := initTestRepo(t)

	// Simulate a dual-branch setup: create the worktree dir with .issues/ inside
	worktreePath := filepath.Join(repo, ".trellis")
	issuesDir := filepath.Join(worktreePath, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	cfg.Mode = "dual-branch"
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	// Set git config keys
	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "trellis.mode", "dual-branch")
	runGit("config", "trellis.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "dual-branch", ctx.Mode)
	assert.Equal(t, issuesDir, ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
}

func TestResolveContext_DualBranch_MissingWorktreePath(t *testing.T) {
	repo := initTestRepo(t)

	// Set dual-branch mode but do NOT set ops-worktree-path
	cmd := exec.Command("git", "-C", repo, "config", "trellis.mode", "dual-branch")
	require.NoError(t, cmd.Run())

	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trellis.ops-worktree-path")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/brian/development/trellis && go test ./internal/config/... -run TestResolveContext_DualBranch -v
```
Expected: FAIL — `TestResolveContext_DualBranch` fails with "not yet implemented"; `TestResolveContext_DualBranch_MissingWorktreePath` fails because the error message doesn't mention `trellis.ops-worktree-path`

- [ ] **Step 3: Implement dual-branch case in `ResolveContext`**

In `internal/config/context.go`, replace:
```go
case "dual-branch":
    return nil, errors.New("dual-branch mode not yet implemented")
```
with:
```go
case "dual-branch":
    worktreePath, err := readGitConfig(repoPath, "trellis.ops-worktree-path")
    if err != nil {
        return nil, fmt.Errorf("dual-branch mode requires trellis.ops-worktree-path to be set: %w", err)
    }
    issuesDir = filepath.Join(worktreePath, ".issues")
```

Add a new helper function in `context.go`. Note: `readGitConfigMode` already uses `os/exec` directly for the same reason (can't import the `git` package without a cycle risk), so this is intentional duplication at the config/context boundary:

```go
// readGitConfig reads a single local git config key. Returns error if unset.
// Note: intentionally does not use git.Client to avoid circular imports.
func readGitConfig(repoPath, key string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "config", "--local", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}
```

Add `"strings"` to imports in `context.go` if not already present. The `errors` import stays because `errors.As` is still used in `readGitConfigMode`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/brian/development/trellis && go test ./internal/config/... -v
```
Expected: PASS all tests

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/config/context.go internal/config/context_test.go
git commit -m "feat(config): implement dual-branch context resolution via trellis.ops-worktree-path"
```

---

### Task 4: Add `--dual-branch` flag to `trls init`

**Files:**
- Modify: `cmd/trellis/init.go`
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing integration test for `init --dual-branch`**

In `cmd/trellis/main_test.go`, add:

```go
func TestInitCommand_DualBranch(t *testing.T) {
	repo := initTempRepo(t)
	// An initial commit is required so CreateOrphanBranch can record current branch
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"init", "--dual-branch", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dual-branch")

	// Worktree should exist at .trellis/
	assert.DirExists(t, filepath.Join(repo, ".trellis"))

	// .issues/ inside worktree should have config.json with dual-branch mode
	cfgPath := filepath.Join(repo, ".trellis", ".issues", "config.json")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "dual-branch")

	// Git config should have mode set
	modeCmd := exec.Command("git", "-C", repo, "config", "trellis.mode")
	modeOut, err := modeCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "dual-branch\n", string(modeOut))

	// Git config should have worktree path set
	wtCmd := exec.Command("git", "-C", repo, "config", "trellis.ops-worktree-path")
	wtOut, err := wtCmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(wtOut), ".trellis")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestInitCommand_DualBranch -v
```
Expected: FAIL — `--dual-branch` flag not recognized

- [ ] **Step 3: Implement `--dual-branch` in `cmd/trellis/init.go`**

Replace the entire file content with:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/trellis/internal/config"
	"github.com/scullxbones/trellis/internal/git"
	"github.com/scullxbones/trellis/internal/ops"
	"github.com/scullxbones/trellis/internal/worker"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var repoPath string
	var dualBranch bool

	cmd := &cobra.Command{
		Use:               "init",
		Short:             "Initialize Trellis in the current repository",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoPath == "" {
				repoPath = "."
			}
			return runInit(cmd, repoPath, dualBranch)
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "repository path (default: current directory)")
	cmd.Flags().BoolVar(&dualBranch, "dual-branch", false, "initialize in dual-branch mode (issues stored on separate _trellis branch)")
	return cmd
}

func runInit(cmd *cobra.Command, repoPath string, dualBranch bool) error {
	gitClient := git.New(repoPath)

	var issuesDir string
	if dualBranch {
		// Create orphan branch _trellis (idempotent)
		if err := gitClient.CreateOrphanBranch("_trellis"); err != nil {
			return fmt.Errorf("create _trellis branch: %w", err)
		}

		// Create .trellis/ worktree (idempotent)
		worktreePath := filepath.Join(repoPath, ".trellis")
		if err := gitClient.AddWorktree("_trellis", worktreePath); err != nil {
			return fmt.Errorf("add .trellis worktree: %w", err)
		}

		// Set git config keys
		if err := gitClient.SetGitConfig("trellis.mode", "dual-branch"); err != nil {
			return fmt.Errorf("set trellis.mode: %w", err)
		}
		if err := gitClient.SetGitConfig("trellis.ops-worktree-path", worktreePath); err != nil {
			return fmt.Errorf("set trellis.ops-worktree-path: %w", err)
		}

		issuesDir = filepath.Join(worktreePath, ".issues")
	} else {
		issuesDir = filepath.Join(repoPath, ".issues")
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(issuesDir, "ops"),
		filepath.Join(issuesDir, "state"),
		filepath.Join(issuesDir, "state", "issues"),
		filepath.Join(issuesDir, "templates"),
		filepath.Join(issuesDir, "hooks"),
		filepath.Join(issuesDir, "review"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write SCHEMA file
	schemaPath := filepath.Join(issuesDir, "ops", "SCHEMA")
	if err := os.WriteFile(schemaPath, []byte(ops.GenerateSchema()), 0644); err != nil {
		return fmt.Errorf("write SCHEMA: %w", err)
	}

	// Detect project type and write config
	configPath := filepath.Join(issuesDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		projectType := config.DetectProjectType(repoPath)
		cfg := config.DefaultConfig(projectType)
		if dualBranch {
			cfg.Mode = "dual-branch"
		}
		if err := config.WriteConfig(configPath, cfg); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	// Init worker if not already configured
	if ok, _ := worker.CheckWorkerID(repoPath); !ok {
		if _, err := worker.InitWorker(repoPath); err != nil {
			return fmt.Errorf("init worker: %w", err)
		}
	}

	mode := "single-branch"
	if dualBranch {
		mode = "dual-branch"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Initialized Trellis in %s mode at %s\n", mode, issuesDir)
	return nil
}
```

- [ ] **Step 4: Run all tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS — new dual-branch init test passes, all existing tests continue to pass

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/init.go cmd/trellis/main_test.go
git commit -m "feat(init): add --dual-branch flag to create _trellis orphan branch and .trellis worktree"
```

---

## Chunk 4: End-to-End Verification

### Task 5: Manual smoke test of dual-branch mode

**Files:** None (read-only verification)

- [ ] **Step 1: Build the binary**

```bash
cd /home/brian/development/trellis && go build -o /tmp/trls ./cmd/trellis
```
Expected: Clean build, no errors

- [ ] **Step 2: Smoke test in a temp directory**

```bash
TMPDIR=$(mktemp -d)
cd "$TMPDIR"
git init
git config user.email "test@test.com"
git config user.name "Test"
git config commit.gpgsign false
git commit --allow-empty -m "init"
/tmp/trls init --dual-branch
# Should print: "Initialized Trellis in dual-branch mode at .../\.trellis/.issues"
ls .trellis/.issues/  # Should show ops/ state/ config.json etc
git branch            # Should show _trellis branch
git config trellis.mode  # Should print "dual-branch"
git config trellis.ops-worktree-path  # Should print path to .trellis/
```
Expected: All steps succeed

- [ ] **Step 3: Verify materialize works in dual-branch mode**

Continuing in the same temp dir:

```bash
/tmp/trls worker-init
/tmp/trls materialize
# Should print: "Materialized 0 issues from 0 ops"
```
Expected: Success (ops dir is empty but path resolves correctly)

- [ ] **Step 4: Run full test suite**

```bash
cd /home/brian/development/trellis && go test ./...
```
Expected: All PASS

- [ ] **Step 5: Commit if any fixes were needed**

If smoke test revealed any issues, fix them and commit:
```bash
cd /home/brian/development/trellis && git add -p
git commit -m "fix(dual-branch): fix issues found during smoke test"
```

---

## Definition of Done Checklist

- [ ] `trls init --dual-branch` creates `_trellis` orphan branch and `.trellis/` worktree
- [ ] `trellis.mode=dual-branch` and `trellis.ops-worktree-path` are set in git config
- [ ] `ResolveContext` resolves `IssuesDir` to `.trellis/.issues/` in dual-branch mode
- [ ] `ResolveContext` returns a clear error when `trellis.ops-worktree-path` is absent
- [ ] `CreateOrphanBranch` always returns to the original branch (captures it by name, not `checkout -`)
- [ ] All `Materialize()` calls use `appCtx.Mode == "single-branch"` (not hardcoded `true`)
- [ ] All existing single-branch tests pass
- [ ] `go test ./...` passes clean
