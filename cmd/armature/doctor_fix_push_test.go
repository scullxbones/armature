package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorFixPushesToOriginInDualBranchMode(t *testing.T) {
	bareDir := t.TempDir()
	run(t, bareDir, "git", "init", "--bare")

	repo := initTempRepo(t)
	run(t, repo, "git", "remote", "set-url", "origin", bareDir)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	refBefore := showArmatureRef(t, bareDir)

	opsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	logPath := filepath.Join(opsDir, "worker-01.log")
	staleClaim := time.Now().Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "fixpush-01", Timestamp: staleClaim, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Doctor fix push test", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "fixpush-01", Timestamp: staleClaim, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 5}},
	}))

	out, err := runTrls(t, repo, "doctor", "--fix")
	require.NoError(t, err, "doctor --fix output: %s", out)

	refAfter := showArmatureRef(t, bareDir)
	require.NotEqual(t, refBefore, refAfter,
		"doctor --fix must push its repair ops to origin's _armature branch, not just commit them locally")

	treeOut := runOutput(t, bareDir, "ls-tree", "-r", "--name-only", "refs/heads/_armature")
	var found bool
	for _, path := range strings.Fields(treeOut) {
		if !strings.HasPrefix(path, "ops/") {
			continue
		}
		content := runOutput(t, bareDir, "show", "refs/heads/_armature:"+path)
		if strings.Contains(content, "fixpush-01") && strings.Contains(content, "doctor --fix:") {
			found = true
			break
		}
	}
	require.True(t, found,
		"origin's _armature branch must contain the doctor repair op for fixpush-01, not just an unrelated commit")
}

func TestDoctorFixPublishFailureKeepsLocalRepair_REQ_OPS_PUBLISH(t *testing.T) {
	_, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	logPath := filepath.Join(opsDir, "worker-01.log")
	staleClaim := time.Now().Add(-2 * time.Hour).Unix()
	require.NoError(t, ops.AppendOps(logPath, []ops.Op{
		{Type: ops.OpCreate, TargetID: "fixpush-fail", Timestamp: staleClaim, WorkerID: "worker-01",
			Payload: ops.Payload{Title: "Doctor fix publish fail", NodeType: "task"}},
		{Type: ops.OpClaim, TargetID: "fixpush-fail", Timestamp: staleClaim, WorkerID: "worker-01",
			Payload: ops.Payload{TTL: 5}},
	}))

	worktree := filepath.Join(repo, ".armature")
	headBefore := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	breakOrigin(t, repo)
	out, err := runTrls(t, repo, "doctor", "--fix")
	require.Error(t, err, "doctor --fix output: %s", out)
	assert.True(t, isLocalArmatureTipPublishError(err) || strings.Contains(err.Error(), "publish _armature"), "got %v", err)

	headAfter := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "HEAD"))
	assert.NotEqual(t, headBefore, headAfter, "local doctor --fix commit must remain after publish failure")
	show := runOutput(t, worktree, "show", "HEAD")
	assert.True(t,
		strings.Contains(show, "doctor --fix:") || strings.Contains(show, "fixpush-fail"),
		"HEAD commit must contain the repair; show=%s", show)
}

func showArmatureRef(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--verify", "--quiet", "refs/heads/_armature")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
