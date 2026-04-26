# E2-004: PR-Based Done-to-Merged Workflow — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In dual-branch mode, `done` and `merged` are distinct states — issues stay `done` after work completes and move to `merged` only after the PR/branch merges. `arm status` shows a clear separation of done-awaiting-merge vs fully merged issues, and `arm merged` validates current state before accepting an explicit merge op.

**Architecture:** The `SingleBranchMode=false` flag (set by E2-001) already prevents auto-merge in `applyTransition`. This plan adds: (1) `Branch` and `PR` fields to `IndexEntry` so the status command can show them without loading per-issue files; (2) `arm status` command rendering issues grouped by status with `done` issues in dual-branch mode calling out awaiting-merge; (3) `arm merged` rewritten to validate state before accepting the op and to accept an optional `--pr` flag.

**Tech Stack:** Go, testify

**Prerequisites:**
- E2-001 complete — `appCtx.Mode` correctly set
- E2-002 complete — `appendOp` helper defined in `helpers.go`, `runTrls` test helper in `main_test.go`
- E2-003 complete — `arm sync` for auto-detection (used in integration test)

**Cross-plan note:** E2-002 made a narrow change to `merged.go` (replacing `ops.AppendOp` with `appendOp`). This plan completely rewrites `merged.go`. Ensure the rewrite includes `appendOp` (it does — see Task 3 Step 3).

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/materialize/state.go` | Add `Branch`, `PR` to `IndexEntry`; populate in `BuildIndex` |
| Modify | `internal/materialize/engine_test.go` | Test that `BuildIndex` populates Branch and PR |
| Create | `cmd/trellis/status.go` | `arm status` — grouped status view, highlights done-awaiting-merge |
| Modify | `cmd/trellis/main.go` | Register `newStatusCmd()` |
| Modify | `cmd/trellis/merged.go` | Full rewrite: validate done state, add `--pr` flag, use `appendOp` |
| Modify | `cmd/trellis/main_test.go` | Integration tests for `arm status` and `arm merged` |

---

## Chunk 1: Add Branch/PR to IndexEntry

### Task 1: Populate `Branch` and `PR` in `IndexEntry`

**Files:**
- Modify: `internal/materialize/state.go`
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write failing test**

In `internal/materialize/engine_test.go`, add:

```go
func TestBuildIndex_IncludesBranchAndPR(t *testing.T) {
	s := NewState()
	s.Issues["T-001"] = &Issue{
		ID: "T-001", Type: "task", Status: "done",
		Title: "some task", Branch: "feature/my-work", PR: "42",
		Children: []string{}, BlockedBy: []string{}, Blocks: []string{},
	}

	index := s.BuildIndex()
	entry, ok := index["T-001"]
	require.True(t, ok)
	assert.Equal(t, "feature/my-work", entry.Branch)
	assert.Equal(t, "42", entry.PR)
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./internal/materialize/... -run TestBuildIndex_IncludesBranchAndPR -v
```
Expected: FAIL — `entry.Branch` undefined on `IndexEntry`

- [ ] **Step 3: Add Branch and PR to IndexEntry and BuildIndex**

In `internal/materialize/state.go`, find the `IndexEntry` struct and add:
```go
Branch string `json:"branch,omitempty"`
PR     string `json:"pr,omitempty"`
```

In `BuildIndex()`, add to the `IndexEntry` construction:
```go
Branch: issue.Branch,
PR:     issue.PR,
```

- [ ] **Step 4: Run tests**

```bash
cd /home/brian/development/trellis && go test ./internal/materialize/... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add internal/materialize/state.go internal/materialize/engine_test.go
git commit -m "feat(materialize): include Branch and PR in IndexEntry for status display"
```

---

## Chunk 2: `arm status` Command

### Task 2: Implement `arm status`

**Files:**
- Create: `cmd/trellis/status.go`
- Modify: `cmd/trellis/main.go`
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write failing integration test**

In `cmd/trellis/main_test.go`, add:

```go
func TestStatus_ShowsInProgressIssue(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my work", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.Contains(t, out, "in-progress")
	assert.Contains(t, out, "T-001")
}

func TestStatus_DualBranch_DoneShowsAwaitingMerge(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Use dual-branch mode so done != merged
	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "pending merge", "--id", "T-001")
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
		"--branch", "feature/my-pr", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	// In dual-branch mode, done issues should be labeled "awaiting merge"
	assert.Contains(t, out, "awaiting merge")
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "feature/my-pr")
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestStatus -v
```
Expected: FAIL — `status` command not registered

- [ ] **Step 3: Create `cmd/trellis/status.go`**

```go
package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

// statusOrder defines display priority — lower number appears first.
var statusOrder = map[string]int{
	ops.StatusInProgress: 0,
	ops.StatusClaimed:    1,
	ops.StatusDone:       2, // "awaiting merge" in dual-branch mode
	ops.StatusOpen:       3,
	ops.StatusBlocked:    4,
	ops.StatusMerged:     5,
	ops.StatusCancelled:  6,
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show issues grouped by status",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
				return err
			}

			index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
			if err != nil {
				return err
			}

			// Group by status
			groups := make(map[string][]string)
			for id, entry := range index {
				groups[entry.Status] = append(groups[entry.Status], id)
			}

			// Sort statuses by display priority
			statuses := make([]string, 0, len(groups))
			for s := range groups {
				statuses = append(statuses, s)
			}
			sort.Slice(statuses, func(i, j int) bool {
				oi, ok1 := statusOrder[statuses[i]]
				oj, ok2 := statusOrder[statuses[j]]
				if !ok1 {
					oi = 99
				}
				if !ok2 {
					oj = 99
				}
				return oi < oj
			})

			for _, status := range statuses {
				ids := groups[status]
				sort.Strings(ids)

				label := status
				if status == ops.StatusDone && !singleBranch {
					label = "done (awaiting merge)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n=== %s ===\n", label)

				for _, id := range ids {
					entry := index[id]
					line := fmt.Sprintf("  %-12s  %s", id, entry.Title)
					if status == ops.StatusDone && !singleBranch && entry.Branch != "" {
						line += fmt.Sprintf("  [branch: %s", entry.Branch)
						if entry.PR != "" {
							line += fmt.Sprintf(", PR: #%s", entry.PR)
						}
						line += "]"
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}

			return nil
		},
	}
	return cmd
}
```

- [ ] **Step 4: Register in `main.go`**

In `cmd/trellis/main.go`, add:
```go
root.AddCommand(newStatusCmd())
```

- [ ] **Step 5: Run tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/status.go cmd/trellis/main.go cmd/trellis/main_test.go
git commit -m "feat: add arm status command with grouped view and awaiting-merge callout"
```

---

## Chunk 3: Rewrite `arm merged` with State Validation

### Task 3: Validate done state and add `--pr` flag

**Files:**
- Modify: `cmd/trellis/merged.go`
- Modify: `cmd/trellis/main_test.go`

Note: E2-002 made a narrow change to this file. This task replaces the entire file content. The new version uses `appendOp` (defined in E2-002), which is required.

- [ ] **Step 1: Write failing tests**

In `cmd/trellis/main_test.go`, add:

```go
func TestMerged_RequiresDoneState_InDualBranchMode(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "new task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Try to merge an open issue in dual-branch mode — should fail with clear error
	_, err = runTrls(t, repo, "merged", "--issue", "T-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "done")
}

func TestMerged_AcceptsDoneIssue_SingleBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// In single-branch mode the validation is skipped — any issue can be passed to merged
	// (single-branch auto-merges on transition to done, so merged is a no-op)
	out, err := runTrls(t, repo, "merged", "--issue", "T-001", "--pr", "123")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
}

func TestMerged_AcceptsDoneIssue_DualBranch(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "my task", "--id", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "T-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "T-001", "--to", "done", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Now in done state — merged should accept it
	out, err := runTrls(t, repo, "merged", "--issue", "T-001", "--pr", "42")
	require.NoError(t, err)
	assert.Contains(t, out, "T-001")
	assert.Contains(t, out, "#42")
}
```

- [ ] **Step 2: Run to verify test behavior**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestMerged -v
```
Expected: `TestMerged_RequiresDoneState_InDualBranchMode` FAIL (no validation yet), others may pass or fail

- [ ] **Step 3: Rewrite `cmd/trellis/merged.go`**

```go
package main

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/spf13/cobra"
)

func newMergedCmd() *cobra.Command {
	var issueID, pr string

	cmd := &cobra.Command{
		Use:   "merged",
		Short: "Mark a done issue as merged after its branch/PR is merged",
		RunE: func(cmd *cobra.Command, args []string) error {
			issuesDir := appCtx.IssuesDir
			singleBranch := appCtx.Mode == "single-branch"

			// Materialize to get current state
			if _, err := materialize.Materialize(issuesDir, singleBranch); err != nil {
				return fmt.Errorf("materialize: %w", err)
			}

			index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
			if err != nil {
				return fmt.Errorf("load index: %w", err)
			}

			entry, ok := index[issueID]
			if !ok {
				return fmt.Errorf("issue %s not found", issueID)
			}

			// In dual-branch mode, require current status to be "done"
			if !singleBranch && entry.Status != ops.StatusDone {
				return fmt.Errorf("issue %s is in status %q; arm merged requires status=done (transition it to done first)", issueID, entry.Status)
			}

			workerID, logPath, err := resolveWorkerAndLog()
			if err != nil {
				return err
			}

			op := ops.Op{
				Type:      ops.OpTransition,
				TargetID:  issueID,
				Timestamp: nowEpoch(),
				WorkerID:  workerID,
				Payload:   ops.Payload{To: ops.StatusMerged, PR: pr},
			}
			if err := appendOp(logPath, op); err != nil {
				return err
			}

			if singleBranch {
				fmt.Fprintf(cmd.OutOrStdout(), "Note: in single-branch mode, done→merged is automatic. Op recorded for %s.\n", issueID)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Marked %s as merged", issueID)
				if pr != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " (PR #%s)", pr)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "issue ID")
	cmd.Flags().StringVar(&pr, "pr", "", "PR number or URL")
	cmd.MarkFlagRequired("issue")
	return cmd
}
```

- [ ] **Step 4: Run all tests**

```bash
cd /home/brian/development/trellis && go test ./... -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/merged.go cmd/trellis/main_test.go
git commit -m "feat(merged): validate done state in dual-branch mode and add --pr flag"
```

---

## Chunk 4: End-to-End Dual-Branch Workflow Test

### Task 4: Integration test for full done→merged workflow

**Files:**
- Modify: `cmd/trellis/main_test.go`

- [ ] **Step 1: Write the integration test**

```go
func TestDualBranch_DoneToMergedWorkflow(t *testing.T) {
	// Full workflow: init --dual-branch → create → claim → in-progress → done →
	// status shows awaiting merge → merged --pr → status shows merged
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "init", "--dual-branch")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "feature work", "--id", "F-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "--issue", "F-001")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "in-progress")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "F-001", "--to", "done",
		"--branch", "feature/e2-test", "--outcome", "done")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Status should show done (awaiting merge)
	statusOut, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.Contains(t, statusOut, "awaiting merge")
	assert.Contains(t, statusOut, "F-001")
	assert.Contains(t, statusOut, "feature/e2-test")

	// Mark as merged with PR reference
	mergedOut, err := runTrls(t, repo, "merged", "--issue", "F-001", "--pr", "99")
	require.NoError(t, err)
	assert.Contains(t, mergedOut, "F-001")
	assert.Contains(t, mergedOut, "#99")

	// Materialize and verify final state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// In dual-branch mode, the issues dir is in the worktree
	issuesDir := filepath.Join(repo, ".trellis", ".issues")
	index, err := materialize.LoadIndex(filepath.Join(issuesDir, "state", "index.json"))
	require.NoError(t, err)
	assert.Equal(t, "merged", index["F-001"].Status)

	// Status should no longer show done-awaiting-merge for F-001
	finalStatus, err := runTrls(t, repo, "status")
	require.NoError(t, err)
	assert.NotContains(t, finalStatus, "awaiting merge")
}
```

- [ ] **Step 2: Run the test**

```bash
cd /home/brian/development/trellis && go test ./cmd/trellis/... -run TestDualBranch_DoneToMergedWorkflow -v -timeout 60s
```
Expected: PASS

- [ ] **Step 3: Run full suite**

```bash
cd /home/brian/development/trellis && go test ./...
```
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
cd /home/brian/development/trellis && git add cmd/trellis/main_test.go
git commit -m "test: add full dual-branch done-to-merged workflow integration test"
```

---

## Definition of Done Checklist

- [ ] `arm status` renders issues grouped by status
- [ ] In dual-branch mode, `done` issues display as "done (awaiting merge)" with their branch/PR info
- [ ] `arm merged --issue X --pr 42` records the merge op with PR reference
- [ ] In dual-branch mode, `arm merged` validates current status is `done` before accepting
- [ ] In single-branch mode, `arm merged` works without validation (with explanatory note)
- [ ] `IndexEntry` includes `Branch` and `PR` fields populated by `BuildIndex`
- [ ] `go test ./...` passes clean
