package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hostRefURLA = "https://github.com/octocat/hello-world/issues/1"

func committedOpsLayout(t *testing.T) (worktree, opsDir, stateDir string) {
	t.Helper()
	worktree = gittest.Init(t)
	opsDir = filepath.Join(worktree, "ops")
	stateDir = t.TempDir()
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	return worktree, opsDir, stateDir
}

func commitOps(t *testing.T, worktree string, msg string) {
	t.Helper()
	gittest.Git(t, worktree, "add", "ops")
	gittest.Git(t, worktree, "commit", "-m", msg)
}

func pinOporderCutoverAtHEAD(t *testing.T, worktree string) {
	t.Helper()
	sha, err := adapters.New(worktree).HeadSHA()
	require.NoError(t, err)
	require.NotEmpty(t, sha)
	require.NoError(t, gittest.SetLocalConfig(worktree, "armature.oporder-cutover", sha))
}

func loadSnap(opsDir, stateDir, worktree string) (*Snapshot, error) {
	return NewStore(opsDir, stateDir).WithOpsWorktree(worktree).Load(context.Background())
}

func TestLoad_RequiresOpsWorktree_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	_, err := NewStore(opsDir, stateDir).Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops worktree is required")
}

func TestLoad_EmptyDir(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	snap, err := loadSnap(opsDir, stateDir, worktree)
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

func TestLoad_EmptyStateDirRefusesCwdMaterialization(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	_, err := NewStore(opsDir, "").WithOpsWorktree(tmpDir).Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state dir is required")
	assert.Contains(t, err.Error(), "refusing cwd materialization")
}

func TestLoad_SingleIssue(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	workerID := "test-worker"
	logPath := filepath.Join(opsDir, workerID+".log")
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0o644))
	commitOps(t, worktree, "create issue-1")

	snap, err := loadSnap(opsDir, stateDir, worktree)
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
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "alice.log")
	opLine := `["create","issue-1",1000,"bob",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0o644))
	commitOps(t, worktree, "mismatch")

	snap, err := loadSnap(opsDir, stateDir, worktree)
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
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "worker-x.log")
	content := "" +
		`["create","issue-1",1000,"worker-x",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]` + "\n" +
		`["unknown_future_type","issue-1",1001,"worker-x",{}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(content), 0o644))
	commitOps(t, worktree, "unknown op")

	snap, err := loadSnap(opsDir, stateDir, worktree)
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
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "worker1.log")
	op1 := `["create","parent-task",1000,"worker1",{"title":"Parent Task","type":"task","scope":[],"context_files":[]}]`
	op2 := `["create","child-task",1001,"worker1",{"title":"Child Task","parent":"parent-task","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(op1+"\n"+op2+"\n"), 0o644))
	commitOps(t, worktree, "parent child")

	snap, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)

	assert.Equal(t, 2, len(snap.Issues))
	assert.NotNil(t, snap.Issues["parent-task"])
	assert.NotNil(t, snap.Issues["child-task"])

	assert.Equal(t, 2, len(snap.State.Issues))
	parent := snap.Issues["parent-task"]
	child := snap.Issues["child-task"]
	assert.Equal(t, "parent-task", child.Parent)
	assert.Contains(t, parent.Children, "child-task")
}

func TestLoad_IndexPopulated(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "worker1.log")
	op1 := `["create","task-1",1000,"worker1",{"title":"Task 1","type":"task","scope":[],"context_files":[]}]`
	op2 := `["create","task-2",1001,"worker1",{"title":"Task 2","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(op1+"\n"+op2+"\n"), 0o644))
	commitOps(t, worktree, "two tasks")

	snap, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)

	assert.NotNil(t, snap.Index)
	assert.Equal(t, 2, len(snap.Index))
	assert.Equal(t, "Task 1", snap.Index["task-1"].Title)
	assert.Equal(t, "Task 2", snap.Index["task-2"].Title)
}

func TestLoad_StateIndexConsistency(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "worker1.log")
	op := `["create","issue-1",1000,"worker1",{"title":"Test Issue","type":"task","scope":["file1.txt"],"context_files":["file2.txt"]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(op+"\n"), 0o644))
	commitOps(t, worktree, "issue-1")

	snap, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)

	stateIssue := snap.State.Issues["issue-1"]
	indexEntry := snap.Index["issue-1"]
	assert.Equal(t, stateIssue.Type, indexEntry.Type)
	assert.Equal(t, stateIssue.Status, indexEntry.Status)
}

func TestLoad_AllFieldsPopulated(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	snap, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)

	assert.NotNil(t, snap)
	assert.NotNil(t, snap.State)
	assert.NotNil(t, snap.Index)
	assert.NotNil(t, snap.Issues)
	assert.NotNil(t, snap.MaterializedOps)
	assert.NotNil(t, snap.Warnings)
}

func TestLoad_CapturedOpsDoNotIncludeLaterAppends(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "test-worker.log")
	first := `["create","issue-1",1000,"test-worker",{"title":"First","type":"task","scope":[],"context_files":[]}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(first), 0o644))
	commitOps(t, worktree, "first")

	snap, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)
	require.Len(t, snap.MaterializedOps, 1)
	require.Contains(t, snap.Issues, "issue-1")

	second := first + `["create","issue-2",2000,"test-worker",{"title":"Second","type":"task","scope":[],"context_files":[]}]` + "\n"
	require.NoError(t, adapters.WriteFile(logPath, []byte(second), 0o644))

	assert.Len(t, snap.MaterializedOps, 1, "captured op set must stay frozen after later appends")
	assert.NotContains(t, snap.Issues, "issue-2", "snapshot hierarchy must match the captured ops, not a later log read")

	snap2, err := loadSnap(opsDir, stateDir, worktree)
	require.NoError(t, err)
	require.Len(t, snap2.MaterializedOps, 2)
	assert.Contains(t, snap2.Issues, "issue-2")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestStore_IssueAfterRefresh_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	logPath := filepath.Join(opsDir, "test-worker.log")
	opLine := `["create","issue-1",1000,"test-worker",{"title":"Test Issue","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0o644))
	commitOps(t, worktree, "issue-1")

	store := NewStore(opsDir, stateDir).WithOpsWorktree(worktree)
	ctx := context.Background()

	snap, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	issue := store.Issue("issue-1")
	require.NotNil(t, issue)
	assert.Equal(t, "Test Issue", issue.Title)

	snap2, err := store.Load(ctx)
	require.NoError(t, err)
	require.NotNil(t, snap2)
	assert.Equal(t, "Test Issue", store.Issue("issue-1").Title)
}

func TestStore_IssueNotFound_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	store := NewStore(opsDir, stateDir).WithOpsWorktree(worktree)
	assert.Nil(t, store.Issue("nonexistent"))

	_, err := store.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, store.Issue("nonexistent"))
}

func TestStore_Paths_REQ_ARCHIMP_S14_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	opsDir := filepath.Join(tmpDir, "ops")
	stateDir := filepath.Join(tmpDir, "state")

	store := NewStore(opsDir, stateDir)
	assert.Equal(t, filepath.Join(stateDir, "issues", "issue-1.json"), store.IssuePath("issue-1"))
	assert.Equal(t, filepath.Join(stateDir, "index.json"), store.IndexPath())
}

func TestStore_ReadIndex_ReadsFromDiskWithoutMaterialize(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

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
	require.NoError(t, adapters.WriteFile(filepath.Join(stateDir, "index.json"), []byte(indexContent), 0o644))
	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	store := NewStore(opsDir, stateDir)
	index, err := store.ReadIndex()
	require.NoError(t, err)
	assert.Equal(t, 2, len(index))
	assert.NoFileExists(t, filepath.Join(stateDir, "checkpoint.json"))
}

func TestStore_ReadIssue_ReadsFromDiskWithoutMaterialize(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	issuesDir := filepath.Join(stateDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

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
	require.NoError(t, adapters.WriteFile(filepath.Join(issuesDir, "task-1.json"), []byte(issueContent), 0o644))
	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	store := NewStore(opsDir, stateDir)
	issue, err := store.ReadIssue("task-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Issue", issue.Title)
	assert.NoFileExists(t, filepath.Join(stateDir, "checkpoint.json"))
}

func TestStore_ReadIssue_NotFound(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	require.NoError(t, os.MkdirAll(filepath.Join(stateDir, "issues"), 0o755))
	opsDir := filepath.Join(tmpDir, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))

	_, err := NewStore(opsDir, stateDir).ReadIssue("nonexistent")
	assert.Error(t, err)
}

func TestSnapshotCurrentTruthAccess_REQ_ARCHIMP_S18_T3(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")

	store := NewStore(opsDir, stateDir).WithOpsWorktree(worktree)
	ctx := context.Background()
	assert.Nil(t, store.Issue("initial-id"))
	assert.Empty(t, store.Index())

	logPath := filepath.Join(opsDir, "test-worker.log")
	opLine := `["create","task-1",1000,"test-worker",{"title":"Task One","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"), 0o644))
	commitOps(t, worktree, "task-1")

	snap1, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(snap1.Issues))
	assert.Equal(t, "Task One", store.Issue("task-1").Title)

	opLine2 := `["create","task-2",1001,"test-worker",{"title":"Task Two","type":"task","scope":[],"context_files":[]}]`
	require.NoError(t, adapters.WriteFile(logPath, []byte(opLine+"\n"+opLine2+"\n"), 0o644))
	commitOps(t, worktree, "task-2")

	snap2, err := store.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, len(snap2.Issues))
	assert.Equal(t, "Task Two", store.Issue("task-2").Title)
}

func TestLoadReadOnly_ClockSkewedWithdrawMatchesCold_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")
	pinOporderCutoverAtHEAD(t, worktree)
	logPath := filepath.Join(opsDir, "cattle-a.log")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	commitOps(t, worktree, "create")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 300, WorkerID: "cattle-a",
		Payload: ops.Payload{ExternalRef: hostRefURLA, ExternalRole: "engineering"},
	}))
	commitOps(t, worktree, "add A ts300")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
		Payload: ops.Payload{WithdrawExternalRef: hostRefURLA},
	}))
	commitOps(t, worktree, "withdraw A ts200")

	before := stateTreeDigest(t, stateDir)
	store := NewStore(opsDir, stateDir).WithOpsWorktree(worktree)
	snap, err := store.LoadReadOnly(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, stateTreeDigest(t, stateDir), "LoadReadOnly must write no state files")

	issue := snap.Issues["task-01"]
	require.NotNil(t, issue)
	require.Equal(t, []materialize.HostRef{
		{URL: hostRefURLA, Role: "engineering", At: 300, WorkerID: "cattle-a"},
		{URL: hostRefURLA, At: 200, WorkerID: "cattle-a", Withdrawn: true},
	}, issue.HostRefs)
	_, ok := materialize.CurrentHostRef(issue.HostRefs)
	assert.False(t, ok, "committed withdraw after add must leave no live A despite ts200 < ts300")

	coldDir := t.TempDir()
	cold, _, err := materialize.Run(coldDir, nil, nil, materialize.Options{
		WriteStateFiles: true,
		OpsWorktree:     worktree,
	})
	require.NoError(t, err)
	require.Equal(t, cold.Issues["task-01"].HostRefs, issue.HostRefs)
}

func TestLoad_ClockSkewedWithdrawWritePathMatchesReadOnly_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	worktree, opsDir, stateDir := committedOpsLayout(t)
	gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")
	pinOporderCutoverAtHEAD(t, worktree)
	logPath := filepath.Join(opsDir, "cattle-a.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	commitOps(t, worktree, "create")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 300, WorkerID: "cattle-a",
		Payload: ops.Payload{ExternalRef: hostRefURLA},
	}))
	commitOps(t, worktree, "add")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
		Payload: ops.Payload{WithdrawExternalRef: hostRefURLA},
	}))
	commitOps(t, worktree, "withdraw")

	store := NewStore(opsDir, stateDir).WithOpsWorktree(worktree)
	ro, err := store.LoadReadOnly(context.Background())
	require.NoError(t, err)
	rw, err := store.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, ro.Issues["task-01"].HostRefs, rw.Issues["task-01"].HostRefs)
	_, ok := materialize.CurrentHostRef(rw.Issues["task-01"].HostRefs)
	assert.False(t, ok)
}

func FuzzLoadReadOnlyHostRefCommitOrder_REQ_HOSTREF_S1(f *testing.F) {
	f.Add([]byte{0, 1, 2})
	f.Add([]byte{0, 2, 1, 3})
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) == 0 || len(script) > 12 {
			t.Skip()
		}
		worktree, opsDir, stateDir := committedOpsLayout(t)
		gittest.Git(t, worktree, "commit", "--allow-empty", "-m", "init")
		pinOporderCutoverAtHEAD(t, worktree)
		logPath := filepath.Join(opsDir, "cattle-a.log")
		require.NoError(t, ops.AppendOp(logPath, ops.Op{
			Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"},
		}))
		commitOps(t, worktree, "create")

		ts := int64(200)
		for i, raw := range script {
			ts += 10
			skewed := ts
			if raw%2 == 1 {
				skewed = ts - 50
			}
			switch raw % 3 {
			case 0:
				require.NoError(t, ops.AppendOp(logPath, ops.Op{
					Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: skewed, WorkerID: "cattle-a",
					Payload: ops.Payload{ExternalRef: hostRefURLA, ExternalRole: "engineering"},
				}))
			case 1:
				require.NoError(t, ops.AppendOp(logPath, ops.Op{
					Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: skewed, WorkerID: "cattle-a",
					Payload: ops.Payload{WithdrawExternalRef: hostRefURLA},
				}))
			default:
				require.NoError(t, ops.AppendOp(logPath, ops.Op{
					Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: skewed, WorkerID: "cattle-a",
					Payload: ops.Payload{ExternalRef: hostRefURLA, ExternalRole: "support"},
				}))
			}
			commitOps(t, worktree, "step")
			_ = i
		}

		snap, err := NewStore(opsDir, stateDir).WithOpsWorktree(worktree).LoadReadOnly(context.Background())
		require.NoError(t, err)
		coldDir := t.TempDir()
		cold, _, err := materialize.Run(coldDir, nil, nil, materialize.Options{
			WriteStateFiles: true,
			OpsWorktree:     worktree,
		})
		require.NoError(t, err)
		require.Equal(t, cold.Issues["task-01"].HostRefs, snap.Issues["task-01"].HostRefs)
	})
}

func stateTreeDigest(t *testing.T, root string) string {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "ABSENT"
		}
		require.NoError(t, err)
	}
	require.True(t, info.IsDir())
	var lines []string
	fsys := os.DirFS(root)
	err = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			lines = append(lines, "D|"+filepath.ToSlash(rel))
			return nil
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, "F|"+filepath.ToSlash(rel)+"|"+hex.EncodeToString(sum[:]))
		return nil
	})
	require.NoError(t, err)
	return strings.Join(lines, "\n")
}
