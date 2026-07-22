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
	"github.com/stretchr/testify/require"
)

// TestDoctorFixPushesToOriginInDualBranchMode verifies that `arm doctor --fix`
// pushes its repair ops to origin after committing them, the same way the
// high-stakes op path (claim/transition/assign, via appendHighStakesOp) does.
// Before the fix, ApplyFixes only appended and committed the repair ops
// locally — doctor --fix never called Push — so a coordinator could report
// stale-claim repairs as applied while every other clone kept replaying the
// old _armature branch until someone manually ran `arm push-ops`.
func TestDoctorFixPushesToOriginInDualBranchMode(t *testing.T) {
	bareDir := t.TempDir()
	run(t, bareDir, "git", "init", "--bare")

	repo := initTempRepo(t)
	run(t, repo, "git", "remote", "add", "origin", bareDir)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Push the initial bootstrap state so the bare origin has a starting
	// _armature branch to compare against below.
	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	refBefore := showRef(t, bareDir, "refs/heads/_armature")

	// Directly append a create + claim op with a claim far enough in the past
	// to be stale, bypassing `arm claim` so the test doesn't depend on TTL
	// timing or worktree creation.
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

	refAfter := showRef(t, bareDir, "refs/heads/_armature")
	require.NotEqual(t, refBefore, refAfter,
		"doctor --fix must push its repair ops to origin's _armature branch, not just commit them locally")

	// It's not enough for the ref to have moved — the pushed commit must
	// actually contain the doctor repair op for fixpush-01, not some
	// unrelated change. Walk the ops files tracked on origin's _armature
	// branch and confirm at least one contains the repair note.
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

func showRef(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--verify", "--quiet", ref)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
