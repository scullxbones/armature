# E2-002: Branch Isolation for Worker Operations — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In dual-branch mode, every worker op (claim, note, heartbeat, transition) appends to a log in the `.trellis/` worktree and immediately commits that file change to the `_armature` branch, keeping issues history completely isolated from the working branch.

**Architecture:** When `appCtx.Mode == "dual-branch"`, the `IssuesDir` already points to `.trellis/.armature/`. The existing `ops.AppendOp` writes to the correct file. What's missing: after writing, git-add and git-commit the changed log file within the worktree (using `git -C .trellis add ops/... && git -C .trellis commit`). This is wrapped in a new `git.Client.CommitWorktreeOp` method and called by a new `ops.AppendAndCommit` function used by all worker commands in dual-branch mode.

**Tech Stack:** Go, os/exec for git, testify

**Prerequisite:** E2-001 must be complete. The following methods must exist on `*git.Client` before implementing this plan:
- `CreateOrphanBranch(branch string) error`
- `AddWorktree(branch, path string) error`
- `SetGitConfig(key, value string) error`

`appCtx.Mode`, `appCtx.IssuesDir`, and `appCtx.WorktreePath` must be set correctly (added in E2-001).

**Cross-plan note:** E2-002 makes a narrow change to `cmd/trellis/merged.go` (replace one `ops.AppendOp` call with `appendOp`). E2-004 later rewrites the entire file. The E2-004 rewrite already uses `appendOp` — no conflict if implemented in order.

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/git/git.go` | Add `CommitWorktreeOp(relPath, message string) error` |
| Modify | `internal/git/git_test.go` | Tests for `CommitWorktreeOp` |
| Create | `internal/ops/commit.go` | `AppendAndCommit(logPath, worktreePath string, op Op, gc GitCommitter) error` |
| Create | `internal/ops/commit_test.go` | Tests for `AppendAndCommit` |
| Modify | `internal/config/context.go` | Add `WorktreePath string` field to `Context` |
| Modify | `internal/config/context_test.go` | Assert `WorktreePath` set correctly in dual-branch mode |
| Modify | `cmd/trellis/helpers.go` | Add `appendOp(logPath string, op ops.Op) error` wrapper |
| Modify | `cmd/trellis/main_test.go` | Add `runTrls` helper + regression and isolation tests |
| Modify | `cmd/trellis/claim.go` | Use `appendOp` instead of `ops.AppendOp` |
| Modify | `cmd/trellis/note.go` | Same |
| Modify | `cmd/trellis/heartbeat.go` | Same |
| Modify | `cmd/trellis/transition.go` | Same |
| Modify | `cmd/trellis/create.go` | Same |
| Modify | `cmd/trellis/link.go` | Same |
| Modify | `cmd/trellis/decision.go` | Same |
| Modify | `cmd/trellis/reopen.go` | Same |
| Modify | `cmd/trellis/merged.go` | Same (narrow change only; E2-004 rewrites this file later) |

---

## Chunk 1: Git Worktree Commit Method

### Task 1: Add `git.Client.CommitWorktreeOp`

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/git/git_test.go` (this file already exists after E2-001):

```go
func TestCommitWorktreeOp(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	// Create orphan branch and worktree (using E2-001 methods)
	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".trellis")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	// Write a file in the worktree
	opsDir := filepath.Join(worktreePath, ".issues", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("test op\n"), 0644))

	// CommitWorktreeOp is called on a client rooted at the worktree
	wc := git.New(worktreePath)
	err := wc.CommitWorktreeOp(".armature/ops/worker-abc.log", "ops: append claim for E2-001")
	require.NoError(t, err)

	// Verify commit exists in the worktree branch
	cmd := exec.Command("git", "-C", worktreePath, "log", "--oneline", "-1")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "ops: append")
}

func TestCommitWorktreeOp_NoChanges_IsNoop(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	require.NoError(t, c.CreateOrphanBranch("_armature"))
	worktreePath := filepath.Join(repo, ".trellis")
	require.NoError(t, c.AddWorktree("_armature", worktreePath))

	// Write and commit file first
	opsDir := filepath.Join(worktreePath, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	logFile := filepath.Join(opsDir, "worker-abc.log")
	require.NoError(t, os.WriteFile(logFile, []byte("op1\n"), 0644))
	wc := git.New(worktreePath)
	require.NoError(t, wc.CommitWorktreeOp("ops/worker-abc.log", "first commit"))

	// Call again without changes — should not error
	err := wc.CommitWorktreeOp("ops/worker-abc.log", "second commit")
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/brian/development/trellis && go test ./internal/git/... -run TestCommitWorktreeOp -v
```
Expected: FAIL — `(*Client).CommitWorktreeOp undefined`

- [ ] **Step 3: Implement `CommitWorktreeOp`**

Append to `internal/git/git.go`:

```go
// CommitWorktreeOp stages and commits a single file change within a worktree.
// The receiver's repoPath must be the worktree root (not the main repo root).
// relPath is relative to the worktree root. If there is nothing to commit, this is a no-op.
func (c *Client) CommitWorktreeOp(relPath, message string) error {
	// Stage the specific file
	add := exec.Command("git", "-C", c.repoPath, "add", relPath)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w\n%s", relPath, err, out)
	}

	// Check if there is actually something staged
	diff := exec.Command("git", "-C", c.repoPath, "diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil // nothing staged, no-op
	}

	// Commit
	commit := exec.Command("git", "-C", c.repoPath, "commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./internal/git/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/git/git.go internal/git/git_test.go
git commit -m "feat(git): add CommitWorktreeOp for dual-branch op isolation"
```

---

## Chunk 2: Context WorktreePath + ops.AppendAndCommit

### Task 2: Add `WorktreePath` to Context

**Files:**
- Modify: `internal/config/context.go`
- Modify: `internal/config/context_test.go`

- [ ] **Step 1: Write failing tests**

In `internal/config/context_test.go`, add:

```go
func TestResolveContext_DualBranch_WorktreePath(t *testing.T) {
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".trellis")
	issuesDir := filepath.Join(worktreePath, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	cfg.Mode = "dual-branch"
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "trellis.mode", "dual-branch")
	runGit("config", "trellis.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, worktreePath, ctx.WorktreePath)
}

func TestResolveContext_SingleBranch_WorktreePath_Empty(t *testing.T) {
	repo := initTestRepo(t)
	issuesDir := filepath.Join(repo, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), DefaultConfig("go")))

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, "", ctx.WorktreePath)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/brian/development/trellis && go test ./internal/config/... -run TestResolveContext.*WorktreePath -v
```
Expected: FAIL — `ctx.WorktreePath` undefined

- [ ] **Step 3: Add `WorktreePath` to Context and populate it**

In `internal/config/context.go`, update the struct:

```go
type Context struct {
	RepoPath     string // resolved repo root
	IssuesDir    string // path to issues directory
	WorktreePath string // path to .trellis/ worktree; empty in single-branch mode
	Mode         string // "single-branch" or "dual-branch"
	Config       Config // loaded from IssuesDir/config.json
}
```

In `ResolveContext`, capture `worktreePath` as a local variable. In the dual-branch case it is already read (in E2-001's implementation). In the single-branch case it is `""`. Update the return:

```go
return &Context{
	RepoPath:     repoPath,
	IssuesDir:    issuesDir,
	WorktreePath: worktreePath, // "" in single-branch
	Mode:         mode,
	Config:       cfg,
}, nil
```

Declare `var worktreePath string` before the switch statement so it is in scope for the return.

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./internal/config/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/config/context.go internal/config/context_test.go
git commit -m "feat(config): add WorktreePath to Context for dual-branch mode"
```

---

### Task 3: Create `ops.AppendAndCommit`

**Files:**
- Create: `internal/ops/commit.go`
- Create: `internal/ops/commit_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/ops/commit_test.go`:

```go
package ops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCommitter struct {
	calls []struct{ relPath, message string }
	err   error
}

func (f *fakeCommitter) CommitWorktreeOp(relPath, message string) error {
	f.calls = append(f.calls, struct{ relPath, message string }{relPath, message})
	return f.err
}

func TestAppendAndCommit_SingleBranch_NoCommit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &fakeCommitter{}
	op := ops.Op{Type: ops.OpNote, TargetID: "T1", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{Msg: "hello"}}

	err := ops.AppendAndCommit(logPath, "", op, fc)
	require.NoError(t, err)

	// File should contain the op
	data, _ := os.ReadFile(logPath)
	assert.Contains(t, string(data), "note")

	// No commit was called (worktreePath is "")
	assert.Len(t, fc.calls, 0)
}

func TestAppendAndCommit_DualBranch_Commits(t *testing.T) {
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".trellis")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "abc.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &fakeCommitter{}
	op := ops.Op{Type: ops.OpClaim, TargetID: "T1", Timestamp: 1000, WorkerID: "abc-def-ghi-jkl",
		Payload: ops.Payload{TTL: 60}}

	err := ops.AppendAndCommit(logPath, worktreePath, op, fc)
	require.NoError(t, err)

	// Commit was called once
	require.Len(t, fc.calls, 1)
	assert.Contains(t, fc.calls[0].message, "claim")
	assert.Contains(t, fc.calls[0].message, "T1")
}

func TestAppendAndCommit_ShortWorkerID(t *testing.T) {
	dir := t.TempDir()
	worktreePath := filepath.Join(dir, ".trellis")
	logPath := filepath.Join(worktreePath, ".issues", "ops", "x.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0755))

	fc := &fakeCommitter{}
	// WorkerID shorter than 8 chars must not panic
	op := ops.Op{Type: ops.OpNote, TargetID: "T2", Timestamp: 1000, WorkerID: "abc",
		Payload: ops.Payload{Msg: "hi"}}

	assert.NotPanics(t, func() {
		_ = ops.AppendAndCommit(logPath, worktreePath, op, fc)
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/brian/development/trellis && go test ./internal/ops/... -run TestAppendAndCommit -v
```
Expected: FAIL — `AppendAndCommit` undefined

- [ ] **Step 3: Create `internal/ops/commit.go`**

```go
package ops

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GitCommitter is the interface for committing a file change in a worktree.
type GitCommitter interface {
	CommitWorktreeOp(relPath, message string) error
}

// AppendAndCommit appends op to logPath and, if worktreePath is non-empty,
// commits the log file to the worktree's branch via gc.
// Pass worktreePath="" (and gc=nil) for single-branch mode — commit is skipped.
func AppendAndCommit(logPath, worktreePath string, op Op, gc GitCommitter) error {
	if err := AppendOp(logPath, op); err != nil {
		return err
	}
	if worktreePath == "" {
		return nil // single-branch: no git commit needed
	}

	relPath, err := filepath.Rel(worktreePath, logPath)
	if err != nil {
		return fmt.Errorf("resolve relative log path: %w", err)
	}

	// Safely truncate WorkerID to at most 8 chars for the commit message
	workerPrefix := op.WorkerID
	if len(workerPrefix) > 8 {
		workerPrefix = workerPrefix[:8]
	}

	message := fmt.Sprintf("ops: %s %s by %s", strings.ToLower(op.Type), op.TargetID, workerPrefix)
	return gc.CommitWorktreeOp(relPath, message)
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./internal/ops/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/ops/commit.go internal/ops/commit_test.go
git commit -m "feat(ops): add AppendAndCommit for dual-branch worktree isolation"
```

---

## Chunk 3: Wire AppendAndCommit into All Worker Commands

### Task 4: Add `runTrls` helper and `appendOp` wrapper, update all worker commands

**Files:**
- Modify: `cmd/trellis/helpers.go`
- Modify: `cmd/trellis/main_test.go`
- Modify: `cmd/trellis/claim.go`, `note.go`, `heartbeat.go`, `transition.go`, `create.go`, `link.go`, `decision.go`, `reopen.go`, `merged.go`

- [ ] **Step 1: Add `runTrls` test helper to `main_test.go`**

This helper is used by all integration tests in E2-002, E2-003, and E2-004. Add it to `cmd/trellis/main_test.go`:

```go
// runTrls invokes the trellis cobra command tree with --repo injected and returns stdout + error.
func runTrls(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(errBuf)
	root.SetArgs(append(args, "--repo", repo))
	err := root.Execute()
	return buf.String(), err
}
```

- [ ] **Step 2: Write regression test confirming single-branch behavior unchanged**

Add to `cmd/trellis/main_test.go`:

```go
func TestNote_SingleBranch_ViaAppendOp(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Init and set up a task
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "test task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Note on the task
	out, err := runTrls(t, repo, "note", "--issue", "T-001", "--msg", "hello world")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
}
```

- [ ] **Step 3: Run baseline test to confirm it passes**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestNote_SingleBranch_ViaAppendOp -v
```
Expected: PASS (baseline before the code change)

- [ ] **Step 4: Add `appendOp` wrapper in `helpers.go`**

In `cmd/trellis/helpers.go`, add:

```go
// appendOp appends an op to the log and, in dual-branch mode, commits it to the worktree branch.
func appendOp(logPath string, op ops.Op) error {
	var gc ops.GitCommitter
	if appCtx.WorktreePath != "" {
		gc = git.New(appCtx.WorktreePath)
	}
	return ops.AppendAndCommit(logPath, appCtx.WorktreePath, op, gc)
}
```

Add imports to `helpers.go`:
- `"github.com/scullxbones/armature/internal/git"`
- `"github.com/scullxbones/armature/internal/ops"`

- [ ] **Step 5: Replace `ops.AppendOp(logPath, op)` in all nine worker command files**

In each file listed below, replace every `ops.AppendOp(logPath, op)` call with `appendOp(logPath, op)`:

- `cmd/trellis/claim.go` — two calls (scope overlap notes + main claim op)
- `cmd/trellis/note.go`
- `cmd/trellis/heartbeat.go`
- `cmd/trellis/transition.go`
- `cmd/trellis/create.go`
- `cmd/trellis/link.go`
- `cmd/trellis/decision.go`
- `cmd/trellis/reopen.go`
- `cmd/trellis/merged.go`

In each file, check whether the `ops` package is still imported for other purposes (e.g., `ops.Op`, `ops.OpNote` etc.). Keep the import if still used; remove it only if `AppendOp` was the sole usage.

- [ ] **Step 6: Run all tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS — single-branch behavior unchanged (worktreePath is "" so no git calls are made)

- [ ] **Step 7: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/helpers.go cmd/trellis/claim.go cmd/trellis/note.go cmd/trellis/heartbeat.go cmd/trellis/transition.go cmd/trellis/create.go cmd/trellis/link.go cmd/trellis/decision.go cmd/trellis/reopen.go cmd/trellis/merged.go cmd/trellis/main_test.go
git commit -m "feat(ops): route all worker op writes through AppendAndCommit for branch isolation"
```

---

## Chunk 4: Integration Test for Dual-Branch Isolation

### Task 5: Verify ops are committed to `_armature` branch only

**Files:**
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write integration test**

```go
func TestDualBranch_OpsCommittedToArmatureBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create an issue
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "test task", "--id", "T-001")
	require.NoError(t, err)

	// Materialize (reads ops dir, which is in worktree)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Write a note op — should commit to _armature, not to main
	_, err = runTrls(t, repo, "note", "--issue", "T-001", "--msg", "dual branch test")
	require.NoError(t, err)

	// Verify the commit appeared on _armature branch (inside the worktree)
	worktreePath := filepath.Join(repo, ".trellis")
	cmd := exec.Command("git", "-C", worktreePath, "log", "--oneline", "-3")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "ops: note")

	// Verify the main repo's log does NOT contain the ops commit
	mainCmd := exec.Command("git", "-C", repo, "log", "--oneline", "-5")
	mainOut, err := mainCmd.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(mainOut), "ops: note")
}
```

- [ ] **Step 2: Run test**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestDualBranch_OpsCommittedToArmatureBranch -v -timeout 60s
```
Expected: PASS

- [ ] **Step 3: Run full suite**

```bash
cd /home/brian/development/trellis && go test ./...
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/main_test.go
git commit -m "test: add integration test for dual-branch op isolation"
```

---

## Definition of Done Checklist

- [ ] In dual-branch mode, every `AppendOp` call is followed by a git commit in the `.trellis/` worktree
- [ ] The commit appears on the `_armature` branch, not on the working branch
- [ ] Single-branch mode behavior is completely unchanged (no extra git calls; nil gc is never invoked)
- [ ] `op.WorkerID` is safely truncated to ≤8 chars in commit messages
- [ ] `runTrls` test helper is defined in `main_test.go` for use by E2-002, E2-003, E2-004
- [ ] `go test ./...` passes clean
