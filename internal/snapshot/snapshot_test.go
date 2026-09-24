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

func loadSnap(opsDir, stateDir string) (*Snapshot, error) {
	return NewStore(opsDir, stateDir).Load(context.Background())
}

func TestLoad_EmptyDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.NotNil(t, snap.State)
	assert.NotNil(t, snap.Index)
	assert.NotNil(t, snap.Issues)
	assert.NotNil(t, snap.MaterializedOps)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Equal(t, 0, len(snap.MaterializedOps))
	assert.Equal(t, 0, len(snap.Warnings))
}

func TestLoad_SingleIssue(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")

	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.Equal(t, 1, len(snap.Issues))
	assert.NotNil(t, snap.Issues["issue-1"])
	assert.Equal(t, "issue-1", snap.Issues["issue-1"].ID)
	assert.Equal(t, "Test Issue", snap.Issues["issue-1"].Title)
	require.Len(t, snap.MaterializedOps, 1)
	assert.Equal(t, "issue-1", snap.MaterializedOps[0].TargetID)
}

func TestLoad_WorkerIDMismatchWarning(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	logPath := filepath.Join(opsDir, "alice.log")
	opLine := `["create","issue-1",1000,"bob",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.Equal(t, 0, len(snap.Issues))
	assert.Empty(t, snap.MaterializedOps, "mismatched ops must be excluded from the captured set")
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

	snap, err := loadSnap(opsDir, stateDir)
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

	snap, err := loadSnap(opsDir, stateDir)
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

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)

	assert.NotNil(t, snap.Index)
	assert.Equal(t, 2, len(snap.Index))
	assert.NotNil(t, snap.Index["task-1"])
	assert.NotNil(t, snap.Index["task-2"])
	assert.Equal(t, "Task 1", snap.Index["task-1"].Title)
	assert.Equal(t, "Task 2", snap.Index["task-2"].Title)
}

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

	snap, err := loadSnap(opsDir, stateDir)
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

func TestLoad_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)

	assert.NotNil(t, snap, "Snapshot should not be nil")
	assert.NotNil(t, snap.State, "State should not be nil")
	assert.NotNil(t, snap.Index, "Index should not be nil")
	assert.NotNil(t, snap.Issues, "Issues should not be nil")
	assert.NotNil(t, snap.MaterializedOps, "MaterializedOps should not be nil")
	assert.NotNil(t, snap.Warnings, "Warnings should not be nil")
}

func TestLoad_CapturedOpsDoNotIncludeLaterAppends(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	logPath := filepath.Join(opsDir, "test-worker.log")
	first := `["create","issue-1",1000,"test-worker",{"title":"First","type":"task","scope":[],"context_files":[]}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(first), 0644))

	snap, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)
	require.Len(t, snap.MaterializedOps, 1)
	require.Contains(t, snap.Issues, "issue-1")

	second := first + `["create","issue-2",2000,"test-worker",{"title":"Second","type":"task","scope":[],"context_files":[]}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(second), 0644))

	assert.Len(t, snap.MaterializedOps, 1, "captured op set must stay frozen after later appends")
	assert.NotContains(t, snap.Issues, "issue-2", "snapshot hierarchy must match the captured ops, not a later log read")

	snap2, err := loadSnap(opsDir, stateDir)
	require.NoError(t, err)
	require.Len(t, snap2.MaterializedOps, 2)
	assert.Contains(t, snap2.Issues, "issue-2")
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

func TestStore_IssueAfterRefresh_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	store := NewStore(opsDir, stateDir)
	ctx := context.Background()

	snap, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	issue := store.Issue("issue-1")
	require.NotNil(t, issue)
	assert.Equal(t, "issue-1", issue.ID)
	assert.Equal(t, "Test Issue", issue.Title)

	snap2, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap2)

	issue2 := store.Issue("issue-1")
	require.NotNil(t, issue2)
	assert.Equal(t, "issue-1", issue2.ID)
	assert.Equal(t, "Test Issue", issue2.Title)
}

func TestStore_IssueNotFound_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	store := NewStore(opsDir, stateDir)
	ctx := context.Background()

	issue := store.Issue("nonexistent")
	assert.Nil(t, issue)

	_, err := store.Load(ctx)
	require.NoError(t, err)

	issue = store.Issue("nonexistent")
	assert.Nil(t, issue)
}

func TestStore_Paths_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	store := NewStore(opsDir, stateDir)

	issuePath := store.IssuePath("issue-1")
	expectedIssuePath := filepath.Join(stateDir, "issues", "issue-1.json")
	assert.Equal(t, expectedIssuePath, issuePath)

	indexPath := store.IndexPath()
	expectedIndexPath := filepath.Join(stateDir, "index.json")
	assert.Equal(t, expectedIndexPath, indexPath)
}

func TestStore_ReadIndex_ReadsFromDiskWithoutMaterialize(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(stateDir, 0755))

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

	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	store := NewStore(opsDir, stateDir)

	index, err := store.ReadIndex()
	require.NoError(t, err)

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

	assert.NoFileExists(t, filepath.Join(stateDir, "checkpoint.json"),
		"ReadIndex must not write checkpoint.json (materialization must not occur)")
	issuesDir := filepath.Join(stateDir, "issues")
	if entries, err := os.ReadDir(issuesDir); err == nil {
		assert.Empty(t, entries, "ReadIndex must not populate the issues/ directory (materialization must not occur)")
	}
}

func TestStore_ReadIssue_ReadsFromDiskWithoutMaterialize(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	issuesDir := filepath.Join(stateDir, "issues")

	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	issueContent := `{
  "id": "task-1",
  "type": "task",
  "status": "open",
  "title": "Test Issue",
  "scope": ["file1.txt"],
  "children": [],
  "blocked_by": [],
  "blocks": [],
  "decision_refs": [],
  "source_links": [],
  "citation_acceptances": [],
  "notes": [],
  "decisions": [],
  "provenance": {
    "method": "test",
    "confidence": "high",
    "source_worker": "test-worker"
  },
  "updated": 1000
}`
	issuePath := filepath.Join(issuesDir, "task-1.json")
	require.NoError(t, adapters.WriteFile(issuePath, []byte(issueContent), 0644))

	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	store := NewStore(opsDir, stateDir)

	issue, err := store.ReadIssue("task-1")
	require.NoError(t, err)

	assert.NotNil(t, issue)
	assert.Equal(t, "task-1", issue.ID)
	assert.Equal(t, "Test Issue", issue.Title)
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "task", issue.Type)

	assert.NoFileExists(t, filepath.Join(stateDir, "checkpoint.json"),
		"ReadIssue must not write checkpoint.json (materialization must not occur)")
}

func TestStore_ReadIssue_NotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	issuesDir := filepath.Join(stateDir, "issues")

	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0755))

	store := NewStore(opsDir, stateDir)

	_, err := store.ReadIssue("nonexistent")
	assert.Error(t, err, "ReadIssue should return error for non-existent issue")
}

func TestSnapshotCurrentTruthAccess_REQ_ARCHIMP_S18_T3(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	require.NoError(t, os.MkdirAll(opsDir, 0755))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	store := NewStore(opsDir, stateDir)
	ctx := context.Background()

	issue := store.Issue("initial-id")
	assert.Nil(t, issue, "Issue() before Load() must return nil")

	index := store.Index()
	assert.Empty(t, index, "Index() before Load() must return empty")

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")
	opLine := `["create","task-1",1000,"test-worker",{"title":"Task One","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0644))

	snap1, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap1)
	assert.Equal(t, 1, len(snap1.Issues))

	issue1 := store.Issue("task-1")
	require.NotNil(t, issue1)
	assert.Equal(t, "task-1", issue1.ID)
	assert.Equal(t, "Task One", issue1.Title)

	index1 := store.Index()
	require.NotEmpty(t, index1)
	assert.Equal(t, 1, len(index1))
	assert.NotNil(t, index1["task-1"])
	assert.Equal(t, "Task One", index1["task-1"].Title)

	issue2 := store.Issue("task-1")
	require.NotNil(t, issue2)
	assert.Equal(t, "task-1", issue2.ID)
	assert.Equal(t, issue1.ID, issue2.ID)
	assert.Equal(t, issue1.Title, issue2.Title)

	index2 := store.Index()
	assert.Equal(t, len(index1), len(index2))

	opLine2 := `["create","task-2",1001,"test-worker",{"title":"Task Two","type":"task","scope":[],"context_files":[]}]`
	content := opLine + "\n" + opLine2 + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0644))

	snap2, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap2)
	assert.Equal(t, 2, len(snap2.Issues), "after Refresh(), snapshot should have 2 issues")

	issue3 := store.Issue("task-2")
	require.NotNil(t, issue3, "after Refresh(), Issue() must return the newly added issue")
	assert.Equal(t, "task-2", issue3.ID)
	assert.Equal(t, "Task Two", issue3.Title)

	index3 := store.Index()
	assert.Equal(t, 2, len(index3), "after Refresh(), Index() must reflect the updated snapshot")
	assert.NotNil(t, index3["task-2"])
	assert.Equal(t, "Task Two", index3["task-2"].Title)
}
