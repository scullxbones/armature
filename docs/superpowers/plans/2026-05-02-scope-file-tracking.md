# Scope File Tracking Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `arm scope-rename` and `arm scope-delete` commands plus post-commit hook auto-detection so that issue scope entries stay accurate when files are renamed or deleted.

**Architecture:** Two new event-sourced op types (`scope-rename`, `scope-delete`) follow the existing per-issue aggregate pattern: one op per affected issue, emitted at the same timestamp. The materializer applies them as substring-replace and exact-remove respectively. The post-commit hook detects renames/deletions from the most recent commit and emits the appropriate ops automatically. The W10 phantom-scope validator is narrowed to skip terminal-status issues.

**Tech Stack:** Go 1.22+, Cobra (CLI), testify (testing), standard library only (no new deps).

---

## Chunk 1: Op Types, Materializer, and W10 Narrowing

### Task 1: Add Op Type Constants and Payload Fields

**Files:**
- Modify: `internal/ops/types.go`

- [ ] **Step 1: Add op type constants**

In `internal/ops/types.go`, add after `OpCitationAccepted`:

```go
OpScopeRename = "scope-rename"
OpScopeDelete = "scope-delete"
```

- [ ] **Step 2: Add to ValidOpTypes**

In the `ValidOpTypes` map, add:

```go
OpScopeRename: true,
OpScopeDelete: true,
```

- [ ] **Step 3: Add payload fields**

In the `Payload` struct, add after the `// assign` comment block:

```go
// scope-rename
OldPath string `json:"old_path,omitempty"`
NewPath string `json:"new_path,omitempty"`

// scope-delete
DeletedPath string `json:"deleted_path,omitempty"`
```

- [ ] **Step 4: Run existing tests to confirm nothing broken**

```bash
cd /home/brian/development/armature && go test ./internal/ops/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ops/types.go
git commit -m "feat: add OpScopeRename and OpScopeDelete op types and payload fields"
```

---

### Task 2: Write Failing Unit Tests for applyScopeRename

**Files:**
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/materialize/engine_test.go`:

```go
func TestApplyScopeRenameOp_ExactPath(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/trellis/main.go", "cmd/trellis/util.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "cmd/trellis/main.go", NewPath: "cmd/armature/main.go"},
	}))

	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"cmd/armature/main.go", "cmd/trellis/util.go"}, issue.Scope)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyScopeRenameOp_GlobPattern(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/trellis/*.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "cmd/trellis", NewPath: "cmd/armature"},
	}))

	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"cmd/armature/*.go"}, issue.Scope)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyScopeRenameOp_NoMatch_NoOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/other/main.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "cmd/trellis", NewPath: "cmd/armature"},
	}))

	issue := state.Issues["task-01"]
	// Updated must NOT change when no scope entries matched
	assert.Equal(t, int64(100), issue.Updated)
	assert.Equal(t, []string{"cmd/other/main.go"}, issue.Scope)
}

func TestApplyScopeRenameOp_Idempotent(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/trellis/main.go"}},
	}))

	op := ops.Op{
		Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "cmd/trellis/main.go", NewPath: "cmd/armature/main.go"},
	}
	require.NoError(t, state.ApplyOp(op))
	// Apply same op again — OldPath no longer present, so no-op
	require.NoError(t, state.ApplyOp(op))

	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"cmd/armature/main.go"}, issue.Scope)
}

func TestApplyScopeRenameOp_UnknownIssue_Tolerated(t *testing.T) {
	state := NewState()
	// No issue exists — must not error
	err := state.ApplyOp(ops.Op{
		Type: ops.OpScopeRename, TargetID: "nonexistent", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{OldPath: "cmd/old", NewPath: "cmd/new"},
	})
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run to confirm they fail (OpScopeRename unknown)**

```bash
cd /home/brian/development/armature && go test ./internal/materialize/... -run TestApplyScopeRename -v
```

Expected: FAIL — "unknown op type: scope-rename"

---

### Task 3: Write Failing Unit Tests for applyScopeDelete

**Files:**
- Modify: `internal/materialize/engine_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/materialize/engine_test.go`:

```go
func TestApplyScopeDeleteOp_ExactMatch(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/trellis/main.go", "cmd/other/util.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "cmd/trellis/main.go"},
	}))

	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"cmd/other/util.go"}, issue.Scope)
	assert.Equal(t, int64(200), issue.Updated)
}

func TestApplyScopeDeleteOp_GlobNotRemoved(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/trellis/*.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "cmd/trellis/main.go"},
	}))

	// Glob entry does NOT match exact path — must remain
	issue := state.Issues["task-01"]
	assert.Equal(t, []string{"cmd/trellis/*.go"}, issue.Scope)
	assert.Equal(t, int64(100), issue.Updated) // Updated unchanged (no-op)
}

func TestApplyScopeDeleteOp_NoMatch_NoOp(t *testing.T) {
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task",
			Scope: []string{"cmd/other/main.go"}},
	}))

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "cmd/trellis/main.go"},
	}))

	issue := state.Issues["task-01"]
	assert.Equal(t, int64(100), issue.Updated)
	assert.Equal(t, []string{"cmd/other/main.go"}, issue.Scope)
}

func TestApplyScopeDeleteOp_UnknownIssue_Tolerated(t *testing.T) {
	state := NewState()
	err := state.ApplyOp(ops.Op{
		Type: ops.OpScopeDelete, TargetID: "nonexistent", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{DeletedPath: "cmd/trellis/main.go"},
	})
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd /home/brian/development/armature && go test ./internal/materialize/... -run TestApplyScopeDelete -v
```

Expected: FAIL — "unknown op type: scope-delete"

---

### Task 4: Implement applyScopeRename and applyScopeDelete (committed together)

**Files:**
- Modify: `internal/materialize/engine.go`

> **Critical:** Both `case ops.OpScopeRename` and `case ops.OpScopeDelete` must be added to `ApplyOp` in the **same commit**. A log containing one op type but missing the other switch case causes hard-error on replay.

- [ ] **Step 1: Add both cases to ApplyOp**

In `ApplyOp`'s switch statement in `internal/materialize/engine.go`, add before `default:`:

```go
case ops.OpScopeRename:
    return s.applyScopeRename(op)
case ops.OpScopeDelete:
    return s.applyScopeDelete(op)
```

- [ ] **Step 2: Implement applyScopeRename**

Add to `internal/materialize/engine.go` (before the helper functions at the bottom):

```go
func (s *State) applyScopeRename(op ops.Op) error {
    issue, ok := s.Issues[op.TargetID]
    if !ok {
        return nil
    }
    changed := false
    for i, entry := range issue.Scope {
        if renamed := strings.ReplaceAll(entry, op.Payload.OldPath, op.Payload.NewPath); renamed != entry {
            issue.Scope[i] = renamed
            changed = true
        }
    }
    if changed {
        issue.Updated = op.Timestamp
    }
    return nil
}
```

Add `"strings"` to the import block in `engine.go` if not already present.

- [ ] **Step 3: Implement applyScopeDelete**

Add to `internal/materialize/engine.go`:

```go
func (s *State) applyScopeDelete(op ops.Op) error {
    issue, ok := s.Issues[op.TargetID]
    if !ok {
        return nil
    }
    before := len(issue.Scope)
    result := issue.Scope[:0]
    for _, entry := range issue.Scope {
        if entry != op.Payload.DeletedPath {
            result = append(result, entry)
        }
    }
    issue.Scope = result
    if len(issue.Scope) != before {
        issue.Updated = op.Timestamp
    }
    return nil
}
```

- [ ] **Step 4: Run all materializer tests**

```bash
cd /home/brian/development/armature && go test ./internal/materialize/... -v
```

Expected: all tests pass including the new scope rename/delete tests.

- [ ] **Step 5: Commit both methods together**

```bash
git add internal/materialize/engine.go internal/materialize/engine_test.go
git commit -m "feat: implement applyScopeRename and applyScopeDelete in materializer"
```

---

### Task 5: Narrow W10 Phantom Scope Check

**Files:**
- Modify: `internal/validate/validate.go`
- Modify: `internal/validate/validate_test.go`

- [ ] **Step 1: Write failing tests for status filtering**

Add to `internal/validate/validate_test.go`:

```go
func TestCheckW10PhantomScope_SkipsTerminalStatuses(t *testing.T) {
    // Create a temp dir so filepath.Glob has a real FS to work with
    repoPath := t.TempDir()

    terminalStatuses := []string{"merged", "done", "cancelled"}
    for _, status := range terminalStatuses {
        t.Run(status, func(t *testing.T) {
            issues := map[string]*materialize.Issue{
                "task-01": {
                    ID:     "task-01",
                    Type:   "task",
                    Status: status,
                    Scope:  []string{"nonexistent-file-that-never-existed.go"},
                },
            }
            result := checkW10PhantomScope(issues, repoPath)
            assert.Empty(t, result, "terminal status %q should be skipped by W10", status)
        })
    }
}

func TestCheckW10PhantomScope_ChecksBlockedIssues(t *testing.T) {
    repoPath := t.TempDir()

    issues := map[string]*materialize.Issue{
        "task-01": {
            ID:     "task-01",
            Type:   "task",
            Status: "blocked",
            Scope:  []string{"nonexistent-ghost-file.go"},
        },
    }
    result := checkW10PhantomScope(issues, repoPath)
    assert.NotEmpty(t, result, "blocked issues should still be checked by W10")
    assert.Contains(t, result[0], "phantom scope")
}

func TestCheckW10PhantomScope_SkipsTerminal_EpicsAndStories(t *testing.T) {
    repoPath := t.TempDir()

    issues := map[string]*materialize.Issue{
        "epic-01": {
            ID:     "epic-01",
            Type:   "epic",
            Status: "done",
            Scope:  []string{"nonexistent-epic-file.go"},
        },
        "story-01": {
            ID:     "story-01",
            Type:   "story",
            Status: "merged",
            Scope:  []string{"nonexistent-story-file.go"},
        },
    }
    result := checkW10PhantomScope(issues, repoPath)
    assert.Empty(t, result, "terminal epics and stories should also be skipped")
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd /home/brian/development/armature && go test ./internal/validate/... -run TestCheckW10Phantom -v
```

Expected: tests for `SkipsTerminalStatuses` and `SkipsTerminal_EpicsAndStories` FAIL (currently the function checks all issues).

- [ ] **Step 3: Update checkW10PhantomScope to skip terminal statuses**

In `internal/validate/validate.go`, replace `checkW10PhantomScope`:

```go
func checkW10PhantomScope(issues map[string]*materialize.Issue, repoPath string) []string {
    var warns []string
    for id, issue := range issues {
        // Skip terminal issues — their scope was valid when work completed.
        // blocked is intentionally NOT skipped: it is still active work.
        if issue.Status == ops.StatusMerged || issue.Status == ops.StatusDone || issue.Status == ops.StatusCancelled {
            continue
        }
        for _, glob := range issue.Scope {
            matches, err := filepath.Glob(filepath.Join(repoPath, glob))
            if err != nil || len(matches) == 0 {
                warns = append(warns, fmt.Sprintf("phantom scope: %s on %s does not match any file", glob, id))
            }
        }
    }
    return warns
}
```

Note: the `ops` package is already imported in `validate.go`. The call site in `Validate()` already appends the return value to `infos` — do not change that routing.

- [ ] **Step 4: Run validate tests**

```bash
cd /home/brian/development/armature && go test ./internal/validate/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/validate/validate.go internal/validate/validate_test.go
git commit -m "feat: narrow W10 phantom scope check to skip terminal-status issues"
```

---

## Chunk 2: Commands and Hook Extension

### Task 6: arm scope-rename Command

**Files:**
- Create: `cmd/armature/scope_rename.go`
- Create: `cmd/armature/scope_rename_test.go`
- Modify: `cmd/armature/main.go`

- [ ] **Step 1: Write failing integration tests**

Create `cmd/armature/scope_rename_test.go`:

```go
package main

import (
    "testing"

    "github.com/scullxbones/armature/internal/materialize"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestScopeRenameCmd_EmptyArgs(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "scope-rename")
    assert.Error(t, err)
}

func TestScopeRenameCmd_EqualArgs(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "scope-rename", "cmd/foo", "cmd/foo")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "old-path and new-path must differ")
}

func TestScopeRenameCmd_NoMatch_Warning(t *testing.T) {
    repo := setupRepoWithTask(t)
    out, err := runTrls(t, repo, "scope-rename", "cmd/nonexistent", "cmd/other")
    require.NoError(t, err) // exits 0
    assert.Contains(t, out, "no scope entries reference")
}

func TestScopeRenameCmd_RenamesAffectedIssues(t *testing.T) {
    repo := setupRepoWithTaskAndScope(t, []string{"cmd/trellis/main.go", "cmd/trellis/util.go"})

    out, err := runTrls(t, repo, "scope-rename", "cmd/trellis", "cmd/armature")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")

    // Verify state was rematerialized with renamed scope
    ctx := loadTestState(t, repo)
    issue := ctx.Issues["task-01"]
    require.NotNil(t, issue)
    assert.Contains(t, issue.Scope, "cmd/armature/main.go")
    assert.Contains(t, issue.Scope, "cmd/armature/util.go")
    assert.NotContains(t, issue.Scope, "cmd/trellis/main.go")
}

func TestScopeRenameCmd_EmitsOneOpPerAffectedIssue(t *testing.T) {
    repo := setupRepoWithTaskAndScope(t, []string{"cmd/trellis/main.go"})

    // Create a second task with the same prefix
    _, err := runTrls(t, repo, "create", "--title", "Task 2", "--type", "task", "--id", "task-02")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "amend", "task-02", "--scope", "cmd/trellis/util.go")
    require.NoError(t, err)

    out, err := runTrls(t, repo, "scope-rename", "cmd/trellis", "cmd/armature")
    require.NoError(t, err)

    // Output should mention both tasks
    assert.Contains(t, out, "task-01")
    assert.Contains(t, out, "task-02")

    ctx := loadTestState(t, repo)
    assert.Contains(t, ctx.Issues["task-01"].Scope, "cmd/armature/main.go")
    assert.Contains(t, ctx.Issues["task-02"].Scope, "cmd/armature/util.go")
}

// setupRepoWithTaskAndScope creates a repo with task-01 having the given scope.
func setupRepoWithTaskAndScope(t *testing.T, scope []string) string {
    t.Helper()
    repo := setupRepoWithTask(t)
    args := []string{"amend", "task-01"}
    for _, s := range scope {
        args = append(args, "--scope", s)
    }
    _, err := runTrls(t, repo, args...)
    require.NoError(t, err)
    return repo
}

// loadTestState materializes and returns the current state for a test repo.
func loadTestState(t *testing.T, repo string) *materialize.State {
    t.Helper()
    issuesDir := repo + "/.armature"
    stateDir := getTestStateDir(t, repo)
    state, _, err := materialize.MaterializeAndReturn(issuesDir, stateDir, false)
    require.NoError(t, err)
    return state
}
```

- [ ] **Step 2: Run to confirm tests fail**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run TestScopeRenameCmd -v
```

Expected: FAIL — "unknown command: scope-rename"

- [ ] **Step 3: Implement the command**

Create `cmd/armature/scope_rename.go`:

```go
package main

import (
    "fmt"
    "strings"

    "github.com/scullxbones/armature/internal/materialize"
    "github.com/scullxbones/armature/internal/ops"
    "github.com/spf13/cobra"
)

func newScopeRenameCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "scope-rename <old-path> <new-path>",
        Short: "Rename a path/prefix across all issue scope entries",
        Args:  cobra.ExactArgs(2),
        RunE: func(cmd *cobra.Command, args []string) error {
            oldPath, newPath := args[0], args[1]

            if oldPath == "" || newPath == "" {
                return fmt.Errorf("old-path and new-path must not be empty")
            }
            if oldPath == newPath {
                return fmt.Errorf("old-path and new-path must differ")
            }

            state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
            if err != nil {
                return fmt.Errorf("materialize: %w", err)
            }

            // Find affected issues
            var affected []string
            for id, issue := range state.Issues {
                for _, entry := range issue.Scope {
                    if strings.Contains(entry, oldPath) {
                        affected = append(affected, id)
                        break
                    }
                }
            }

            if len(affected) == 0 {
                _, _ = fmt.Fprintf(cmd.OutOrStdout(), "no scope entries reference %s\n", oldPath)
                return nil
            }

            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            ts := nowEpoch()
            for _, id := range affected {
                op := ops.Op{
                    Type:      ops.OpScopeRename,
                    TargetID:  id,
                    Timestamp: ts,
                    WorkerID:  workerID,
                    Payload:   ops.Payload{OldPath: oldPath, NewPath: newPath},
                }
                if err := appendLowStakesOp(logPath, op); err != nil {
                    return fmt.Errorf("write op for %s: %w", id, err)
                }
            }

            _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Renamed %q to %q in %d issue(s): %s\n",
                oldPath, newPath, len(affected), strings.Join(affected, ", "))

            if _, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); err != nil {
                return fmt.Errorf("re-materialize: %w", err)
            }
            return nil
        },
    }
}
```

- [ ] **Step 4: Register the command in main.go**

In `cmd/armature/main.go`, add after `amendCmd` registration:

```go
scopeRenameCmd := newScopeRenameCmd()
scopeRenameCmd.GroupID = "workflow"
root.AddCommand(scopeRenameCmd)
```

- [ ] **Step 5: Run tests**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run TestScopeRenameCmd -v
```

Expected: all scope-rename tests pass.

- [ ] **Step 6: Run full test suite**

```bash
cd /home/brian/development/armature && go test ./... 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/scope_rename.go cmd/armature/scope_rename_test.go cmd/armature/main.go
git commit -m "feat: add arm scope-rename command"
```

---

### Task 7: arm scope-delete Command

**Files:**
- Create: `cmd/armature/scope_delete.go`
- Create: `cmd/armature/scope_delete_test.go`
- Modify: `cmd/armature/main.go`

- [ ] **Step 1: Write failing integration tests**

Create `cmd/armature/scope_delete_test.go`:

```go
package main

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestScopeDeleteCmd_EmptyArg(t *testing.T) {
    repo := setupRepoWithTask(t)
    _, err := runTrls(t, repo, "scope-delete")
    assert.Error(t, err)
}

func TestScopeDeleteCmd_NoMatch_Warning(t *testing.T) {
    repo := setupRepoWithTask(t)
    out, err := runTrls(t, repo, "scope-delete", "cmd/nonexistent.go")
    require.NoError(t, err) // exits 0
    assert.Contains(t, out, "no scope entries contain")
}

func TestScopeDeleteCmd_RemovesExactEntry(t *testing.T) {
    repo := setupRepoWithTaskAndScope(t, []string{"cmd/trellis/main.go", "cmd/other/util.go"})

    out, err := runTrls(t, repo, "scope-delete", "cmd/trellis/main.go")
    require.NoError(t, err)
    assert.Contains(t, out, "task-01")

    ctx := loadTestState(t, repo)
    issue := ctx.Issues["task-01"]
    require.NotNil(t, issue)
    assert.NotContains(t, issue.Scope, "cmd/trellis/main.go")
    assert.Contains(t, issue.Scope, "cmd/other/util.go")
}

func TestScopeDeleteCmd_WarnOnEmptyScope(t *testing.T) {
    // A non-terminal issue that would have empty scope after deletion gets a warning
    repo := setupRepoWithTaskAndScope(t, []string{"cmd/trellis/main.go"})

    out, stderr, err := runTrlsWithStderr(t, repo, "scope-delete", "cmd/trellis/main.go")
    require.NoError(t, err) // proceeds regardless
    _ = out
    // Warning must mention the issue ID
    assert.Contains(t, stderr, "task-01")
}

func TestScopeDeleteCmd_EmitsOneOpPerAffectedIssue(t *testing.T) {
    repo := setupRepoWithTaskAndScope(t, []string{"cmd/trellis/main.go"})

    // Create second task with same exact scope entry
    _, err := runTrls(t, repo, "create", "--title", "Task 2", "--type", "task", "--id", "task-02")
    require.NoError(t, err)
    _, err = runTrls(t, repo, "amend", "task-02", "--scope", "cmd/trellis/main.go")
    require.NoError(t, err)

    out, _, err := runTrlsWithStderr(t, repo, "scope-delete", "cmd/trellis/main.go")
    require.NoError(t, err)

    assert.Contains(t, out, "task-01")
    assert.Contains(t, out, "task-02")

    ctx := loadTestState(t, repo)
    assert.NotContains(t, ctx.Issues["task-01"].Scope, "cmd/trellis/main.go")
    assert.NotContains(t, ctx.Issues["task-02"].Scope, "cmd/trellis/main.go")
}
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run TestScopeDeleteCmd -v
```

Expected: FAIL — "unknown command: scope-delete"

- [ ] **Step 3: Implement the command**

Create `cmd/armature/scope_delete.go`:

```go
package main

import (
    "fmt"
    "strings"

    "github.com/scullxbones/armature/internal/materialize"
    "github.com/scullxbones/armature/internal/ops"
    "github.com/spf13/cobra"
)

func newScopeDeleteCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "scope-delete <path>",
        Short: "Remove an exact file path from all issue scope entries",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            path := args[0]
            if path == "" {
                return fmt.Errorf("path must not be empty")
            }

            state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
            if err != nil {
                return fmt.Errorf("materialize: %w", err)
            }

            // Find affected issues (exact match only)
            var affected []string
            for id, issue := range state.Issues {
                for _, entry := range issue.Scope {
                    if entry == path {
                        affected = append(affected, id)
                        break
                    }
                }
            }

            if len(affected) == 0 {
                _, _ = fmt.Fprintf(cmd.OutOrStdout(), "no scope entries contain %s\n", path)
                return nil
            }

            // Warn for non-terminal issues that would have empty scope after deletion
            terminalStatuses := map[string]bool{
                ops.StatusMerged:    true,
                ops.StatusDone:      true,
                ops.StatusCancelled: true,
            }
            var emptyAfter []string
            for _, id := range affected {
                issue := state.Issues[id]
                if terminalStatuses[issue.Status] {
                    continue
                }
                remaining := 0
                for _, entry := range issue.Scope {
                    if entry != path {
                        remaining++
                    }
                }
                if remaining == 0 {
                    emptyAfter = append(emptyAfter, id)
                }
            }
            if len(emptyAfter) > 0 {
                _, _ = fmt.Fprintf(cmd.ErrOrStderr(),
                    "WARNING: the following non-terminal issues will have empty scope after deletion: %s\n",
                    strings.Join(emptyAfter, ", "))
            }

            workerID, logPath, err := resolveWorkerAndLog()
            if err != nil {
                return err
            }

            ts := nowEpoch()
            for _, id := range affected {
                op := ops.Op{
                    Type:      ops.OpScopeDelete,
                    TargetID:  id,
                    Timestamp: ts,
                    WorkerID:  workerID,
                    Payload:   ops.Payload{DeletedPath: path},
                }
                if err := appendLowStakesOp(logPath, op); err != nil {
                    return fmt.Errorf("write op for %s: %w", id, err)
                }
            }

            _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted %q from %d issue(s): %s\n",
                path, len(affected), strings.Join(affected, ", "))

            if _, err := materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch"); err != nil {
                return fmt.Errorf("re-materialize: %w", err)
            }
            return nil
        },
    }
}
```

- [ ] **Step 4: Register the command in main.go**

In `cmd/armature/main.go`, add after `scopeRenameCmd` registration:

```go
scopeDeleteCmd := newScopeDeleteCmd()
scopeDeleteCmd.GroupID = "workflow"
root.AddCommand(scopeDeleteCmd)
```

- [ ] **Step 5: Run tests**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run TestScopeDeleteCmd -v
```

Expected: all scope-delete tests pass.

- [ ] **Step 6: Run full test suite**

```bash
cd /home/brian/development/armature && go test ./... 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/armature/scope_delete.go cmd/armature/scope_delete_test.go cmd/armature/main.go
git commit -m "feat: add arm scope-delete command"
```

---

### Task 8: Extend Post-Commit Hook for Rename/Delete Detection

**Files:**
- Modify: `cmd/armature/hook.go`
- Modify: `cmd/armature/hook_test.go`

- [ ] **Step 1: Write failing integration test**

Add to `cmd/armature/hook_test.go`:

```go
// TestHookPostCommit_ScopeRenameDetection verifies post-commit detects file renames
// and emits scope-rename ops for issues whose scope references the old path.
func TestHookPostCommit_ScopeRenameDetection(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

    // arm init
    cmd := newRootCmd()
    cmd.SetOut(new(bytes.Buffer))
    cmd.SetArgs([]string{"init", "--repo", repo})
    require.NoError(t, cmd.Execute())

    // Create task with scope referencing a file
    cmd2 := newRootCmd()
    cmd2.SetOut(new(bytes.Buffer))
    cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Rename test", "--type", "task", "--id", "task-01"})
    require.NoError(t, cmd2.Execute())

    _, err := runTrls(t, repo, "amend", "task-01", "--scope", "old/file.go")
    require.NoError(t, err)

    // Create a commit that renames old/file.go to new/file.go
    require.NoError(t, os.MkdirAll(filepath.Join(repo, "old"), 0755))
    require.NoError(t, os.WriteFile(filepath.Join(repo, "old", "file.go"), []byte("package old\n"), 0644))
    run(t, repo, "git", "add", "old/file.go")
    run(t, repo, "git", "commit", "-m", "add old file")

    require.NoError(t, os.MkdirAll(filepath.Join(repo, "new"), 0755))
    require.NoError(t, os.Rename(filepath.Join(repo, "old", "file.go"), filepath.Join(repo, "new", "file.go")))
    run(t, repo, "git", "add", "-A")
    run(t, repo, "git", "commit", "-m", "rename old to new")

    // Run post-commit hook
    out, err := runTrls(t, repo, "hook", "run", "post-commit")
    require.NoError(t, err)
    _ = out // hook is best-effort; output is informational

    // Verify scope was updated
    ctx := loadTestState(t, repo)
    issue := ctx.Issues["task-01"]
    require.NotNil(t, issue)
    assert.Contains(t, issue.Scope, "new/file.go")
    assert.NotContains(t, issue.Scope, "old/file.go")
}

// TestHookPostCommit_ScopeDeleteDetection verifies post-commit detects file deletions
// and emits scope-delete ops for issues with exact scope entries matching the deleted file.
func TestHookPostCommit_ScopeDeleteDetection(t *testing.T) {
    repo := initTempRepo(t)
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

    cmd := newRootCmd()
    cmd.SetOut(new(bytes.Buffer))
    cmd.SetArgs([]string{"init", "--repo", repo})
    require.NoError(t, cmd.Execute())

    cmd2 := newRootCmd()
    cmd2.SetOut(new(bytes.Buffer))
    cmd2.SetArgs([]string{"create", "--repo", repo, "--title", "Delete test", "--type", "task", "--id", "task-01"})
    require.NoError(t, cmd2.Execute())

    _, err := runTrls(t, repo, "amend", "task-01", "--scope", "old/file.go")
    require.NoError(t, err)

    // Create then delete a file in two commits
    require.NoError(t, os.MkdirAll(filepath.Join(repo, "old"), 0755))
    require.NoError(t, os.WriteFile(filepath.Join(repo, "old", "file.go"), []byte("package old\n"), 0644))
    run(t, repo, "git", "add", "old/file.go")
    run(t, repo, "git", "commit", "-m", "add file")

    require.NoError(t, os.Remove(filepath.Join(repo, "old", "file.go")))
    run(t, repo, "git", "add", "-A")
    run(t, repo, "git", "commit", "-m", "delete file")

    out, err := runTrls(t, repo, "hook", "run", "post-commit")
    require.NoError(t, err)
    _ = out

    ctx := loadTestState(t, repo)
    issue := ctx.Issues["task-01"]
    require.NotNil(t, issue)
    assert.NotContains(t, issue.Scope, "old/file.go")
}

// TestHookPostCommit_InitialCommit_SkipsScopeDetection verifies that on the first commit
// (when HEAD~1 doesn't exist), scope detection is skipped without error.
func TestHookPostCommit_InitialCommit_SkipsScopeDetection(t *testing.T) {
    repo := initTempRepo(t)

    cmd := newRootCmd()
    cmd.SetOut(new(bytes.Buffer))
    cmd.SetArgs([]string{"init", "--repo", repo})
    require.NoError(t, cmd.Execute())

    // Initial commit — HEAD~1 does not exist
    run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

    // Must not error even with no HEAD~1
    _, err := runTrls(t, repo, "hook", "run", "post-commit")
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run to confirm the new tests fail (or the scope detection ones do)**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run "TestHookPostCommit_Scope|TestHookPostCommit_Initial" -v
```

Expected: `ScopeRenameDetection` and `ScopeDeleteDetection` likely pass (hook currently returns without scope detection) but the actual scope change won't be applied. Confirm the scope assertions fail.

- [ ] **Step 3: Implement scope detection in runPostCommitHook**

In `cmd/armature/hook.go`, replace `runPostCommitHook` with:

```go
func runPostCommitHook(cmd *cobra.Command) error {
    // Skip on _armature branch
    branch := hookCurrentBranch()
    if branch == "_armature" {
        return nil
    }

    claimID := hookFindActiveClaimID()
    if claimID != "" {
        workerID, logPath, err := resolveWorkerAndLog()
        if err == nil {
            op := ops.Op{
                Type:      ops.OpHeartbeat,
                TargetID:  claimID,
                Timestamp: nowEpoch(),
                WorkerID:  workerID,
            }
            if err := appendLowStakesOp(logPath, op); err == nil {
                _, _ = fmt.Fprintf(cmd.OutOrStdout(), "Heartbeat recorded for %s\n", claimID)
            }
        }
    }

    // Best-effort scope tracking from most recent commit
    hookDetectScopeChanges(cmd)
    return nil
}

// hookDetectScopeChanges parses the most recent commit for renames and deletions
// and emits scope-rename / scope-delete ops for affected issues.
// All errors are swallowed — this is best-effort and must not block commits.
func hookDetectScopeChanges(cmd *cobra.Command) {
    // Check that HEAD~1 exists (skip on first commit or unborn HEAD)
    checkCmd := exec.Command("git", "-C", appCtx.RepoPath, "rev-parse", "--verify", "HEAD~1")
    if err := checkCmd.Run(); err != nil {
        return // fewer than 2 commits — skip
    }

    state, _, err := materialize.MaterializeAndReturn(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
    if err != nil {
        return
    }

    workerID, logPath, err := resolveWorkerAndLog()
    if err != nil {
        return
    }

    ts := nowEpoch()

    // Detect renames
    renameOut, err := exec.Command("git", "-C", appCtx.RepoPath,
        "diff", "--diff-filter=R", "--name-status", "HEAD~1", "HEAD").Output()
    if err == nil {
        for line := range strings.SplitSeq(strings.TrimSpace(string(renameOut)), "\n") {
            if line == "" {
                continue
            }
            fields := strings.Split(line, "\t")
            if len(fields) < 3 || !strings.HasPrefix(fields[0], "R") {
                continue
            }
            oldPath, newPath := fields[1], fields[2]
            for id, issue := range state.Issues {
                affected := false
                for _, entry := range issue.Scope {
                    if strings.Contains(entry, oldPath) {
                        affected = true
                        break
                    }
                }
                if !affected {
                    continue
                }
                op := ops.Op{
                    Type:      ops.OpScopeRename,
                    TargetID:  id,
                    Timestamp: ts,
                    WorkerID:  workerID,
                    Payload:   ops.Payload{OldPath: oldPath, NewPath: newPath},
                }
                _ = appendLowStakesOp(logPath, op) // best-effort
            }
        }
    }

    // Detect deletions
    deleteOut, err := exec.Command("git", "-C", appCtx.RepoPath,
        "diff", "--diff-filter=D", "--name-status", "HEAD~1", "HEAD").Output()
    if err == nil {
        for line := range strings.SplitSeq(strings.TrimSpace(string(deleteOut)), "\n") {
            if line == "" {
                continue
            }
            fields := strings.Split(line, "\t")
            if len(fields) < 2 {
                continue
            }
            deletedPath := fields[1]
            for id, issue := range state.Issues {
                affected := false
                for _, entry := range issue.Scope {
                    if entry == deletedPath {
                        affected = true
                        break
                    }
                }
                if !affected {
                    continue
                }
                op := ops.Op{
                    Type:      ops.OpScopeDelete,
                    TargetID:  id,
                    Timestamp: ts,
                    WorkerID:  workerID,
                    Payload:   ops.Payload{DeletedPath: deletedPath},
                }
                _ = appendLowStakesOp(logPath, op) // best-effort
            }
        }
    }

    // Rematerialize after any scope ops were written
    _, _ = materialize.Materialize(appCtx.IssuesDir, appCtx.StateDir, appCtx.Mode == "single-branch")
}
```

Check that `materialize` is in the import block of `hook.go`. If not, add `"github.com/scullxbones/armature/internal/materialize"`.

- [ ] **Step 4: Run hook tests**

```bash
cd /home/brian/development/armature && go test ./cmd/armature/... -run "TestHookPostCommit" -v
```

Expected: all hook tests pass including the new scope detection ones.

- [ ] **Step 5: Run full test suite**

```bash
cd /home/brian/development/armature && go test ./... 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/armature/hook.go cmd/armature/hook_test.go
git commit -m "feat: extend post-commit hook to detect scope renames and deletions"
```

---

## Chunk 3: Documentation

### Task 9: Update docs/commands.md

**Files:**
- Modify: `docs/commands.md`

- [ ] **Step 1: Add scope-delete and scope-rename entries between `sources` and `stale-review`**

The spec places both entries between `sources` and `stale-review`. Locate the `## stale-review` heading in `docs/commands.md` and insert before it:

```markdown
## scope-delete

Remove an exact file path from all issue scope entries.

**Synopsis:**
`arm scope-delete <path>`

**Behaviour:**
- Errors if `path` is empty.
- Prints a warning and exits 0 if no scope entries contain `path` exactly.
- Warns to stderr (but proceeds) if any non-terminal issue would have an empty scope after deletion.
- Prints affected issue IDs before writing ops.
- Removes only exact-match entries; glob entries are not removed (use `arm amend --scope` for glob cleanup).

**Example:**
```bash
arm scope-delete cmd/trellis/main.go
```

---

## scope-rename

Rename a path or prefix across all issue scope entries.

**Synopsis:**
`arm scope-rename <old-path> <new-path>`

**Behaviour:**
- Errors if either argument is empty or if `old-path == new-path`.
- Prints a warning and exits 0 if no scope entries contain `old-path` as a substring.
- Prints a summary of affected issues before writing ops.
- Applies `strings.ReplaceAll(entry, old-path, new-path)` to every scope entry, so a directory prefix rename updates both exact paths and glob patterns.
- Idempotent: a second application finds nothing to replace and is a no-op.

**Examples:**
```bash
# Rename a single file
arm scope-rename cmd/trellis/main.go cmd/armature/main.go

# Rename a directory prefix (updates exact paths and globs)
arm scope-rename cmd/trellis cmd/armature
```

---
```

- [ ] **Step 2: Verify placement**

```bash
grep -n "^## " /home/brian/development/armature/docs/commands.md
```

Confirm the order reads: `... sources ... scope-delete ... scope-rename ... stale-review ...`

- [ ] **Step 3: Commit**

```bash
git add docs/commands.md
git commit -m "docs: add scope-delete and scope-rename command reference entries"
```

---

### Task 10: Update Skill Files

**Files:**
- Modify: `skills/armature/SKILL.md`
- Modify: `skills/armature-coordinator/SKILL.md`
- Modify: `skills/armature-planner/SKILL.md`

- [ ] **Step 1: Add Scope Management section to skills/armature/SKILL.md**

Find the `## Repo Health` heading in `skills/armature/SKILL.md` and insert before it:

```markdown
## Scope Management

When files are renamed or deleted, scope entries in existing issues become stale. Two commands handle this:

**`arm scope-rename <old-path> <new-path>`** — substring replacement across all issue scopes. Handles both exact file paths and glob patterns because it uses `strings.ReplaceAll` on each entry. Use when a file or directory is renamed.

**`arm scope-delete <path>`** — removes the exact string from all issue scopes. Glob entries are NOT removed; glob cleanup requires `arm amend --scope`. Use when a file is deleted.

Both commands are no-ops if the path does not appear in any issue scope.

`arm scope-delete` warns to stderr (but proceeds) if any non-terminal active issue would be left with empty scope.

**When to use commands vs. relying on the hook:** The post-commit hook detects renames and deletions automatically from each commit. Run commands directly when:
- Files were renamed/deleted without committing (e.g. in-progress working tree changes).
- The rename or deletion happened during a rebase or amend (hook output is unreliable in those cases).
- You want to batch-rename a directory prefix across all scopes.

```

- [ ] **Step 2: Add scope maintenance block to skills/armature-coordinator/SKILL.md**

Find `## Command Reference` in `skills/armature-coordinator/SKILL.md` and add inside the command reference code block:

```bash
# Scope maintenance (after file renames or deletions)
arm scope-rename <old> <new>    # rewrite path/prefix across all issue scopes
arm scope-delete <path>         # remove exact file path from all issue scopes
```

- [ ] **Step 3: Add scope maintenance block to skills/armature-planner/SKILL.md**

Find `## Quick Reference` in `skills/armature-planner/SKILL.md` and add inside the quick reference code block:

```bash
# Scope maintenance (after refactoring renames or deletions)
arm scope-rename <old-path> <new-path>   # rename path/prefix across all scopes
arm scope-delete <path>                  # remove exact path from all scopes
```

- [ ] **Step 4: Commit**

```bash
git add skills/armature/SKILL.md skills/armature-coordinator/SKILL.md skills/armature-planner/SKILL.md
git commit -m "docs: add scope management section to skill files"
```

---

### Task 11: Final Verification

- [ ] **Step 1: Run make check**

```bash
cd /home/brian/development/armature && make check
```

Expected: all four stages (lint, test, coverage ≥80%, mutate) pass green.

- [ ] **Step 2: Smoke-test the commands manually**

```bash
# In a test repo, verify the commands work end-to-end
arm scope-rename --help
arm scope-delete --help
```

Expected: help text shows correct synopsis and description.

- [ ] **Step 3: Verify W10 narrowing in validate**

```bash
arm validate
```

Confirm that `INFO: phantom scope:` lines no longer appear for issues with status `merged`, `done`, or `cancelled`.
