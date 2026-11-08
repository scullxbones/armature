package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

const showHostRefURLA = "https://github.com/octocat/hello-world/issues/1"

func setupBootstrappedShowRepo(t *testing.T) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-01", "--title", "Host ref order", "--type", "task")
	require.NoError(t, err)
	return repo
}

func commitOpsWorktree(t *testing.T, worktree, msg string) {
	t.Helper()
	gittest.Git(t, worktree, "add", "-A")
	gittest.Git(t, worktree, "commit", "-m", msg)
}

func armatureStateTreeDigest(t *testing.T, repo string) string {
	t.Helper()
	root := getTestStateDir(t, repo)
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

func plantClockSkewedHostRefWithdraw(t *testing.T, repo string) string {
	t.Helper()
	ctx := getTestContext(t, repo)
	require.NotEmpty(t, ctx.WorktreePath)
	_, err := os.Stat(filepath.Join(ctx.WorktreePath, ".git"))
	require.NoError(t, err, "ops worktree must be a real git checkout for committed-order Load")

	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	// Pin cutover at current HEAD (post-create) so add/withdraw use commit order.
	sha, headErr := adapters.New(ctx.WorktreePath).HeadSHA()
	require.NoError(t, headErr)
	require.NotEmpty(t, sha)
	require.NoError(t, gittest.SetLocalConfig(ctx.WorktreePath, "armature.oporder-cutover", sha))

	// create is already committed via arm create. Record A at ts300, then withdraw
	// at ts200, committing in that order so cold LocateOps sees withdraw last while
	// timestamp sort would resurrect A.
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 300, WorkerID: workerID,
		Payload: ops.Payload{ExternalRef: showHostRefURLA, ExternalRole: "engineering"},
	}))
	commitOpsWorktree(t, ctx.WorktreePath, "hostref add A ts300")

	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: workerID,
		Payload: ops.Payload{WithdrawExternalRef: showHostRefURLA},
	}))
	commitOpsWorktree(t, ctx.WorktreePath, "hostref withdraw A ts200")
	return workerID
}

func requireNoLiveHostRefA(t *testing.T, repo, workerID string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)
	before := armatureStateTreeDigest(t, repo)
	snap, err := newSnapshotStore(ctx).LoadReadOnly(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, armatureStateTreeDigest(t, repo), "LoadReadOnly must write no state files")

	hostIssue := snap.Issues["task-01"]
	require.NotNil(t, hostIssue)
	require.Equal(t, []materialize.HostRef{
		{URL: showHostRefURLA, Role: "engineering", At: 300, WorkerID: workerID},
		{URL: showHostRefURLA, At: 200, WorkerID: workerID, Withdrawn: true},
	}, hostIssue.HostRefs)
	_, ok := materialize.CurrentHostRef(hostIssue.HostRefs)
	assert.False(t, ok, "committed withdraw after add must leave no live A despite ts200 < ts300")
	assert.Empty(t, snap.Index["task-01"].CurrentHostURL)
	assert.Empty(t, snap.Index["task-01"].LiveHostURLs)
}

func TestShowClockSkewedWithdrawLeavesNoLiveHostRef_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	repo := setupBootstrappedShowRepo(t)
	workerID := plantClockSkewedHostRefWithdraw(t, repo)

	out, err := runTrls(t, repo, "--format", "human", "show", "task-01")
	require.NoError(t, err)
	assert.Contains(t, out, "task-01")

	jsonOut, err := runTrls(t, repo, "--format", "json", "show", "task-01")
	require.NoError(t, err)
	issue := decodeShowIssue(t, jsonOut)
	assert.Equal(t, "task-01", issue["id"])

	requireNoLiveHostRefA(t, repo, workerID)
}

func TestShowUnknownIssueAppendsNoOp_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	repo := setupBootstrappedShowRepo(t)
	workerID := plantClockSkewedHostRefWithdraw(t, repo)

	beforeLogs := workerLogSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "show", "no-such-issue")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, "no-such-issue")
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "unknown issue must append no op")

	requireNoLiveHostRefA(t, repo, workerID)
}

func TestShowJSONUnknownIssueFailure_REQ_HOSTREF_S1(t *testing.T) {
	t.Parallel()
	repo := setupBootstrappedShowRepo(t)
	_ = plantClockSkewedHostRefWithdraw(t, repo)
	beforeLogs := workerLogSnapshot(t, repo)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"show", "--repo", repo, "missing-issue", "--format", "json")
	assert.Equal(t, 1, code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &envelope))
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo))
}
