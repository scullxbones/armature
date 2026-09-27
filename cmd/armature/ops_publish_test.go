package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimPublishAuthClassKeepsLocalCommit_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "auth fail", "--id", "task-auth")
	require.NoError(t, err)
	installBarePreReceive(t, bareDir, `#!/bin/sh
echo >&2 "remote: Permission to example.git denied to tester."
echo >&2 "fatal: unable to access 'https://github.com/example/repo.git/': The requested URL returned error: 403"
exit 1
`)

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-auth", "--worktree", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "CLAIM-1", cf.Code)
	assert.Contains(t, cf.Cause, "class=auth")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "Contents: Write")
	assert.Contains(t, joined, "arm push-ops")
	assert.Contains(t, cf.NextActions[0], "arm push-ops")
	assert.NotContains(t, joined, "arm doctor")
	assert.False(t, originArmatureContains(t, bareDir, "task-auth"),
		"origin must not receive the claim when push is denied")

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local claim commit must remain after auth publish failure")
	require.True(t, localOpsContainClaim(t, repo, "task-auth"), "local ops log must keep the claim")
}

func TestClaimPublishNonFFClass_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "non-ff fail", "--id", "task-nff")
	require.NoError(t, err)
	installBarePreReceive(t, bareDir, `#!/bin/sh
echo >&2 "! [rejected]        _armature -> _armature (non-fast-forward)"
echo >&2 "error: failed to push some refs to 'origin'"
echo >&2 "hint: Updates were rejected because the tip of your current branch is behind"
echo >&2 "hint: its remote counterpart. Fetch first."
exit 1
`)

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"claim", "--repo", repo, "--issue", "task-nff", "--worktree", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "CLAIM-1", cf.Code)
	assert.Contains(t, cf.Cause, "class=non-fast-forward")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm push-ops")
	assert.Contains(t, joined, "fetch")
	assert.Contains(t, joined, "rebase")
	assert.NotContains(t, joined, "arm doctor")
	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local claim commit must remain after non-ff publish failure")
	require.True(t, localOpsContainClaim(t, repo, "task-nff"))
}

func installBarePreReceive(t *testing.T, bareDir, script string) {
	t.Helper()
	hookDir := filepath.Join(bareDir, "hooks")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	hook := filepath.Join(hookDir, "pre-receive")
	require.NoError(t, os.WriteFile(hook, []byte(script), 0o755))
}

func localOpsContainClaim(t *testing.T, repo, issueID string) bool {
	t.Helper()
	workerID := strings.TrimSpace(runOutput(t, repo, "config", "--get", "armature.worker-id"))
	require.NotEmpty(t, workerID)
	logged, err := ops.ReadLog(filepath.Join(repo, ".armature", "ops", workerID+".log"))
	require.NoError(t, err)
	for _, op := range logged {
		if op.Type == ops.OpClaim && op.TargetID == issueID {
			return true
		}
	}
	return false
}

func TestAppendHighStakesOp_PublishFailureKeepsLocalCommit_REQ_OPS_PUBLISH(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "high-stakes-publish.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))

	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	breakOrigin(t, repo)

	op := ops.Op{Type: ops.OpNote, TargetID: "T-HS-1", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "loud publish"}}
	err = appendHighStakesOp(state, logPath, op)
	require.Error(t, err)
	assert.True(t, isLocalArmatureTipPublishError(err), "got %v", err)
	assert.Contains(t, err.Error(), "publish _armature")

	logged, readErr := ops.ReadLog(logPath)
	require.NoError(t, readErr)
	require.NotEmpty(t, logged)
	assert.Equal(t, "T-HS-1", logged[len(logged)-1].TargetID)

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local ops commit must remain after publish failure")

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 0, tracker.resetCalls, "failed high-stakes publish must not Reset the pending tracker")
}

func TestAppendHighStakesOp_SuccessfulPushResetsTracker_REQ_OPS_PUBLISH(t *testing.T) {
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "high-stakes-ok.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-HS-2", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "ok"}}
	require.NoError(t, appendHighStakesOp(state, logPath, op))

	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.resetCalls)
}

func TestAppendLowStakesOps_PublishFailureIsBestEffort_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	refBefore := showArmatureRef(t, bareDir)
	breakOrigin(t, repo)

	state := lowStakesState(t, repo, worktree, 1)
	logPath := filepath.Join(worktree, "ops", "low-stakes-be.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-LOW-BE", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "best-effort"}}
	require.NoError(t, appendLowStakesOp(state, logPath, op), "low-stakes coalesced publish must swallow git errors")

	assert.Equal(t, refBefore, showArmatureRef(t, bareDir), "failed best-effort push must not update origin")
	tracker, ok := state.tracker.(*fakePendingPushTracker)
	require.True(t, ok)
	assert.Equal(t, 1, tracker.resetCalls)
}

func TestPushOps_PushFailureStillPUSHOPS1_REQ_OPS_PUBLISH(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	dropOrigin(t, repo)

	out, err := runTrls(t, repo, "push-ops", "--format", "json")
	require.Error(t, err)
	assert.NotContains(t, out, `"status":"pushed"`)
	assert.Contains(t, err.Error(), "push-ops: push failed")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"push-ops", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "PUSH-OPS-1", cf.Code)
	assert.Contains(t, cf.Cause, "class=other")
	assert.Contains(t, cf.Cause, "push-ops: push failed")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm push-ops")
	assert.Contains(t, joined, "arm doctor")
}

func TestPushOps_AuthClass_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "auth push-ops", "--id", "task-auth-po")
	require.NoError(t, err)
	installBarePreReceive(t, bareDir, `#!/bin/sh
echo >&2 "remote: Permission to example.git denied to tester."
echo >&2 "fatal: unable to access 'https://github.com/example/repo.git/': The requested URL returned error: 403"
exit 1
`)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"push-ops", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "PUSH-OPS-1", cf.Code)
	assert.Contains(t, cf.Cause, "class=auth")
	require.NotEmpty(t, cf.NextActions)
	assert.Contains(t, cf.NextActions[0], "Contents: Write")
	assert.Contains(t, cf.NextActions[0], "arm push-ops")
	assert.NotContains(t, strings.Join(cf.NextActions, "\n"), "arm doctor")
}

func TestPushOps_NonFFClass_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "non-ff push-ops", "--id", "task-nff-po")
	require.NoError(t, err)
	installBarePreReceive(t, bareDir, `#!/bin/sh
echo >&2 "! [rejected]        _armature -> _armature (non-fast-forward)"
echo >&2 "error: failed to push some refs to 'origin'"
echo >&2 "hint: Updates were rejected because the tip of your current branch is behind"
exit 1
`)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"push-ops", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "PUSH-OPS-1", cf.Code)
	assert.Contains(t, cf.Cause, "class=non-fast-forward")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "fetch")
	assert.Contains(t, joined, "rebase")
	assert.Contains(t, joined, "arm push-ops")
	assert.NotContains(t, joined, "arm doctor")
}

func TestTransitionIdenticalRetryPublishesUnpublishedLocalOp_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	issueID := "ops-pub-retry"
	_, err = runTrls(t, repo, "create", "--id", issueID, "--title", "unpublished retry", "--type", "task")
	require.NoError(t, err)
	breakOrigin(t, repo)

	_, err = runTrls(t, repo, "transition", "--issue", issueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.Error(t, err)
	assert.True(t, isLocalArmatureTipPublishError(err), "first transition must fail loud on publish, got %v", err)
	require.Len(t, transitionOpsForIssue(t, repo, issueID), 1)
	assert.False(t, originArmatureContains(t, bareDir, issueID), "origin must still lack the unpublished transition")

	retryBroken := new(bytes.Buffer)
	code := executeThenHandleRootError(t, retryBroken, new(bytes.Buffer),
		"transition", "--repo", repo, "--issue", issueID, "--to", "blocked",
		"--outcome", idempotentOutcomeWaiting, "--format", "agent")
	assert.Equal(t, 1, code, "identical retry while origin lacks the op must not exit 0")
	cf := agentFailureFromStdout(t, retryBroken.String())
	assert.Equal(t, codeTransition1, cf.Code)
	assert.Contains(t, cf.Cause, "publish _armature")
	joined := strings.Join(cf.NextActions, "\n")
	assert.Contains(t, joined, "arm push-ops")
	assert.Contains(t, joined, "arm doctor")
	require.Len(t, transitionOpsForIssue(t, repo, issueID), 1, "retry must not append a second transition")

	restoreOrigin(t, repo, bareDir)
	out, err := runTrls(t, repo, "transition", "--issue", issueID, "--to", "blocked", "--outcome", idempotentOutcomeWaiting)
	require.NoError(t, err)
	assert.Contains(t, out, `"noop":true`)
	require.Len(t, transitionOpsForIssue(t, repo, issueID), 1)
	assert.True(t, originArmatureContains(t, bareDir, issueID), "identical retry must publish the already-local transition")
}

func TestAppendHighStakesOpIfNoWriteStillPublishes_REQ_OPS_PUBLISH(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "high-stakes-nowrite.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-HS-NW", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "local unpublished"}}
	breakOrigin(t, repo)
	require.Error(t, appendHighStakesOp(state, logPath, op))
	assert.False(t, originArmatureContains(t, bareDir, "T-HS-NW"))

	restoreOrigin(t, repo, bareDir)
	wrote, err := appendHighStakesOpIf(state, logPath, op, func() (bool, error) { return false, nil })
	require.NoError(t, err)
	assert.False(t, wrote)
	assert.True(t, originArmatureContains(t, bareDir, "T-HS-NW"),
		"!wrote high-stakes retry must still publish the already-local tip")
}

func TestPublishArmatureBranchReturnsRebaseError_REQ_OPS_PUBLISH(t *testing.T) {
	pushErr := errors.New("git push origin _armature: rejected (non-fast-forward)")
	rebaseErr := errors.New("git rebase origin/_armature: conflict in ops/w1.log")
	stub := &stubOpsPublisher{firstPushErr: pushErr, rebaseErr: rebaseErr}

	err := publishArmatureSequence(stub)
	require.Error(t, err)
	assert.ErrorIs(t, err, rebaseErr)
	assert.Contains(t, err.Error(), "rebase")
	assert.NotContains(t, err.Error(), "non-fast-forward",
		"rebase failure must not hide behind the stale first-push rejection")
	assert.Equal(t, 1, stub.pushes)
	assert.Equal(t, 1, stub.rebases)
}

type stubOpsPublisher struct {
	firstPushErr error
	rebaseErr    error
	secondPush   error
	pushes       int
	rebases      int
}

func (s *stubOpsPublisher) Push(string) error {
	s.pushes++
	if s.pushes == 1 {
		return s.firstPushErr
	}
	return s.secondPush
}

func (s *stubOpsPublisher) FetchAndRebase(string) error {
	s.rebases++
	return s.rebaseErr
}

func breakOrigin(t *testing.T, repo string) {
	t.Helper()
	run(t, repo, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
}

func restoreOrigin(t *testing.T, repo, bareDir string) {
	t.Helper()
	run(t, repo, "git", "remote", "set-url", "origin", "file://"+bareDir)
}

func originArmatureContains(t *testing.T, bareDir, needle string) bool {
	t.Helper()
	ref := showArmatureRef(t, bareDir)
	if ref == "" {
		return false
	}
	treeOut := runOutput(t, bareDir, "ls-tree", "-r", "--name-only", "refs/heads/_armature")
	for _, path := range strings.Fields(treeOut) {
		if !strings.HasPrefix(path, "ops/") {
			continue
		}
		if strings.Contains(runOutput(t, bareDir, "show", "refs/heads/_armature:"+path), needle) {
			return true
		}
	}
	return false
}
