# E2-003: Cross-Branch Merge Detection and Auto-Transition — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a worker's feature branch is merged into main, `arm sync` detects this and auto-transitions all `done` issues whose `Branch` field matches the merged branch to `merged` status by appending transition ops.

**Architecture:** A new `arm sync` command reads the materialized `state/issues/` directory, finds issues with `status=done` and `branch` set, checks if that branch is reachable from the current branch via `git merge-base --is-ancestor`, and appends `OpTransition{To: "merged"}` for each detected merge. An optional git `post-merge` hook template calls `arm sync` automatically. Sync works in both single- and dual-branch modes.

**Tech Stack:** Go, os/exec (git), testify

**Prerequisites:**
- E2-001 complete — `appCtx`, `ResolveContext`, dual-branch context all working
- E2-002 complete — `appendOp` helper and `runTrls` test helper both defined in `cmd/trellis/`

**Cross-plan note:** `arm sync` uses `appendOp` (defined in E2-002's `helpers.go`). Implement E2-002 before E2-003.

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/git/git.go` | Add `BranchMergedInto(branch, target string) (bool, error)` |
| Modify | `internal/git/git_test.go` | Tests for `BranchMergedInto` |
| Create | `internal/sync/sync.go` | `DetectMerges(issuesDir, targetBranch string, mc MergeChecker) ([]string, error)` |
| Create | `internal/sync/sync_test.go` | Unit tests with fake MergeChecker |
| Create | `cmd/trellis/sync.go` | `arm sync` command |
| Modify | `cmd/trellis/main.go` | Register `newSyncCmd()` |
| Modify | `cmd/trellis/main_test.go` | Integration test for `arm sync` |
| Modify | `cmd/trellis/init.go` | Write `post-merge.sh.template` during init |

---

## Chunk 1: Git Merge Detection

### Task 1: Add `BranchMergedInto` to git.Client

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

- [ ] **Step 1: Write failing tests**

Add to `internal/git/git_test.go`:

```go
func TestBranchMergedInto_Merged(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	// Detect what branch we're on
	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create and merge a feature branch
	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("checkout", "-b", "feature/my-work")
	gitRun("commit", "--allow-empty", "-m", "feat: work")
	gitRun("checkout", mainBranch)
	gitRun("merge", "--no-ff", "feature/my-work", "-m", "Merge feature/my-work")

	merged, err := c.BranchMergedInto("feature/my-work", mainBranch)
	require.NoError(t, err)
	assert.True(t, merged)
}

func TestBranchMergedInto_NotMerged(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	branchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(branchOut))

	gitRun := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	gitRun("checkout", "-b", "feature/unmerged")
	gitRun("commit", "--allow-empty", "-m", "wip")
	gitRun("checkout", mainBranch)

	merged, err := c.BranchMergedInto("feature/unmerged", mainBranch)
	require.NoError(t, err)
	assert.False(t, merged)
}

func TestBranchMergedInto_NonexistentBranch(t *testing.T) {
	repo := initTestRepo(t)
	c := git.New(repo)

	// Non-existent branch should return (false, nil) not an error
	merged, err := c.BranchMergedInto("feature/ghost", "main")
	assert.NoError(t, err)
	assert.False(t, merged)
}
```

Add `"strings"` to the imports in `git_test.go` if not already present.

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./internal/git/... -run TestBranchMergedInto -v
```
Expected: FAIL — `(*Client).BranchMergedInto undefined`

- [ ] **Step 3: Implement `BranchMergedInto`**

Append to `internal/git/git.go`:

```go
// BranchMergedInto checks if branch has been fully merged into target.
// Returns (false, nil) if the branch does not exist, rather than an error.
func (c *Client) BranchMergedInto(branch, target string) (bool, error) {
	// Check that branch exists
	check := exec.Command("git", "-C", c.repoPath, "rev-parse", "--verify", branch)
	if err := check.Run(); err != nil {
		return false, nil // branch doesn't exist
	}

	// Get the tip commit of branch
	tip := exec.Command("git", "-C", c.repoPath, "rev-parse", branch)
	tipOut, err := tip.Output()
	if err != nil {
		return false, fmt.Errorf("rev-parse %s: %w", branch, err)
	}
	sha := strings.TrimSpace(string(tipOut))

	return c.IsCommitOnBranch(sha, target)
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
git commit -m "feat(git): add BranchMergedInto for merge detection"
```

---

## Chunk 2: Sync Logic

### Task 2: Create `internal/sync` package

**Files:**
- Create: `internal/sync/sync.go`
- Create: `internal/sync/sync_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/sync/sync_test.go`:

```go
package sync_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/materialize"
	trellissync "github.com/scullxbones/armature/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMergeChecker struct {
	merged map[string]bool
}

func (f *fakeMergeChecker) BranchMergedInto(branch, target string) (bool, error) {
	return f.merged[branch], nil
}

func TestDetectMerges_ReturnsMergedIssueIDs(t *testing.T) {
	dir := t.TempDir()
	issuesStateDir := filepath.Join(dir, "state", "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	// done + merged branch
	issue1 := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "feature/merged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// done + unmerged branch
	issue2 := materialize.Issue{
		ID: "T-002", Status: "done", Branch: "feature/unmerged-work", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	// in-progress — should be skipped regardless of branch status
	issue3 := materialize.Issue{
		ID: "T-003", Status: "in-progress", Branch: "feature/wip", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue1))
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue2))
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue3))

	mc := &fakeMergeChecker{merged: map[string]bool{
		"feature/merged-work": true,
	}}

	ids, err := trellissync.DetectMerges(dir, "main", mc)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"T-001"}, ids)
}

func TestDetectMerges_NoBranch_Skipped(t *testing.T) {
	dir := t.TempDir()
	issuesStateDir := filepath.Join(dir, "state", "issues")
	require.NoError(t, os.MkdirAll(issuesStateDir, 0755))

	issue := materialize.Issue{
		ID: "T-001", Status: "done", Branch: "", Type: "task",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}
	require.NoError(t, materialize.WriteIssue(issuesStateDir, issue))

	mc := &fakeMergeChecker{merged: map[string]bool{}}

	ids, err := trellissync.DetectMerges(dir, "main", mc)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDetectMerges_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No state/issues dir — should return nil, nil

	mc := &fakeMergeChecker{merged: map[string]bool{}}
	ids, err := trellissync.DetectMerges(dir, "main", mc)
	assert.NoError(t, err)
	assert.Empty(t, ids)
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./internal/sync/... -v
```
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create `internal/sync/sync.go`**

```go
package sync

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// MergeChecker checks if a branch is merged into a target branch.
type MergeChecker interface {
	BranchMergedInto(branch, target string) (bool, error)
}

// DetectMerges scans all issues in issuesDir/state/issues/ and returns the IDs
// of done issues whose Branch has been merged into targetBranch.
func DetectMerges(issuesDir, targetBranch string, mc MergeChecker) ([]string, error) {
	issuesStateDir := filepath.Join(issuesDir, "state", "issues")
	entries, err := os.ReadDir(issuesStateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var merged []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		issue, err := materialize.LoadIssue(filepath.Join(issuesStateDir, entry.Name()))
		if err != nil {
			continue
		}
		if issue.Status != ops.StatusDone {
			continue
		}
		if issue.Branch == "" {
			continue
		}
		isMerged, err := mc.BranchMergedInto(issue.Branch, targetBranch)
		if err != nil {
			continue // skip on error, don't abort
		}
		if isMerged {
			merged = append(merged, issue.ID)
		}
	}
	return merged, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./internal/sync/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/sync/sync.go internal/sync/sync_test.go
git commit -m "feat(sync): add DetectMerges to find done issues with merged branches"
```

---

## Chunk 3: `arm sync` Command

### Task 3: Implement `arm sync` command

**Files:**
- Create: `cmd/trellis/sync.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/main_test.go`

Note: `arm sync` uses `appendOp` (defined in E2-002's `helpers.go`) and `runTrls` (defined in E2-002's `main_test.go`). Both must exist before compiling/testing this chunk.

Note on worker identity: `sync` uses `resolveWorkerAndLog()` because ops are appended to a worker log (same as all other commands). Users must run `arm worker-init` before `arm sync`, same as any other worker command.

- [ ] **Step 1: Write failing integration test**

In `cmd/trellis/main_test.go`, add:

```go
func TestSync_TransitionsMergedBranchIssuesToMerged(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "some feature", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done",
		"--branch", "feature/sync-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Create and merge the feature branch in the git repo
	currentBranchCmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	require.NoError(t, err)
	mainBranch := strings.TrimSpace(string(currentBranchOut))

	run(t, repo, "git", "checkout", "-b", "feature/sync-test")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "feat: sync test work")
	run(t, repo, "git", "checkout", mainBranch)
	run(t, repo, "git", "merge", "--no-ff", "feature/sync-test", "-m", "Merge feature/sync-test")

	// Run sync — should auto-transition T-001 to merged
	out, err := runTrls(t, repo, "sync")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "merged")

	// Verify via materialized state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	index, err := materialize.LoadIndex(filepath.Join(repo, ".issues", "state", "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["T-001"].Status)
}
```

Add `"strings"` to imports in `main_test.go` (it is already imported per the existing file — verify before adding).

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestSync -v
```
Expected: FAIL — `sync` command not found

- [ ] **Step 3: Create `cmd/trellis/sync.go`**

```go
package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/git"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	trellissync "github.com/scullxbones/armature/internal/sync"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var targetBranch string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Detect merged branches and auto-transition done issues to merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Materialize to ensure state files are up to date
			if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			if targetBranch == "" {
				gc := git.New(appCtx.RepoPath)
				branch, err := gc.CurrentBranch()
				if err != nil {
					return fmt.Errorf("detect current branch: %w", err)
				}
				targetBranch = branch
			}

			gc := git.New(appCtx.RepoPath)
			mergedIDs, err := trellissync.DetectMerges(issuesDir, targetBranch, gc)
			if err != nil {
				return fmt.Errorf("detect merges: %w", err)
			}

			if len(mergedIDs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No merged branches detected.")
				return nil
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			for _, id := range mergedIDs {
				op := ops.Op{
					Type:      ops.OpTransition,
					TargetID:  id,
					WorkerID:  workerID,
					Timestamp: nowEpoch(),
					Payload: ops.Payload{
						To:      ops.StatusMerged,
						Outcome: "auto-detected merge into " + targetBranch,
					},
				}
				if err := appendOp(logPath, op); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to transition %s: %v\n", id, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Transitioned %s to merged\n", id)
			}

			// Re-materialize so state files reflect the new merged status
			if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
				return fmt.Errorf("re-materialize: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&targetBranch, "into", "", "target branch to check merges against (default: current branch)")
	return cmd
}
```

- [ ] **Step 4: Register in `main.go`**

In `cmd/trellis/main.go`, add after `root.AddCommand(newMergedCmd())`:
```go
root.AddCommand(newSyncCmd())
```

- [ ] **Step 5: Run tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/sync.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat: add arm sync command for cross-branch merge detection and auto-transition"
```

---

## Chunk 4: Post-Merge Hook Template

### Task 4: Write `post-merge.sh.template` during `arm init`

**Files:**
- Modify: `cmd/trellis/init.go`
- Modify: `cmd/trellis/main_test.go`

Note: The `hooks/` directory is already created by `runInit` (`dirs` slice includes `filepath.Join(issuesDir, "hooks")`). Only the template file write needs to be added.

- [ ] **Step 1: Write test that verifies init writes the hook template**

In `cmd/trellis/main_test.go`, add:

```go
func TestInit_WritesPostMergeHookTemplate(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)

	hookPath := filepath.Join(repo, ".issues", "hooks", "post-merge.sh.template")
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "arm sync")
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestInit_WritesPostMergeHookTemplate -v
```
Expected: FAIL — file does not exist

- [ ] **Step 3: Add hook template to `runInit` in `cmd/trellis/init.go`**

Add a constant at the top of the file (after the package declaration):

```go
const postMergeHookTemplate = `#!/bin/sh
# Armature post-merge hook: auto-detect merged branches and transition done issues to merged.
# To activate: cp this file to .git/hooks/post-merge && chmod +x .git/hooks/post-merge
arm sync
`
```

In `runInit`, after writing the SCHEMA file and before detecting project type, write the hook template:

```go
hookTemplatePath := filepath.Join(issuesDir, "hooks", "post-merge.sh.template")
if err := os.WriteFile(hookTemplatePath, []byte(postMergeHookTemplate), 0644); err != nil {
    return fmt.Errorf("write post-merge hook template: %w", err)
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/init.go cmd/trellis/main_test.go
git commit -m "feat(init): write post-merge.sh.template hook for arm sync"
```

---

## Definition of Done Checklist

- [ ] `arm sync` detects done issues whose branch is merged into the current/specified branch
- [ ] Detected issues are transitioned to `merged` via appended `OpTransition` ops
- [ ] `arm sync` materializes before detection and after to keep state current
- [ ] Non-existent or non-merged branches are silently skipped (return false, nil)
- [ ] A `post-merge.sh.template` is written to `.armature/hooks/` during `arm init`
- [ ] `go test ./...` passes clean
