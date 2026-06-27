package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1: empty dir → returns zero-value Snapshot (no error)
func TestLoad_EmptyDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.NotNil(t, snap.State)
	assert.NotNil(t, snap.Index)
	assert.NotNil(t, snap.Issues)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Equal(t, 0, len(snap.Warnings))
}

// Test 2: single issue → Snapshot has 1 issue, State populated
func TestLoad_SingleIssue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")

	// Op format is a JSON array: [type, target_id, timestamp, worker_id, payload]
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.Equal(t, 1, len(snap.Issues))
	assert.NotNil(t, snap.Issues["issue-1"])
	assert.Equal(t, "issue-1", snap.Issues["issue-1"].ID)
	assert.Equal(t, "Test Issue", snap.Issues["issue-1"].Title)
}

// Test 3: worker-ID mismatch warning → Warnings slice contains warning about mismatched worker IDs
func TestLoad_WorkerIDMismatchWarning(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// File named "alice.log" but ops are from worker "bob"
	logPath := filepath.Join(opsDir, "alice.log")
	opLine := `["create","issue-1",1000,"bob",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	// The op should be rejected due to worker ID mismatch, resulting in a warning
	assert.NotNil(t, snap)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Greater(t, len(snap.Warnings), 0)
	found := false
	for _, w := range snap.Warnings {
		if containsAny(w, "worker", "mismatch") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about worker ID mismatch, got: %v", snap.Warnings)
}

func TestLoad_UnknownOpWarningIncluded(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	logPath := filepath.Join(opsDir, "worker-x.log")
	content := "" +
		`["create","issue-1",1000,"worker-x",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]` + "\n" +
		`["unknown_future_type","issue-1",1001,"worker-x",{}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.Equal(t, 1, len(snap.Issues))
	assert.NotEmpty(t, snap.Warnings)
	found := false
	for _, warning := range snap.Warnings {
		if containsAny(warning, "unknown", "unknown_future_type") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected unknown op warning, got: %v", snap.Warnings)
}

// Test 4: state+issues agreement → Snapshot.State and Snapshot.Issues are consistent
func TestLoad_StateAndIssuesAgreement(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "worker1"
	logPath := filepath.Join(opsDir, workerID+".log")

	op1 := `["create","parent-task",1000,"worker1",{"title":"Parent Task","type":"task","scope":[],"context_files":[]}]`
	op2 := `["create","child-task",1001,"worker1",{"title":"Child Task","parent":"parent-task","type":"task","scope":[],"context_files":[]}]`
	content := op1 + "\n" + op2 + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.Equal(t, 2, len(snap.Issues))
	assert.NotNil(t, snap.Issues["parent-task"])
	assert.NotNil(t, snap.Issues["child-task"])

	assert.Equal(t, 2, len(snap.State.Issues))
	assert.NotNil(t, snap.State.Issues["parent-task"])
	assert.NotNil(t, snap.State.Issues["child-task"])

	parent := snap.Issues["parent-task"]
	child := snap.Issues["child-task"]
	assert.Equal(t, "parent-task", child.Parent)
	assert.Contains(t, parent.Children, "child-task")

	stateParent := snap.State.Issues["parent-task"]
	assert.Equal(t, parent.ID, stateParent.ID)
	assert.Equal(t, parent.Title, stateParent.Title)
}

// Test 5: index populated → Snapshot.Index contains entries for all issues
func TestLoad_IndexPopulated(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "worker1"
	logPath := filepath.Join(opsDir, workerID+".log")

	op1 := `["create","task-1",1000,"worker1",{"title":"Task 1","type":"task","scope":[],"context_files":[]}]`
	op2 := `["create","task-2",1001,"worker1",{"title":"Task 2","type":"task","scope":[],"context_files":[]}]`
	content := op1 + "\n" + op2 + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap.Index)
	assert.Equal(t, 2, len(snap.Index))
	assert.NotNil(t, snap.Index["task-1"])
	assert.NotNil(t, snap.Index["task-2"])
	assert.Equal(t, "Task 1", snap.Index["task-1"].Title)
	assert.Equal(t, "Task 2", snap.Index["task-2"].Title)
}

// Test 6: state and index consistency → Snapshot.State.Issues and Snapshot.Index are consistent
func TestLoad_StateIndexConsistency(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "worker1"
	logPath := filepath.Join(opsDir, workerID+".log")

	op := `["create","issue-1",1000,"worker1",{"title":"Test Issue","type":"task","scope":["file1.txt"],"context_files":["file2.txt"]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(op+"\n"), 0644))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	stateIssue := snap.State.Issues["issue-1"]
	indexEntry := snap.Index["issue-1"]

	assert.NotNil(t, stateIssue)
	assert.NotNil(t, indexEntry)
	assert.Equal(t, stateIssue.ID, "issue-1")
	assert.Equal(t, indexEntry.Title, "Test Issue")
	assert.Equal(t, stateIssue.Type, indexEntry.Type)
	assert.Equal(t, stateIssue.Status, indexEntry.Status)
}

// Test 7: all snapshot fields populated → Snapshot has non-nil State, Index, Issues, and Warnings (even if empty)
func TestLoad_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	snap, err := Load(opsDir, stateDir, false)
	require.NoError(t, err)

	assert.NotNil(t, snap, "Snapshot should not be nil")
	assert.NotNil(t, snap.State, "State should not be nil")
	assert.NotNil(t, snap.Index, "Index should not be nil")
	assert.NotNil(t, snap.Issues, "Issues should not be nil")
	assert.NotNil(t, snap.Warnings, "Warnings should not be nil")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// TestStore_IssueAfterRefresh_REQ_ARCHIMP_S14_T1 tests that Store can load and retrieve
// issues after refresh.
func TestStore_IssueAfterRefresh_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// Create an issue in ops
	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	// Create Store and load
	store := NewStore(opsDir, stateDir, false)
	ctx := context.Background()

	snap, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	// Issue should be accessible
	issue := store.Issue("issue-1")
	require.NotNil(t, issue)
	assert.Equal(t, "issue-1", issue.ID)
	assert.Equal(t, "Test Issue", issue.Title)

	// Refresh should work
	snap2, err := store.Refresh(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap2)

	// Issue should still be accessible after refresh
	issue2 := store.Issue("issue-1")
	require.NotNil(t, issue2)
	assert.Equal(t, "issue-1", issue2.ID)
	assert.Equal(t, "Test Issue", issue2.Title)
}

// TestStore_IssueNotFound_REQ_ARCHIMP_S14_T1 tests that Store.Issue returns nil for
// non-existent issues.
func TestStore_IssueNotFound_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	store := NewStore(opsDir, stateDir, false)
	ctx := context.Background()

	// Before load, Issue should return nil
	issue := store.Issue("nonexistent")
	assert.Nil(t, issue)

	// After load, Issue should still return nil for nonexistent issue
	_, err := store.Load(ctx)
	require.NoError(t, err)

	issue = store.Issue("nonexistent")
	assert.Nil(t, issue)
}

// TestStore_Paths_REQ_ARCHIMP_S14_T1 tests that Store correctly returns filesystem
// paths for issues and index.
func TestStore_Paths_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	store := NewStore(opsDir, stateDir, false)

	// Test IssuePath
	issuePath := store.IssuePath("issue-1")
	expectedIssuePath := filepath.Join(stateDir, "issues", "issue-1.json")
	assert.Equal(t, expectedIssuePath, issuePath)

	// Test IndexPath
	indexPath := store.IndexPath()
	expectedIndexPath := filepath.Join(stateDir, "index.json")
	assert.Equal(t, expectedIndexPath, indexPath)
}

// TestStore_ReadIndex_ReadsFromDiskWithoutMaterialize tests that Store.ReadIndex()
// reads the index from disk without triggering materialization.
func TestStore_ReadIndex_ReadsFromDiskWithoutMaterialize(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(stateDir, 0755))

	// Write a pre-made index.json without materializing
	indexContent := `{
  "task-1": {
    "status": "open",
    "type": "task",
    "updated": 1000,
    "title": "Test Task"
  },
  "task-2": {
    "status": "done",
    "type": "task",
    "updated": 1001,
    "title": "Completed Task"
  }
}`
	indexPath := filepath.Join(stateDir, "index.json")
	require.NoError(t, adapters.WriteFile(indexPath, []byte(indexContent), 0644))

	// Create Store (note: no ops, so Load would fail or behave differently)
	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	store := NewStore(opsDir, stateDir, false)

	// Call ReadIndex without calling Load first
	index, err := store.ReadIndex()
	require.NoError(t, err)

	// Verify index was read correctly from disk
	assert.NotNil(t, index)
	assert.Equal(t, 2, len(index))

	entry1, ok1 := index["task-1"]
	require.True(t, ok1)
	assert.Equal(t, "open", entry1.Status)
	assert.Equal(t, "Test Task", entry1.Title)

	entry2, ok2 := index["task-2"]
	require.True(t, ok2)
	assert.Equal(t, "done", entry2.Status)
	assert.Equal(t, "Completed Task", entry2.Title)

	// ReadIndex must not trigger materialization: checkpoint.json and issues/ must be absent.
	// If materialize.MaterializeAndReturn* had been called it would have written these paths.
	assert.NoFileExists(t, filepath.Join(stateDir, "checkpoint.json"),
		"ReadIndex must not write checkpoint.json (materialization must not occur)")
	issuesDir := filepath.Join(stateDir, "issues")
	if entries, err := os.ReadDir(issuesDir); err == nil {
		assert.Empty(t, entries, "ReadIndex must not populate the issues/ directory (materialization must not occur)")
	}
	// os.ReadDir returning an error means the directory doesn't exist, which is also acceptable.
}
