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

func TestIncremental_PendingExternalRefThenCommitMatchesCold_REQ_HOSTREF_S1_T2(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
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
	_, firstRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, firstRes.FullReplay)

	logPath := filepath.Join(dir, "ops", "cattle-a.log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "engineering"},
	}))
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 201, WorkerID: "cattle-a",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "engineering"},
	}))

	pending, pendingRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.False(t, pendingRes.FullReplay)
	assert.Equal(t, 2, pendingRes.OpsProcessed)
	require.Equal(t, []HostRef{
		{URL: hostURLA, Role: "engineering", At: 200, WorkerID: "cattle-a"},
		{URL: hostURLA, Role: "engineering", At: 201, WorkerID: "cattle-a"},
	}, pending.Issues["task-01"].HostRefs)

	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "external-ref")

	afterCommit, afterRes, err := Run(stateDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.False(t, afterRes.FullReplay)

	coldDir := t.TempDir()
	cold, coldRes, err := Run(coldDir, nil, nil, Options{WriteStateFiles: true, OpsWorktree: dir})
	require.NoError(t, err)
	assert.True(t, coldRes.FullReplay)

	require.Equal(t, cold.Issues["task-01"].HostRefs, afterCommit.Issues["task-01"].HostRefs,
		"pending then committed HostRefs must equal cold replay; identical URL/role events stay distinct")
	assert.Equal(t, 0, afterRes.OpsProcessed, "pending that later commits must not be applied a second time")

	incIdx, err := LoadIndex(filepath.Join(stateDir, "index.json"))
	require.NoError(t, err)
	coldIdx, err := LoadIndex(filepath.Join(coldDir, "index.json"))
	require.NoError(t, err)
	assert.Equal(t, coldIdx["task-01"].CurrentHostURL, incIdx["task-01"].CurrentHostURL)
	assert.Equal(t, coldIdx["task-01"].LiveHostURLs, incIdx["task-01"].LiveHostURLs)
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

const (
	hostRefActARecordA byte = iota
	hostRefActARecordB
	hostRefActAWithdrawA
	hostRefActAWithdrawB
	hostRefActACommit
	hostRefActBRecordA
	hostRefActBRecordB
	hostRefActBWithdrawA
	hostRefActBWithdrawB
	hostRefActBCommit
	hostRefActCount
)

func TestIncremental_TwoWorkerPendingRecordThenOtherCommitMatchesCold_REQ_HOSTREF_S1_T2(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	driveTwoWorkerHostRefScript(t, []byte{
		hostRefActARecordA,
		hostRefActBRecordB,
		hostRefActBCommit,
		hostRefActACommit,
	})
}

func TestIncremental_TwoWorkerPendingWithdrawThenOtherRecordMatchesCold_REQ_HOSTREF_S1_T2(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	driveTwoWorkerHostRefScript(t, []byte{
		hostRefActARecordA,
		hostRefActACommit,
		hostRefActAWithdrawA,
		hostRefActBRecordA,
		hostRefActBCommit,
		hostRefActACommit,
	})
}

func TestProperty_TwoWorkerHostRefInterleavingsMatchCold_REQ_HOSTREF_S1_T2(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	scripts := [][]byte{
		{hostRefActARecordA, hostRefActBRecordB, hostRefActBCommit, hostRefActACommit},
		{hostRefActARecordA, hostRefActACommit, hostRefActAWithdrawA, hostRefActBRecordA, hostRefActBCommit, hostRefActACommit},
		{hostRefActARecordA, hostRefActACommit, hostRefActBRecordB, hostRefActBCommit},
		{hostRefActBRecordA, hostRefActBCommit, hostRefActARecordB, hostRefActACommit, hostRefActBWithdrawA, hostRefActBCommit},
	}
	for seed := uint64(1); seed <= 16; seed++ {
		scripts = append(scripts, hostRefScriptFromSeed(seed, 6))
	}
	for i, script := range scripts {
		t.Logf("script%d", i)
		driveTwoWorkerHostRefScript(t, script)
	}
}

func FuzzTwoWorkerHostRefInterleavingMatchesCold_REQ_HOSTREF_S1_T2(f *testing.F) {
	f.Add([]byte{hostRefActARecordA, hostRefActBRecordB, hostRefActBCommit, hostRefActACommit})
	f.Add([]byte{hostRefActARecordA, hostRefActACommit, hostRefActAWithdrawA, hostRefActBRecordA, hostRefActBCommit, hostRefActACommit})
	f.Fuzz(func(t *testing.T, script []byte) {
		if len(script) == 0 {
			t.Skip()
		}
		if len(script) > 8 {
			script = script[:8]
		}
		driveTwoWorkerHostRefScript(t, script)
	})
}

func TestIncremental_V1CheckpointWithoutHostRefsRebuilds_REQ_HOSTREF_S1_T2(t *testing.T) { //nolint:paralleltest // gittest.IsolateGit
	dir, stateDir := twoWorkerHostRefFixture(t)
	require.NoError(t, ops.AppendOp(filepath.Join(dir, "ops", "cattle-a.log"), ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "cattle-a",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "engineering"},
	}))
	gittest.Git(t, dir, "add", "ops/cattle-a.log")
	gittest.Git(t, dir, "commit", "-m", "external-ref")

	_, firstRes, err := Run(stateDir, nil, nil, hostRefRunOpts(dir))
	require.NoError(t, err)
	require.True(t, firstRes.FullReplay)

	issuePath := filepath.Join(stateDir, "issues", "task-01.json")
	issue, err := LoadIssue(issuePath)
	require.NoError(t, err)
	require.NotEmpty(t, issue.HostRefs)
	issue.HostRefs = nil
	require.NoError(t, WriteIssue(filepath.Join(stateDir, "issues"), issue))

	cp, err := LoadCheckpoint(filepath.Join(stateDir, "checkpoint.json"))
	require.NoError(t, err)
	raw, err := json.Marshal(Checkpoint{
		LastCommitSHA: cp.LastCommitSHA,
		StateVersion:  1,
		ByteOffsets:   cp.ByteOffsets,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "checkpoint.json"), raw, 0o644))

	res := requireHostRefIncEqualsCold(t, dir, stateDir)
	assert.True(t, res.FullReplay, "a v1 snapshot without HostRefs must rebuild from committed history")
}

func twoWorkerHostRefFixture(t *testing.T) (dir, stateDir string) {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	dir = fx.Dir
	gittest.Git(t, dir, "commit", "--allow-empty", "-m", "init")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ops"), 0o755))
	writeMaterializeOpLog(t, dir, "ops/cattle-a.log", []ops.Op{
		{Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "cattle-a",
			Payload: ops.Payload{Title: "T", NodeType: "task"}},
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ops", "cattle-b.log"), nil, 0o644))
	gittest.Git(t, dir, "add", "ops/cattle-a.log", "ops/cattle-b.log")
	gittest.Git(t, dir, "commit", "-m", "create")
	return dir, t.TempDir()
}

func hostRefRunOpts(dir string) Options {
	return Options{WriteStateFiles: true, OpsWorktree: dir}
}

func requireHostRefIncEqualsCold(t *testing.T, dir, stateDir string) Result {
	t.Helper()
	inc, incRes, err := Run(stateDir, nil, nil, hostRefRunOpts(dir))
	require.NoError(t, err)
	coldDir := t.TempDir()
	cold, _, err := Run(coldDir, nil, nil, hostRefRunOpts(dir))
	require.NoError(t, err)
	const id = "task-01"
	require.Contains(t, inc.Issues, id)
	require.Contains(t, cold.Issues, id)
	require.Equal(t, cold.Issues[id].HostRefs, inc.Issues[id].HostRefs, "HostRefs incremental vs cold")
	incCur, incOK := CurrentHostRef(inc.Issues[id].HostRefs)
	coldCur, coldOK := CurrentHostRef(cold.Issues[id].HostRefs)
	require.Equal(t, coldOK, incOK, "CurrentHostRef presence")
	require.Equal(t, coldCur, incCur, "CurrentHostRef")
	incIdx, err := LoadIndex(filepath.Join(stateDir, "index.json"))
	require.NoError(t, err)
	coldIdx, err := LoadIndex(filepath.Join(coldDir, "index.json"))
	require.NoError(t, err)
	require.Equal(t, coldIdx[id].CurrentHostURL, incIdx[id].CurrentHostURL, "index current_host_url")
	require.Equal(t, coldIdx[id].LiveHostURLs, incIdx[id].LiveHostURLs, "index live_host_urls")
	return incRes
}

func driveTwoWorkerHostRefScript(t *testing.T, script []byte) {
	t.Helper()
	dir, stateDir := twoWorkerHostRefFixture(t)
	requireHostRefIncEqualsCold(t, dir, stateDir)

	workers := []string{"cattle-a", "cattle-b"}
	urls := []string{hostURLA, hostURLB}
	dirty := [2]bool{}
	ts := int64(200)
	for _, raw := range script {
		act := raw % hostRefActCount
		switch act {
		case hostRefActARecordA, hostRefActARecordB, hostRefActAWithdrawA, hostRefActAWithdrawB,
			hostRefActBRecordA, hostRefActBRecordB, hostRefActBWithdrawA, hostRefActBWithdrawB:
			workerIdx := 0
			if act >= hostRefActBRecordA {
				workerIdx = 1
			}
			kind := act
			if workerIdx == 1 {
				kind -= hostRefActBRecordA
			}
			url := urls[0]
			if kind == hostRefActARecordB || kind == hostRefActAWithdrawB {
				url = urls[1]
			}
			op := ops.Op{
				Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: ts, WorkerID: workers[workerIdx],
			}
			ts++
			if kind == hostRefActAWithdrawA || kind == hostRefActAWithdrawB {
				op.Payload.WithdrawExternalRef = url
			} else {
				op.Payload.ExternalRef = url
				op.Payload.ExternalRole = "engineering"
			}
			require.NoError(t, ops.AppendOp(filepath.Join(dir, "ops", workers[workerIdx]+".log"), op))
			dirty[workerIdx] = true
		case hostRefActACommit, hostRefActBCommit:
			workerIdx := 0
			if act == hostRefActBCommit {
				workerIdx = 1
			}
			if !dirty[workerIdx] {
				continue
			}
			rel := "ops/" + workers[workerIdx] + ".log"
			gittest.Git(t, dir, "add", rel)
			gittest.Git(t, dir, "commit", "-m", workers[workerIdx]+" publish")
			dirty[workerIdx] = false
		}
		requireHostRefIncEqualsCold(t, dir, stateDir)
	}
}

func hostRefScriptFromSeed(seed uint64, n int) []byte {
	out := make([]byte, n)
	s := seed
	for i := range out {
		s = s*6364136223846793005 + 1
		out[i] = byte(s % uint64(hostRefActCount))
	}
	return out
}
