package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

const (
	hostRefURLA = "https://github.com/octocat/hello-world/issues/1"
	hostRefURLB = "https://github.com/octocat/hello-world/issues/2"
)

func setupRefRepo(t *testing.T, issueID string) string {
	t.Helper()
	repo := setupTransitionIdempotencyRepo(t, issueID)
	return repo
}

func externalRefOpsForIssue(t *testing.T, repo, issueID string) []ops.Op {
	t.Helper()
	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	var out []ops.Op
	for _, op := range allOps {
		if op.Type == ops.OpExternalRef && op.TargetID == issueID {
			out = append(out, op)
		}
	}
	return out
}

func TestRefRecordsCanonicalURLAndRole_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-record"
	repo := setupRefRepo(t, issueID)

	out, err := runTrls(t, repo, "ref", issueID, "https://GitHub.com/Octocat/Hello-World/issues/1?x=1#frag", "--role", "engineering")
	require.NoError(t, err)
	assert.Contains(t, out, `"url":"`+hostRefURLA+`"`)
	assert.Contains(t, out, `"role":"engineering"`)

	got := externalRefOpsForIssue(t, repo, issueID)
	require.Len(t, got, 1)
	assert.Equal(t, hostRefURLA, got[0].Payload.ExternalRef)
	assert.Equal(t, "engineering", got[0].Payload.ExternalRole)
	assert.Empty(t, got[0].Payload.WithdrawExternalRef)
}

func TestRefIdenticalCurrentIsNoOp_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-noop"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "support")
	require.NoError(t, err)
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 1)

	out, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "support")
	require.NoError(t, err)
	assert.Contains(t, out, `"noop":true`)
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 1)

	human, err := runTrls(t, repo, "--format", "human", "ref", issueID, hostRefURLA, "--role", "support")
	require.NoError(t, err)
	assert.Contains(t, human, "no-op")
}

func TestRefAfterDifferentNameAppends_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-switch"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "engineering")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "ref", issueID, hostRefURLB, "--role", "support")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "engineering")
	require.NoError(t, err)

	got := externalRefOpsForIssue(t, repo, issueID)
	require.Len(t, got, 3)
	assert.Equal(t, hostRefURLA, got[2].Payload.ExternalRef)
	assert.Equal(t, "engineering", got[2].Payload.ExternalRole)
}

func TestRefRoleChangeAppends_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-role"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "engineering")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "support")
	require.NoError(t, err)

	got := externalRefOpsForIssue(t, repo, issueID)
	require.Len(t, got, 2)
	assert.Equal(t, "support", got[1].Payload.ExternalRole)
}

func TestRefWithdrawLiveAndNonLive_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-withdraw"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, hostRefURLA)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--withdraw")
	require.NoError(t, err)
	assert.Contains(t, out, `"withdrawn":true`)
	got := externalRefOpsForIssue(t, repo, issueID)
	require.Len(t, got, 2)
	assert.Equal(t, hostRefURLA, got[1].Payload.WithdrawExternalRef)
	assert.Empty(t, got[1].Payload.ExternalRef)

	out, err = runTrls(t, repo, "ref", issueID, hostRefURLA, "--withdraw")
	require.NoError(t, err)
	assert.Contains(t, out, `"noop":true`)
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 2)
}

func TestRefRoleAndWithdrawConflict_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-conflict"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "support", "--withdraw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--role")
	assert.Contains(t, err.Error(), "--withdraw")
	assert.Empty(t, externalRefOpsForIssue(t, repo, issueID))
}

func TestRefRejectsInvalidURL_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	issueID := "hostref-s1-t5-badurl"
	repo := setupRefRepo(t, issueID)

	_, err := runTrls(t, repo, "ref", issueID, "https://linear.app/team/issue/ABC-1")
	require.Error(t, err)
	assert.Empty(t, externalRefOpsForIssue(t, repo, issueID))
}

func TestRefIdenticalRetryPublishesUnpublishedLocalOp_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	bareDir, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	issueID := "hostref-s1-t5-publish"
	_, err = runTrls(t, repo, "create", "--id", issueID, "--title", "unpublished ref retry", "--type", "task")
	require.NoError(t, err)
	breakOrigin(t, repo)

	_, err = runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "engineering")
	require.Error(t, err)
	assert.True(t, isLocalArmatureTipPublishError(err), "first ref must fail loud on publish, got %v", err)
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 1)
	assert.False(t, originArmatureContains(t, bareDir, issueID), "origin must still lack the unpublished ref")

	retryBroken := new(bytes.Buffer)
	code := executeThenHandleRootError(t, retryBroken, new(bytes.Buffer),
		"ref", "--repo", repo, issueID, hostRefURLA, "--role", "engineering", "--format", "agent")
	assert.Equal(t, 1, code, "identical retry while origin lacks the op must not exit 0")
	cf := agentFailureFromStdout(t, retryBroken.String())
	assert.Equal(t, "REF-1", cf.Code)
	assert.Contains(t, cf.Cause, "publish _armature")
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 1, "retry must not append a second external-ref")

	restoreOrigin(t, repo, bareDir)
	out, err := runTrls(t, repo, "ref", issueID, hostRefURLA, "--role", "engineering")
	require.NoError(t, err)
	assert.Contains(t, out, `"noop":true`)
	require.Len(t, externalRefOpsForIssue(t, repo, issueID), 1)
	assert.True(t, originArmatureContains(t, bareDir, issueID), "identical retry must publish the already-local external-ref")
}

func TestHostRefCommandIsNoOpHelpers_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	refs := []materialize.HostRef{
		{URL: hostRefURLA, Role: "engineering"},
		{URL: hostRefURLB, Role: "support"},
	}
	assert.False(t, hostRefCommandIsNoOp(refs, hostRefURLA, "engineering", false))
	assert.True(t, hostRefCommandIsNoOp(refs, hostRefURLB, "support", false))
	assert.False(t, hostRefCommandIsNoOp(refs, hostRefURLA, "engineering", true),
		"withdraw of a live URL must append")
	assert.True(t, hostRefCommandIsNoOp(refs, "https://github.com/octocat/hello-world/issues/99", "", true),
		"withdraw of a URL that is not live is a no-op")

	withdrawn := append(append([]materialize.HostRef{}, refs...), materialize.HostRef{URL: hostRefURLB, Withdrawn: true})
	assert.True(t, hostRefCommandIsNoOp(withdrawn, hostRefURLB, "", true))
	assert.False(t, hostRefCommandIsNoOp(withdrawn, hostRefURLA, "", true))
}

func TestRefRejectsUnknownTargetRecordAndWithdraw_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "hostref-s1-t5-exists", "--title", "exists", "--type", "task")
	require.NoError(t, err)

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	logPath := filepath.Join(worktree, "ops", workerID+".log")
	logBefore, err := os.ReadFile(logPath)
	require.NoError(t, err)

	for _, args := range [][]string{
		{"ref", "--repo", repo, "hostref-s1-t5-missing", hostRefURLA, "--role", "engineering", "--format", "agent"},
		{"ref", "--repo", repo, "hostref-s1-t5-missing", hostRefURLA, "--withdraw", "--format", "agent"},
	} {
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), args...)
		assert.Equal(t, 1, code, "args=%v", args)
		cf := agentFailureFromStdout(t, stdout.String())
		assert.Equal(t, "REF-1", cf.Code, "args=%v", args)
		assert.Contains(t, cf.Cause, "not found", "args=%v", args)
	}

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.Equal(t, headBefore, headAfter, "unknown target must not commit")
	logAfter, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, logBefore, logAfter, "unknown target must not append")
	assert.Empty(t, externalRefOpsForIssue(t, repo, "hostref-s1-t5-missing"))
	assert.False(t, originArmatureContains(t, bareDir, "hostref-s1-t5-missing"))
}

func TestRefRejectsBrokenReplay_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	issueID := "hostref-s1-t5-broken"
	_, err = runTrls(t, repo, "create", "--id", issueID, "--title", "broken replay", "--type", "task")
	require.NoError(t, err)

	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	logPath := filepath.Join(worktree, "ops", workerID+".log")
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "ghost-never-created", Timestamp: 999, WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusDone},
	}))
	run(t, worktree, "git", "add", "ops/"+workerID+".log")
	run(t, worktree, "git", "commit", "-m", "corrupt-replay-transition")

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	logBefore, err := os.ReadFile(logPath)
	require.NoError(t, err)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"ref", "--repo", repo, issueID, hostRefURLA, "--role", "engineering", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "REF-1", cf.Code)
	assert.True(t,
		strings.Contains(cf.Cause, "materialize") || strings.Contains(cf.Cause, "replay") || strings.Contains(cf.Cause, "not found"),
		"broken replay cause=%q", cf.Cause)

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.Equal(t, headBefore, headAfter, "broken replay must not commit")
	logAfter, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, logBefore, logAfter, "broken replay must not append")
	assert.Empty(t, externalRefOpsForIssue(t, repo, issueID))
	assert.False(t, originArmatureContains(t, bareDir, `"external_ref"`))
}

func requireHostRefsMatchColdCommitted(t *testing.T, repo string) []materialize.HostRef {
	t.Helper()
	const issueID = "task-01"
	ctx := getTestContext(t, repo)
	ctx.StateDir = getTestStateDir(t, repo)
	require.NotEmpty(t, ctx.WorktreePath)

	snap, err := newSnapshotStore(ctx).LoadReadOnly(t.Context())
	require.NoError(t, err)
	live := snap.Issues[issueID]
	require.NotNil(t, live)

	coldDir := t.TempDir()
	cold, _, err := materialize.Run(coldDir, nil, nil, materialize.Options{
		WriteStateFiles: true,
		OpsWorktree:     ctx.WorktreePath,
	})
	require.NoError(t, err)
	coldIssue := cold.Issues[issueID]
	require.NotNil(t, coldIssue)
	require.Equal(t, coldIssue.HostRefs, live.HostRefs,
		"arm ref predicate view must match cold committed-order HostRefs")
	return live.HostRefs
}

func TestRefWithdrawClockSkewedAlreadyWithdrawnIsNoOp_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	repo := setupBootstrappedShowRepo(t)
	workerID := plantClockSkewedHostRefWithdraw(t, repo)
	requireNoLiveHostRefA(t, repo, workerID)
	beforeRefs := requireHostRefsMatchColdCommitted(t, repo)
	beforeLogs := workerLogSnapshot(t, repo)
	beforeOps := externalRefOpsForIssue(t, repo, "task-01")

	out, err := runTrls(t, repo, "ref", "task-01", hostRefURLA, "--withdraw")
	require.NoError(t, err)
	assert.Contains(t, out, `"noop":true`,
		"committed-order has no live A; withdraw must no-op (timestamp order would wrongly append)")

	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo))
	require.Len(t, externalRefOpsForIssue(t, repo, "task-01"), len(beforeOps))
	afterRefs := requireHostRefsMatchColdCommitted(t, repo)
	require.Equal(t, beforeRefs, afterRefs)
	_, ok := materialize.CurrentHostRef(afterRefs)
	assert.False(t, ok)
}

func TestRefRecordClockSkewedWithdrawnReAdds_REQ_HOSTREF_S1_T5(t *testing.T) {
	t.Parallel()
	repo := setupBootstrappedShowRepo(t)
	workerID := plantClockSkewedHostRefWithdraw(t, repo)
	requireNoLiveHostRefA(t, repo, workerID)
	_ = requireHostRefsMatchColdCommitted(t, repo)
	beforeOps := externalRefOpsForIssue(t, repo, "task-01")
	require.Len(t, beforeOps, 2, "planted add+withdraw")

	out, err := runTrls(t, repo, "ref", "task-01", hostRefURLA, "--role", "engineering")
	require.NoError(t, err)
	assert.NotContains(t, out, `"noop":true`,
		"committed-order has no live A; re-record must append (timestamp order would wrongly no-op)")
	assert.Contains(t, out, `"url":"`+hostRefURLA+`"`)

	got := externalRefOpsForIssue(t, repo, "task-01")
	require.Len(t, got, 3)
	assert.Equal(t, hostRefURLA, got[2].Payload.ExternalRef)
	assert.Equal(t, "engineering", got[2].Payload.ExternalRole)
	assert.Empty(t, got[2].Payload.WithdrawExternalRef)

	refs := requireHostRefsMatchColdCommitted(t, repo)
	require.Len(t, refs, 3)
	cur, ok := materialize.CurrentHostRef(refs)
	require.True(t, ok)
	assert.Equal(t, hostRefURLA, cur.URL)
	assert.Equal(t, "engineering", cur.Role)
	assert.Equal(t, workerID, cur.WorkerID)
}
