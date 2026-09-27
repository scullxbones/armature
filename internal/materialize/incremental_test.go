package materialize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplayCostBoundedWithNOneRunLogs_REQ_CLAIMORD_W14(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ops"), 0o755))
	const n = 8
	for i := 0; i < n; i++ {
		wid := "cattle-" + string(rune('a'+i))
		id := "task-0" + string(rune('1'+i))
		writeMaterializeOpLog(t, dir, "ops/"+wid+".log", []ops.Op{
			{Type: ops.OpCreate, TargetID: id, Timestamp: int64(100 + i), WorkerID: wid,
				Payload: ops.Payload{Title: "T" + wid, NodeType: "task"}},
		})
		gittest.Git(t, dir, "add", "ops/"+wid+".log")
		gittest.Git(t, dir, "commit", "-m", "log "+wid)
	}
	stateDir := t.TempDir()
	first, firstRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, firstRes.FullReplay)
	assert.Equal(t, n, firstRes.OpsProcessed)
	assert.Equal(t, n, len(first.Issues))
	cp, err := LoadCheckpoint(filepath.Join(stateDir, "checkpoint.json"))
	require.NoError(t, err)
	require.NotEmpty(t, cp.LastCommitSHA)

	writeMaterializeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "Tcattle-a", NodeType: "task"}},
		{Type: ops.OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
			Payload: ops.Payload{Msg: "new"}},
	})
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "one new op")

	second, secondRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.False(t, secondRes.FullReplay)
	assert.Equal(t, 1, secondRes.OpsProcessed, "incremental must apply only the new line, not N cattle logs")
	assert.Equal(t, "new", second.Issues["task-01"].Notes[0].Msg)

	coldDir := t.TempDir()
	cold, coldRes, err := Run(coldDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, coldRes.FullReplay)
	assert.Greater(t, coldRes.OpsProcessed, n)
	gotInc, err := json.Marshal(issueDigest(second))
	require.NoError(t, err)
	gotCold, err := json.Marshal(issueDigest(cold))
	require.NoError(t, err)
	assert.JSONEq(t, string(gotCold), string(gotInc), "cold walk and incremental state must match")
}

func TestIncrementalReplayAppliesUncommittedDiskOps_REQ_CLAIMORD_W14(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ops"), 0o755))
	writeMaterializeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	})
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "create")

	stateDir := t.TempDir()
	_, _, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)

	require.NoError(t, ops.AppendOp(filepath.Join(dir, "ops", "cattle-a.log"), ops.Op{
		Type: ops.OpNote, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
		Payload: ops.Payload{Msg: "disk-only"},
	}))

	second, secondRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.False(t, secondRes.FullReplay)
	assert.Equal(t, 1, secondRes.OpsProcessed)
	require.NotEmpty(t, second.Issues["task-01"].Notes)
	assert.Equal(t, "disk-only", second.Issues["task-01"].Notes[0].Msg)

	third, thirdRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.Equal(t, 0, thirdRes.OpsProcessed, "already-applied disk ops must not replay every run")
	assert.Equal(t, "disk-only", third.Issues["task-01"].Notes[0].Msg)
}

func TestCommitIncremental_MissingCheckpointSHAForcesColdReplay_REQ_CLAIMORD_W14(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	fx := gittest.InitWithOrigin(t)
	dir := fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ops"), 0o755))
	writeMaterializeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"src/foo.go"}}},
	})
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "create")

	stateDir := t.TempDir()
	_, firstRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, firstRes.FullReplay)

	writeMaterializeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task", Scope: []string{"src/foo.go"}}},
		{Type: ops.OpScopeRename, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
			Payload: ops.Payload{OldPath: "src", NewPath: "src2"}},
	})
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "rename src to src2")

	second, secondRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.False(t, secondRes.FullReplay)
	require.Equal(t, []string{"src2/foo.go"}, second.Issues["task-01"].Scope)

	gittest.Git(t, dir, "commit", "--amend", "-m", "rename src to src2 amended")

	third, thirdRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, thirdRes.FullReplay, "absent checkpoint SHA must force a cold replay")
	require.Equal(t, []string{"src2/foo.go"}, third.Issues["task-01"].Scope,
		"cold replay must not re-apply scope-rename on cached src2 (would become src22)")
}

func issueDigest(state *State) map[string]string {
	out := make(map[string]string, len(state.Issues))
	for id, issue := range state.Issues {
		out[id] = issue.Title + "|" + issue.Status
		if len(issue.Notes) > 0 {
			out[id] += "|" + issue.Notes[len(issue.Notes)-1].Msg
		}
	}
	return out
}

func writeMaterializeOpLog(t *testing.T, repo, rel string, log []ops.Op) {
	t.Helper()
	path := filepath.Join(repo, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	for _, op := range log {
		require.NoError(t, ops.AppendOp(path, op))
	}
}
