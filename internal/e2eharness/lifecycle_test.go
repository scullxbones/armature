package e2eharness_test

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

	"github.com/scullxbones/armature/internal/e2eharness"
	"github.com/scullxbones/armature/internal/review"
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
	// Capture the integration branch before any feature/worktree checkout. The
	// merge below must be into main, never a feature branch merged into itself.
	mainBranch := gitGetCurrentBranch(t, h.WorkDir)

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

	// Step 3: Create a plan and apply it via decompose-apply (dag apply)
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
	assert.Contains(t, out, "Applied", "dag apply should report applied issues")

	// Step 3b: Promote applied draft subtree to verified via dag transition
	t.Logf("Step 3b: Promote to verified via dag transition")
	out, err = h.RunArm("dag", "transition", "--repo", h.WorkDir, "--issue", "TEST-001")
	require.NoError(t, err, "dag transition failed: %s", out)
	assert.Contains(t, out, "verified", "dag transition should promote to verified")
	assertMaterializedField(t, h, "status", "open")

	// Step 4: Claim the issue before recording progress. Claim creates the real
	// worker worktree and branch that the later merge will promote to merged.
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

	// Step 5: The worker makes progress only after the claim has succeeded.
	t.Logf("Step 5: Transition to in-progress")
	out, err = h.RunArmIn(worktreePath, "transition", "--repo", worktreePath, "--issue", "TEST-001", "--to", "in-progress")
	require.NoError(t, err, "transition to in-progress failed: %s", out)
	assertMaterializedField(t, h, "status", "in-progress")

	// Step 6: Complete work on the branch created by claim, then record done
	// with that actual branch for merge detection.
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

	// Step 7: Prepare, strictly decode, and record the review assessment before
	// merge detection. This keeps review attestation in the same composed happy
	// path rather than testing it only as an isolated artifact pipeline.
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
			Citations: []review.Citation{{Path: "task.go", Line: 1}},
		}, {
			ID:        "acceptance[0]",
			Status:    review.Satisfied,
			Rationale: "The declared test-passes criterion was met by the lifecycle.",
			Citations: []review.Citation{{Path: "task.go", Line: 1}},
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

	// Step 8: Simulate a real PR merge into the branch captured before feature
	// checkout. The issue remains done until sync observes that merge (I6).
	t.Logf("Step 8: Merge into %s and sync", mainBranch)
	gitRunInDir(t, h.WorkDir, "checkout", mainBranch)
	gitRunInDir(t, h.WorkDir, "-c", "core.hooksPath=/dev/null", "merge", "--no-ff",
		featureBranch, "-m", "Merge "+featureBranch)

	// Push to origin to simulate PR merge being merged
	gitRunInDir(t, h.WorkDir, "push", "-u", "origin", mainBranch)
	assertMaterializedField(t, h, "status", "done")

	// Run sync to detect merge
	out, err = h.RunArm("sync", "--repo", h.WorkDir, "--into", mainBranch)
	require.NoError(t, err, "sync failed: %s", out)
	assertMaterializedField(t, h, "status", "merged")

	t.Logf("Happy-path lifecycle test completed successfully")
}

func assertMaterializedField(t *testing.T, h *e2eharness.Harness, field, want string) {
	t.Helper()
	got := materializedField(t, h, "TEST-001", field)
	assert.Equal(t, want, got, "materialized %s", field)
}

func materializedField(t *testing.T, h *e2eharness.Harness, issueID, field string) string {
	t.Helper()
	out, err := h.RunArm("materialize", "--repo", h.WorkDir)
	require.NoError(t, err, "materialize failed: %s", out)
	out, err = h.RunArm("show", "--repo", h.WorkDir, "--issue", issueID, "--field", field)
	require.NoError(t, err, "show %s failed: %s", field, out)
	return strings.TrimSpace(out)
}

// buildArmBinaryOnce caches the result of the first buildArmBinary invocation
// so parallel tests in this package share a single build rather than racing
// `make build` writes to the same repo-level bin/arm path.
var buildArmBinaryOnce struct {
	sync.Once
	path string
	err  error
}

// buildArmBinary compiles the arm binary for use in tests. It returns the path
// to the built binary or fails the test if compilation fails.
//
// Several tests that call this helper (TestArtifactPipelineUsesCLI,
// TestHappyPathLifecycle, and the scenario tests) run with t.Parallel(). Each
// invocation used to run its own `make build`, all writing the same
// repo-level bin/arm path concurrently, which could corrupt the binary or
// race with an in-flight execution. sync.Once ensures the build happens
// exactly once per test binary run, and every caller (parallel or not)
// observes the same completed build.
func buildArmBinary(t *testing.T) string {
	t.Helper()

	buildArmBinaryOnce.Do(func() {
		// Find the repo root by looking for go.mod
		cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
		rootOutput, err := cmd.Output()
		if err != nil {
			buildArmBinaryOnce.err = fmt.Errorf("failed to find repo root: %w", err)
			return
		}

		repoRoot := strings.TrimSpace(string(rootOutput))
		binPath := filepath.Join(repoRoot, "bin", "arm")

		// Build the binary
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

// gitRunInDir runs a git command in a specified directory.
func gitRunInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed in %s: %s", args, dir, out)
}

// gitGetCurrentBranch returns the name of the current branch.
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
