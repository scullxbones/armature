package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCmdInDir creates a command to run in a specific directory.
//
//nolint:unparam // name is always "git" but kept for clarity and future flexibility
func newCmdInDir(dir string, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	return cmd
}

func TestReviewPrepareCommand_Success(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker for arm commands
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create another commit to have a range
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	// Get base SHA
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 2")
	// Get head SHA
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Run review prepare
	out, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	// Verify output is valid JSON
	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle)
	require.NoError(t, err, "output should be valid JSON")

	// Verify bundle has expected fields
	assert.Equal(t, "task-01", bundle.Issue.ID)
	assert.Equal(t, review.SchemaVersion, bundle.SchemaVersion)
	assert.NotEmpty(t, bundle.BundleID)
	assert.NotEmpty(t, bundle.Fingerprints.Contract)
	assert.NotEmpty(t, bundle.Fingerprints.Delivery)
	assert.Equal(t, base, bundle.Delivery.BaseSHA)
	assert.Equal(t, head, bundle.Delivery.HeadSHA)
}

func TestReviewPrepareCommand_RequiresIssue(t *testing.T) {
	repo := setupRepoWithTask(t)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "test commit")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "prepare", "--repo", repo, "--base", "HEAD~1", "--head", "HEAD"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue")
}

func TestReviewPrepareCommand_RequiresBase(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "prepare", "--repo", repo, "--issue", "task-01", "--head", "HEAD"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "base")
}

func TestReviewPrepareCommand_RequiresHead(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "prepare", "--repo", repo, "--issue", "task-01", "--base", "HEAD~1"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "head")
}

func TestReviewPrepareCommand_OutputFile(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create commits
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 2")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Write to file
	outputFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", outputFile)
	require.NoError(t, err)

	// Verify file was created and contains valid JSON
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal(data, &bundle)
	require.NoError(t, err)
	assert.Equal(t, "task-01", bundle.Issue.ID)
}

func TestReviewRecordCommand_Success(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create and prepare a review bundle
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 2")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle)
	require.NoError(t, err)

	// Create a conformance assessment
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation is complete and tested.",
			},
		},
	}

	// Write assessment to file
	assessmentFile := filepath.Join(repo, "assessment.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(assessmentFile, assessmentJSON, 0o644)
	require.NoError(t, err)

	// Record the assessment
	out, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err)

	// Verify response indicates success
	assert.Contains(t, out, "recorded")
}

func TestReviewRecordCommand_IsDuplicate(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create and prepare a review bundle
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 2")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle)
	require.NoError(t, err)

	// Create a conformance assessment
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation is complete and tested.",
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(assessmentFile, assessmentJSON, 0o644)
	require.NoError(t, err)

	// Record the assessment first time
	_, err = runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err)

	// Record the same assessment again — should be idempotent
	out, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err, "duplicate record should succeed (idempotent)")

	// Verify response indicates duplicate
	assert.Contains(t, out, "duplicate")
}

func TestReviewRecordCommand_RequiresIssue(t *testing.T) {
	repo := setupRepoWithTask(t)

	assessmentFile := filepath.Join(repo, "assessment.json")
	err := os.WriteFile(assessmentFile, []byte("{}"), 0o644)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--assessment", assessmentFile})

	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue")
}

func TestReviewRecordCommand_RequiresAssessment(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01"})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assessment")
}

func TestReviewRecordCommand_InvalidJSON(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Write invalid JSON
	assessmentFile := filepath.Join(repo, "assessment.json")
	err = os.WriteFile(assessmentFile, []byte("not valid json"), 0o644)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01", "--assessment", assessmentFile})

	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestReviewRecordCommand_ValidationError(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Write assessment missing required fields
	assessmentFile := filepath.Join(repo, "assessment.json")
	invalidAssessment := map[string]interface{}{
		"schema_version": review.SchemaVersion,
		// Missing: bundle_id, contract_fingerprint, delivery_fingerprint, results
	}
	data, err := json.Marshal(invalidAssessment)
	require.NoError(t, err)
	err = os.WriteFile(assessmentFile, data, 0o644)
	require.NoError(t, err)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01", "--assessment", assessmentFile})

	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation")
}
