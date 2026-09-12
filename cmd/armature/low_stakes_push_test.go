package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/require"
)

func TestAppendLowStakesOps_PushesOriginAtThreshold(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	refBefore := showArmatureRef(t, bareDir)
	require.NotEmpty(t, refBefore, "origin must have _armature before the low-stakes write")

	state := lowStakesState(t, repo, worktree, 1)
	logPath := filepath.Join(worktree, "ops", "low-stakes-push.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-LOW-1", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "threshold push"}}
	require.NoError(t, appendLowStakesOp(state, logPath, op))

	refAfter := showArmatureRef(t, bareDir)
	require.NotEqual(t, refBefore, refAfter,
		"low-stakes at threshold must Push origin _armature; Reset-only leaves origin unchanged")
}

func TestAppendLowStakesOps_DoesNotPushBelowThreshold(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	refBefore := showArmatureRef(t, bareDir)
	require.NotEmpty(t, refBefore)

	state := lowStakesState(t, repo, worktree, 5)
	logPath := filepath.Join(worktree, "ops", "low-stakes-hold.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	op := ops.Op{Type: ops.OpNote, TargetID: "T-LOW-2", Timestamp: 100, WorkerID: "w1", Payload: ops.Payload{Msg: "below threshold"}}
	require.NoError(t, appendLowStakesOp(state, logPath, op))

	refAfter := showArmatureRef(t, bareDir)
	require.Equal(t, refBefore, refAfter,
		"a single low-stakes op below threshold must not push origin _armature")
}

func bootstrappedRepoWithFileOrigin(t *testing.T) (bareDir, repo, worktree string) {
	t.Helper()
	bareDir = t.TempDir()
	run(t, bareDir, "git", "init", "--bare")

	repo = initTempRepo(t)
	run(t, repo, "git", "remote", "add", "origin", "file://"+bareDir)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	return bareDir, repo, filepath.Join(repo, ".armature")
}

func lowStakesState(t *testing.T, repo, worktree string, threshold int) *executionState {
	t.Helper()
	return &executionState{
		ctx: &config.Context{
			RepoPath:     repo,
			WorktreePath: worktree,
			IssuesDir:    worktree,
			StateDir:     filepath.Join(worktree, "state"),
			Config:       config.Config{LowStakesPushThreshold: threshold},
		},
		tracker: &fakePendingPushTracker{},
	}
}
