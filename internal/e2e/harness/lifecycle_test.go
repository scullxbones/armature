package harness_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/scullxbones/armature/internal/e2e/harness"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHappyPathLifecycle_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	armBinPath := buildArmBinary(t)

	h := harness.New(t, armBinPath)
	mainBranch := gitGetCurrentBranch(t, h.WorkDir)

	t.Logf("Step 1: Bootstrap")
	out, err := h.RunArm("bootstrap", "--repo", h.WorkDir)
	require.NoError(t, err, "bootstrap failed: %s", out)

	armatureDir := filepath.Join(h.WorkDir, ".armature")
	assert.DirExists(t, armatureDir, ".armature directory should be created after bootstrap")

	t.Logf("Step 2: Worker init")
	out, err = h.RunArm("worker-init", "--repo", h.WorkDir)
	require.NoError(t, err, "worker-init failed: %s", out)
	assert.Contains(t, out, "Worker ID", "worker-init should output Worker ID")

	t.Logf("Step 3: Apply plan via dag apply")
	planData := map[string]any{
		"version": 1,
		"title":   "E2E Test Plan",
		"issues": []map[string]any{
			{
				"id":         "TEST-001",
				"title":      "Test task",
				"type":       "task",
				"source":     "src-e2e",
				"dod":        "Task implementation is complete",
				"scope":      "cmd/armature/test_001.go",
				"acceptance": []map[string]any{{"type": "test_passes"}},
			},
		},
	}
	planJSON, err := json.Marshal(planData)
	require.NoError(t, err)
	planFile := filepath.Join(h.TempDir, "test-plan.json")
	err = os.WriteFile(planFile, planJSON, 0o600)
	require.NoError(t, err)

	out, err = h.RunArm("dag", "apply", "--repo", h.WorkDir, "--plan", planFile)
	require.NoError(t, err, "dag apply failed: %s", out)
	assert.Contains(t, out, `"action":"created"`, "dag apply should report created issues in the envelope")

	t.Logf("Step 3b: Promote to verified via dag transition")
	out, err = h.RunArm("dag", "transition", "--repo", h.WorkDir, "--issue", "TEST-001")
	require.NoError(t, err, "dag transition failed: %s", out)
	assert.Contains(t, out, "verified", "dag transition should promote to verified")
	assertMaterializedField(t, h, "status", "open")

	t.Logf("Step 4: Claim issue")
	worktreePath := filepath.Join(h.WorkDir, ".worktrees", "TEST-001")
	deliveryBase := gitRevision(t, h.WorkDir)

	out, err = h.RunArm("claim", "--repo", h.WorkDir, "--issue", "TEST-001",
		"--worktree")
	require.NoError(t, err, "claim failed: %s", out)
	assert.Contains(t, out, "TEST-001", "claim should output issue ID")
	assertMaterializedField(t, h, "status", "claimed")
	claimedBy := materializedField(t, h, "TEST-001", "claimed_by")
	assert.NotEmpty(t, claimedBy, "claim must have a materialized owner")

	t.Logf("Step 5: Transition to in-progress")
	out, err = h.RunArmIn(worktreePath, "transition", "--repo", worktreePath, "--issue", "TEST-001", "--to", "in-progress")
	require.NoError(t, err, "transition to in-progress failed: %s", out)
	assertMaterializedField(t, h, "status", "in-progress")

	t.Logf("Step 6: Transition to done")
	featureBranch := gitGetCurrentBranch(t, worktreePath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "task.go"), []byte("package task\n"), 0o600))
	gitRunInDir(t, worktreePath, "add", "task.go")
	gitRunInDir(t, worktreePath, "commit", "--allow-empty", "-m", "feat: test task work")
	out, err = h.RunArmIn(worktreePath, "transition", "--repo", worktreePath, "--issue", "TEST-001", "--to", "done",
		"--force", "--skip-delivery-gate", "--branch", featureBranch, "--outcome", "Implementation complete")
	require.NoError(t, err, "transition to done failed: %s", out)
	assertMaterializedField(t, h, "status", "done")
	assertMaterializedField(t, h, "outcome", "Implementation complete")

	t.Logf("Step 7: Prepare and record review assessment")
	deliveryHead := gitRevision(t, worktreePath)
	bundlePath := filepath.Join(h.TempDir, "review-bundle.json")
	out, err = h.RunArmIn(worktreePath, "review", "prepare", "--repo", worktreePath,
		"--issue", "TEST-001", "--base", deliveryBase, "--head", deliveryHead, "--output", bundlePath)
	require.NoError(t, err, "review prepare failed: %s", out)
	bundleJSON, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	bundle, err := review.DecodeReviewBundle(bundleJSON)
	require.NoError(t, err, "prepared bundle must satisfy strict decoding")

	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "The task completed the declared happy-path lifecycle.",
			Citations: []review.Citation{review.FileCitation("task.go", 1, 0)},
		}, {
			ID:        "acceptance[0]",
			Status:    review.Satisfied,
			Rationale: "The declared test-passes criterion was met by the lifecycle.",
			Citations: []review.Citation{review.FileCitation("task.go", 1, 0)},
		}},
	}
	assessmentJSON, err := json.Marshal(assessment)
	require.NoError(t, err)
	_, err = review.DecodeConformanceAssessment(assessmentJSON)
	require.NoError(t, err, "assessment must satisfy strict decoding before record")
	assessmentPath := filepath.Join(h.TempDir, "assessment.json")
	require.NoError(t, os.WriteFile(assessmentPath, assessmentJSON, 0o600))

	out, err = h.RunArmIn(worktreePath, "review", "record", "--repo", worktreePath,
		"--issue", "TEST-001", "--assessment", assessmentPath, "--bundle", bundlePath)
	require.NoError(t, err, "review record failed: %s", out)
	assert.Contains(t, out, "recorded", "review record must durably attest the assessment")

	t.Logf("Step 8: Merge into %s and sync", mainBranch)
	gitRunInDir(t, h.WorkDir, "checkout", mainBranch)
	gitRunInDir(t, h.WorkDir, "-c", "core.hooksPath=/dev/null", "merge", "--no-ff",
		featureBranch, "-m", "Merge "+featureBranch)

	gitRunInDir(t, h.WorkDir, "push", "-u", "origin", mainBranch)
	assertMaterializedField(t, h, "status", "done")

	out, err = h.RunArm("sync", "--repo", h.WorkDir, "--into", mainBranch)
	require.NoError(t, err, "sync failed: %s", out)
	assertMaterializedField(t, h, "status", "merged")

	t.Logf("Happy-path lifecycle test completed successfully")
}

func assertMaterializedField(t *testing.T, h *harness.Harness, field, want string) {
	t.Helper()
	got := materializedField(t, h, "TEST-001", field)
	assert.Equal(t, want, got, "materialized %s", field)
}

func materializedField(t *testing.T, h *harness.Harness, issueID, field string) string {
	t.Helper()
	out, err := h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err, "materialize failed: %s", out)
	out, err = h.RunArm("show", "--repo", h.WorkDir, "--issue", issueID, "--field", field)
	require.NoError(t, err, "show %s failed: %s", field, out)
	return strings.TrimSpace(out)
}

var buildArmBinaryOnce struct {
	sync.Once
	path string
	err  error
}

func buildArmBinary(t *testing.T) string {
	t.Helper()

	buildArmBinaryOnce.Do(func() {
		cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
		rootOutput, err := cmd.Output()
		if err != nil {
			buildArmBinaryOnce.err = fmt.Errorf("failed to find repo root: %w", err)
			return
		}

		repoRoot := strings.TrimSpace(string(rootOutput))
		binPath := filepath.Join(repoRoot, "bin", "arm")

		buildCmd := exec.CommandContext(context.Background(), "make", "-C", repoRoot, "build")
		buildCmd.Dir = repoRoot
		buildOut, err := buildCmd.CombinedOutput()
		if err != nil {
			buildArmBinaryOnce.err = fmt.Errorf("failed to build arm binary: %s: %w", buildOut, err)
			return
		}

		if _, statErr := os.Stat(binPath); statErr != nil {
			buildArmBinaryOnce.err = fmt.Errorf("built arm binary not found at %s: %w", binPath, statErr)
			return
		}

		buildArmBinaryOnce.path = binPath
	})

	require.NoError(t, buildArmBinaryOnce.err)
	return buildArmBinaryOnce.path
}

func gitRunInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed in %s: %s", args, dir, out)
}

func gitGetCurrentBranch(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}

func gitConfigValue(t *testing.T, dir, key string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "config", "--local", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err, "read git config %s", key)
	return strings.TrimSpace(string(out))
}

func gitRevision(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err, "resolve HEAD")
	return strings.TrimSpace(string(out))
}
