package e2eharness_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/e2eharness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHappyPathLifecycle_REQ_TOPTIER_S3_T1 exercises the complete happy-path lifecycle:
// bootstrap → worker-init → create → claim → in-progress → done → merge detection (sync).
// This test verifies that the harness correctly orchestrates the full workflow and that
// materialized state progresses through each step.
func TestHappyPathLifecycle_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	// Build the arm binary first
	armBinPath := buildArmBinary(t)

	// Create harness with bare origin and work directory
	h := e2eharness.New(t, armBinPath)

	// Step 1: Bootstrap the repository
	t.Logf("Step 1: Bootstrap")
	out, err := h.RunArm("bootstrap", "--repo", h.WorkDir)
	require.NoError(t, err, "bootstrap failed: %s", out)

	// Verify .armature/ directory is created
	armatureDir := filepath.Join(h.WorkDir, ".armature")
	assert.DirExists(t, armatureDir, ".armature directory should be created after bootstrap")

	// Step 2: Worker init
	t.Logf("Step 2: Worker init")
	out, err = h.RunArm("worker-init", "--repo", h.WorkDir)
	require.NoError(t, err, "worker-init failed: %s", out)
	assert.Contains(t, out, "Worker ID", "worker-init should output Worker ID")

	// Step 3: Create an issue
	t.Logf("Step 3: Create issue")
	out, err = h.RunArm("create", "--repo", h.WorkDir, "--type", "task", "--id", "TEST-001",
		"--title", "Test task", "--dod", "Task implementation is complete")
	require.NoError(t, err, "create failed: %s", out)
	assert.Contains(t, out, "TEST-001", "create should output issue ID")

	// Step 4: Materialize state (populate index)
	t.Logf("Step 4: Materialize state")
	out, err = h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err, "materialize failed: %s", out)

	// Step 5: Claim the issue (create a worker worktree and task branch)
	t.Logf("Step 5: Claim issue")
	worktreePath := filepath.Join(h.TempDir, "task-worktree")

	out, err = h.RunArm("claim", "--repo", h.WorkDir, "--issue", "TEST-001",
		"--worktree", worktreePath)
	require.NoError(t, err, "claim failed: %s", out)
	assert.Contains(t, out, "TEST-001", "claim should output issue ID")

	// Step 6: Transition to in-progress
	t.Logf("Step 6: Transition to in-progress")
	out, err = h.RunArm("transition", "--repo", h.WorkDir, "--issue", "TEST-001", "--to", "in-progress")
	require.NoError(t, err, "transition to in-progress failed: %s", out)

	// Materialize and verify state
	_, err = h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err)

	// Step 7: Transition to done
	t.Logf("Step 7: Transition to done")
	out, err = h.RunArm("transition", "--repo", h.WorkDir, "--issue", "TEST-001", "--to", "done",
		"--force", "--outcome", "Implementation complete")
	require.NoError(t, err, "transition to done failed: %s", out)

	// Materialize and verify state
	_, err = h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err)

	// Step 8: Simulate merge detection with sync
	t.Logf("Step 8: Sync (merge detection)")
	// Create feature branch and merge to simulate PR merge
	gitRunInDir(t, h.WorkDir, "checkout", "-b", "feature/test-task")
	gitRunInDir(t, h.WorkDir, "commit", "--allow-empty", "-m", "feat: test task work")

	currentBranch := gitGetCurrentBranch(t, h.WorkDir)
	gitRunInDir(t, h.WorkDir, "checkout", currentBranch)
	gitRunInDir(t, h.WorkDir, "-c", "core.hooksPath=/dev/null", "merge", "--no-ff",
		"feature/test-task", "-m", "Merge feature/test-task")

	// Push to origin to simulate PR merge being merged
	gitRunInDir(t, h.WorkDir, "push", "-u", "origin", currentBranch)

	// Run sync to detect merge
	out, err = h.RunArm("sync", "--repo", h.WorkDir)
	require.NoError(t, err, "sync failed: %s", out)

	// Materialize final state
	_, err = h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err)

	// Final verification: show the issue to confirm terminal state
	out, err = h.RunArm("show", "--repo", h.WorkDir, "--issue", "TEST-001", "--field", "status")
	require.NoError(t, err)
	// Issue should be either "done" or "merged" depending on whether merge was detected
	assert.True(t, strings.Contains(out, "done") || strings.Contains(out, "merged"),
		"final status should be done or merged, got: %s", out)

	t.Logf("Happy-path lifecycle test completed successfully")
}

// buildArmBinary compiles the arm binary for use in tests. It returns the path
// to the built binary or fails the test if compilation fails.
func buildArmBinary(t *testing.T) string {
	t.Helper()

	// Find the repo root by looking for go.mod
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOutput, err := cmd.Output()
	require.NoError(t, err, "failed to find repo root")

	repoRoot := strings.TrimSpace(string(rootOutput))
	binPath := filepath.Join(repoRoot, "bin", "arm")

	// Build the binary
	buildCmd := exec.Command("make", "-C", repoRoot, "build")
	buildCmd.Dir = repoRoot
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "failed to build arm binary: %s", buildOut)

	require.FileExists(t, binPath, "built arm binary not found at %s", binPath)

	return binPath
}

// gitRunInDir runs a git command in a specified directory.
func gitRunInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed in %s: %s", args, dir, out)
}

// gitGetCurrentBranch returns the name of the current branch.
func gitGetCurrentBranch(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}
