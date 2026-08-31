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

	"github.com/scullxbones/armature/internal/adapters"
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

	// Add a file so the delivery is non-empty
	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
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

	// Create commits with an actual file change
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
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

	// Create and prepare a review bundle with an actual file change
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
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
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
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

func TestReviewRecordCommand_WithCitation(t *testing.T) {
	// An assessment that contains a file:line citation must be recorded successfully
	// when no --bundle flag is passed (diff-index citation checking is opt-in via --bundle).
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle)
	require.NoError(t, err)

	// Assessment includes a file:line citation that is NOT in the empty diff.
	// record must accept it without error.
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "internal/review/types.go", Line: 42},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_with_citation.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(assessmentFile, assessmentJSON, 0o644)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err, "record must succeed even when assessment contains file:line citations")
	assert.Contains(t, out, "recorded")
}

func TestReviewRecordCommand_IsDuplicate(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create and prepare a review bundle with an actual file change
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
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
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
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

func TestReviewRecordCommand_BundleValidatesCitationCoordinates(t *testing.T) {
	// When --bundle is passed to record, invalid citation coordinates must be rejected.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare and save bundle to file.
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Assessment with a citation that does NOT exist in the diff (line 9999 of impl.go).
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 9999}, // does not exist in the diff
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_cite.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// Without --bundle, record should succeed (no diff-index check).
	_, err = runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err, "record without --bundle must not perform diff-index citation checking")

	// With --bundle, invalid citation coordinates must be rejected.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject citations not present in the diff")
	assert.Contains(t, err.Error(), "citation")
}

func TestReviewRecordCommand_ContractFingerprintMismatch(t *testing.T) {
	// If the assessment's ContractFingerprint doesn't match the issue's contract,
	// record must reject the assessment with a clear error.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle))

	// Use a deliberately wrong contract fingerprint.
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "All requirements met.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_fp.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01", "--assessment", assessmentFile})

	err = cmd.Execute()
	require.Error(t, err, "record must reject assessment with mismatched contract fingerprint")
	assert.Contains(t, err.Error(), "contract fingerprint")
}

func TestReviewRecordCommand_BundleIDMismatch(t *testing.T) {
	// When --bundle is passed, record must reject an assessment whose bundle_id
	// doesn't match the bundle's bundle_id.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare and save bundle to file.
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Assessment with a mismatched bundle_id.
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "wrong-bundle-id-xyz",
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_bundle_id.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// With --bundle, mismatched bundle_id must be rejected.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched bundle_id")
	assert.Contains(t, err.Error(), "bundle_id")
}

func TestReviewRecordCommand_DeliveryFingerprintMismatch(t *testing.T) {
	// When --bundle is passed, record must reject an assessment whose delivery_fingerprint
	// doesn't match the bundle's delivery fingerprint.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare and save bundle to file.
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Assessment with a mismatched delivery_fingerprint.
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_delivery_fp.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// With --bundle, mismatched delivery_fingerprint must be rejected.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched delivery_fingerprint")
	assert.Contains(t, err.Error(), "delivery_fingerprint")
}

func TestReviewRecordCommand_BundleContractFingerprintMismatch(t *testing.T) {
	// When --bundle is passed, record must reject an assessment whose contract_fingerprint
	// doesn't match the bundle's contract fingerprint.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare and save bundle to file.
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Assessment with a mismatched contract_fingerprint.
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_contract_fp.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// With --bundle, mismatched contract_fingerprint must be rejected with an error
	// referencing the bundle contract_fingerprint field name.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched contract_fingerprint")
	assert.Contains(t, err.Error(), "contract_fingerprint")
}

func TestReviewRecordCommand_BundleIssueMismatch(t *testing.T) {
	// When --bundle is passed, record must reject an assessment whose bundle was prepared
	// for a different issue. This prevents cross-issue assessment recording.
	repo := setupRepoWithTwoTasks(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare bundle for task-01
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Verify bundle was prepared for task-01
	assert.Equal(t, "task-01", bundle.Issue.ID)

	// Create assessment with task-01 bundle metadata
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_task01.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// Try to record assessment prepared for task-01 against task-02 with bundle verification.
	// This must be rejected because bundle.Issue.ID (task-01) != --issue (task-02).
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-02",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record must reject assessment with bundle prepared for different issue")
	assert.Contains(t, err.Error(), "bundle was prepared for issue")
	assert.Contains(t, err.Error(), "task-01")
	assert.Contains(t, err.Error(), "task-02")
}

func TestReviewRecordCommand_BundleIssueMatch(t *testing.T) {
	// When --bundle is passed, record must succeed if bundle.Issue.ID matches --issue.
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — add implementation")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Prepare bundle for task-01
	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	// Create assessment with matching issue
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Implementation verified.",
				Citations: []review.Citation{
					{Path: "impl.go", Line: 1},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_matched.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	// Record assessment with matching issue ID and bundle.
	// This must succeed.
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.NoError(t, err, "record must succeed when bundle.Issue.ID matches --issue")
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

// TestReviewRecordCommand_AllowsUnknownAssessmentRootField_REQ_TOPTIER_S3
// verifies that an assessment with an unrecognized root field is not
// rejected purely for that reason. The canonical validator
// (docs/schemas/conformance-assessment.schema.json) does not set
// additionalProperties: false, so a schema-valid assessment may legitimately
// carry an extension/metadata field. The payload below is still missing
// required fields, so recording still fails, but not with a "field" decode
// error.
func TestReviewRecordCommand_AllowsUnknownAssessmentRootField_REQ_TOPTIER_S3(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	assessmentFile := filepath.Join(repo, "assessment.json")
	require.NoError(t, os.WriteFile(assessmentFile, []byte(`{"unexpected":true}`), 0o644))

	_, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown field")
}

// TestReviewRecordCommand_AllowsUnknownBundleRootField_REQ_TOPTIER_S3 mirrors
// the assessment case for review bundles: docs/schemas/review-bundle.schema.json
// does not set additionalProperties: false either, so an unrecognized root
// field alone must not be rejected as an "unknown field" decode error.
func TestReviewRecordCommand_AllowsUnknownBundleRootField_REQ_TOPTIER_S3(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	assessmentFile := filepath.Join(repo, "assessment.json")
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle",
		ContractFingerprint: "contract",
		DeliveryFingerprint: "delivery",
		Results: []review.CriterionResult{{
			ID: "definition_of_done", Status: review.Satisfied, Rationale: "strict decoding is enforced",
			Citations: []review.Citation{{Path: "impl.go", Line: 1}},
		}},
	}
	assessmentJSON, err := json.Marshal(assessment)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))
	bundleFile := filepath.Join(repo, "bundle.json")
	require.NoError(t, os.WriteFile(bundleFile, []byte(`{"unexpected":true}`), 0o644))

	_, err = runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile, "--bundle", bundleFile)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown field")
}

func TestReviewRecordCommand_RejectsTrailingAssessmentJSON_REQ_TOPTIER_S3(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	assessmentFile := filepath.Join(repo, "assessment.json")
	require.NoError(t, os.WriteFile(assessmentFile, []byte(`{"schema_version":1} {"extra":true}`), 0o644))

	_, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing")
}

func TestReviewRecordCommand_ValidationError(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Write assessment missing required fields
	assessmentFile := filepath.Join(repo, "assessment.json")
	invalidAssessment := map[string]any{
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

func TestReviewCommitsCommand_Success(t *testing.T) {
	repo := setupRepoWithTask(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	out, err := runTrls(t, repo, "review", "commits", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "Found 1 commit(s) for issue task-01")
	assert.Contains(t, out, "feat(task-01): add feature")
}

func TestReviewCommitsCommand_JSONFormat(t *testing.T) {
	repo := setupRepoWithTask(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	cmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetArgs([]string{"review", "commits", "task-01", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd.Execute())

	var entries []adapters.LogEntry
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(outBuf.Bytes()), &entries))
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Subject, "feat(task-01): add feature")
}

func TestReviewCommitsCommand_NoCommitsFound(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "review", "commits", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "No commits found for issue task-01")
}

func TestReviewCommitsCommand_RequiresIssue(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "commits", "--repo", repo})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue")
}

func TestReviewCommitsCommand_PositionalAndFlagConflict(t *testing.T) {
	repo := setupRepoWithTask(t)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "commits", "task-01", "--issue", "task-02", "--repo", repo})

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting issue ID")
}

func TestReviewCommitsCommand_PositionalAndFlagAgree(t *testing.T) {
	repo := setupRepoWithTask(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	out, err := runTrls(t, repo, "review", "commits", "task-01", "--issue", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "Found 1 commit(s) for issue task-01")
}

func TestReviewCommitsCommand_BranchFlag(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Create a side branch with a commit for task-01, then switch back to main
	// so HEAD no longer contains it — --branch should still find it.
	run(t, repo, "git", "checkout", "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	out, err := runTrls(t, repo, "review", "commits", "task-01", "--branch", "task/task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "Found 1 commit(s) for issue task-01")
}

// TestReviewCommitsCommand_DefaultBranchResolvesFromWorktree verifies that
// `arm review commits <issue-id>` with the default --branch ("HEAD") resolves
// against the invoking git worktree's own checked-out branch, not the parent
// repo root's branch. `ctx.RepoPath` resolves to the *parent* repo root when
// this command runs from inside a linked git worktree (the standard
// `arm claim --worktree` delivery flow) -- see config.ResolveContext's
// worktree handling. Passing "HEAD" straight through to git.LogBranch would
// therefore report the parent repo's checked-out branch (which may have no
// commits for the issue at all), silently returning wrong/empty results
// with exit 0 instead of the worktree's own commits.
func TestReviewCommitsCommand_DefaultBranchResolvesFromWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Record whatever branch the parent repo is on (it will remain checked
	// out there, untouched by the worktree add below) so the assertion
	// isn't tied to init.defaultBranch naming ("main" vs "master").
	parentBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	require.NotEmpty(t, parentBranch)

	// Create a linked worktree on its own branch, with a commit for task-01
	// that only exists on that branch -- the parent repo's checked-out
	// branch never gets this commit.
	worktreeDir := t.TempDir()
	run(t, repo, "git", "worktree", "add", worktreeDir, "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "impl.go"), []byte("package main\n"), 0o644))
	run(t, worktreeDir, "git", "add", "impl.go")
	run(t, worktreeDir, "git", "commit", "-m", "feat(task-01): add feature")

	// Sanity check: the parent repo's own branch does NOT have this commit.
	parentOut, err := runTrls(t, repo, "review", "commits", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, parentOut, "No commits found for issue task-01",
		"parent repo's checked-out branch should not see the worktree-only commit")

	// Invoke `review commits` pointed at the worktree, with --branch left at
	// its default. It must resolve the worktree's own branch (task/task-01),
	// not the parent repo's.
	cmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetArgs([]string{"review", "commits", "task-01", "--repo", worktreeDir, "--format", "human"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, outBuf.String(), "Found 1 commit(s) for issue task-01",
		"review commits from inside a worktree should find the worktree's own commit without an explicit --branch")
}

// runGitOutput runs a git command in dir and returns its stdout.
func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err, "git %v failed", args)
	return string(out)
}

// TestReviewPrepare_CoordinatorWaveScope verifies that review bundles use task-specific
// commit ranges, not wave-combined ranges. This is critical for the coordinator workflow:
// when multiple tasks complete in a wave, each task's review should be scoped to only its own
// changes, not the cumulative wave diff.
//
// Scenario:
//
//	Wave with 2 independent tasks (TASK-A, TASK-B)
//	WAVE_BASE = commit 0
//	TASK-A commits changes to file_a.go → SHA_A
//	TASK-B commits changes to file_b.go → SHA_B (on top of SHA_A)
//
// OLD (broken): review prepare --base WAVE_BASE --head HEAD
//
//	→ TASK-A's bundle includes file_b.go (wrong scope)
//	→ TASK-B's bundle includes file_a.go (wrong scope)
//
// NEW (correct): review prepare with task-specific range
//
//	→ TASK-A uses --base WAVE_BASE --head SHA_A → only file_a.go
//	→ TASK-B uses --base SHA_A --head SHA_B → only file_b.go
func TestReviewPrepare_CoordinatorWaveScope(t *testing.T) {
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create TASK-B (parallel to TASK-A from setupRepoWithTask)
	_, err = runTrls(t, repo, "create", "--title", "Task B", "--type", "task", "--id", "task-02",
		"--dod", "Task B implementation complete")
	require.NoError(t, err)

	// Record wave base before any task commits
	waveBaseSHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	waveBaseSHAOut, err := waveBaseSHACmd.Output()
	require.NoError(t, err)
	waveBaseSHA := strings.TrimSpace(string(waveBaseSHAOut))

	// Simulate TASK-A worker: create commit for file_a.go
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file_a.go"), []byte("package main\n\nfunc A() {}\n"), 0o644))
	run(t, repo, "git", "add", "file_a.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): implement file_a.go")

	taskASHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	taskASHAOut, err := taskASHACmd.Output()
	require.NoError(t, err)
	taskASHA := strings.TrimSpace(string(taskASHAOut))

	// Simulate TASK-B worker: create commit for file_b.go (on top of TASK-A)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file_b.go"), []byte("package main\n\nfunc B() {}\n"), 0o644))
	run(t, repo, "git", "add", "file_b.go")
	run(t, repo, "git", "commit", "-m", "feat(task-02): implement file_b.go")

	taskBSHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	taskBSHAOut, err := taskBSHACmd.Output()
	require.NoError(t, err)
	taskBSHA := strings.TrimSpace(string(taskBSHAOut))

	// Verify commit ancestry: waveBase → taskA → taskB
	assert.NotEqual(t, waveBaseSHA, taskASHA, "TASK-A should have created a new commit")
	assert.NotEqual(t, taskASHA, taskBSHA, "TASK-B should have created a new commit")

	// OLD APPROACH (broken): use WAVE_BASE..HEAD for all tasks
	// This would show both file_a.go and file_b.go in each task's bundle
	oldBundleA, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", waveBaseSHA, "--head", taskBSHA)
	require.NoError(t, err, "prepare bundle with wave range should succeed")

	var bundleA review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(oldBundleA)), &bundleA)
	require.NoError(t, err)

	// In the broken approach, TASK-A's bundle spans waveBase..taskBSHA,
	// so it includes changes from both TASK-A and TASK-B
	assert.Equal(t, waveBaseSHA, bundleA.Delivery.BaseSHA, "bundle should use wave base as-is (broken behavior)")
	assert.Equal(t, taskBSHA, bundleA.Delivery.HeadSHA, "bundle head is combined wave HEAD (broken behavior)")

	// NEW APPROACH (correct): use task-specific ranges
	// TASK-A should see only file_a.go (waveBase..taskA)
	newBundleA, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", waveBaseSHA, "--head", taskASHA)
	require.NoError(t, err)

	var correctBundleA review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(newBundleA)), &correctBundleA)
	require.NoError(t, err)

	assert.Equal(t, waveBaseSHA, correctBundleA.Delivery.BaseSHA, "TASK-A bundle should use wave base")
	assert.Equal(t, taskASHA, correctBundleA.Delivery.HeadSHA, "TASK-A bundle should use task-specific head")

	// TASK-B should see only file_b.go (taskA..taskB)
	newBundleB, err := runTrls(t, repo, "review", "prepare", "--issue", "task-02", "--base", taskASHA, "--head", taskBSHA)
	require.NoError(t, err)

	var correctBundleB review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(newBundleB)), &correctBundleB)
	require.NoError(t, err)

	assert.Equal(t, taskASHA, correctBundleB.Delivery.BaseSHA, "TASK-B bundle should use previous task's head as base")
	assert.Equal(t, taskBSHA, correctBundleB.Delivery.HeadSHA, "TASK-B bundle should use task-specific head")

	// Verify that the fingerprints differ (they encode the different diffs)
	assert.NotEqual(t, bundleA.Fingerprints.Delivery, correctBundleA.Fingerprints.Delivery,
		"delivery fingerprint should differ when using different commit ranges")
}

// TestReviewPrepareCommand_FindsActivityLogInDeliveryWorktree_REQ_EXECEV_C1
// verifies the fix for C1: `arm review prepare` run inside a linked delivery
// worktree must attach the Activity section from *that worktree's* private
// git dir (<repo>/.git/worktrees/<name>/armature-activity.log), not from the
// parent repo root's .git/armature-activity.log. Before the fix, ctx.RepoPath
// is resolved to the parent repo root for worktree invocations, so the command
// would either miss the log entirely or (worse) attach an unrelated session's
// activity log as evidence for this delivery.
func TestReviewPrepareCommand_FindsActivityLogInDeliveryWorktree_REQ_EXECEV_C1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Resolve the worktree's actual (private) git dir.
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

	// A decoy activity log at the parent repo root's .git dir, from an unrelated
	// session. If C1 regresses, this is the log the command would wrongly read.
	decoyContent := []byte(`{"timestamp":"2020-01-01T00:00:00Z","command":"rm -rf /",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"deadbeef","output_hash":"decoy"}` + "\n")
	decoyLogPath := filepath.Join(repo, ".git", "armature-activity.log")
	require.NoError(t, os.WriteFile(decoyLogPath, decoyContent, 0o600))

	// The genuine activity log, written to the worktree's own private git dir.
	realContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"go build ./...",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"realsha","output_hash":"real"}` + "\n")
	realLogPath := filepath.Join(worktreeGitDir, "armature-activity.log")
	require.NoError(t, os.WriteFile(realLogPath, realContent, 0o600)) //nolint:gosec // G703: fixed test-controlled path, not user input

	// Create a commit range inside the worktree.
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "impl.go"), []byte("package main\n"), 0o644))
	run(t, worktreeDir, "git", "add", "impl.go")
	run(t, worktreeDir, "git", "commit", "-m", "implementation")
	baseCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD~1")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))
	headCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	out, err := runTrls(t, worktreeDir, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle))

	require.NotNil(t, bundle.Activity, "activity section must be attached from the worktree's own git dir")
	assert.Equal(t, review.FingerprintActivity(realContent), bundle.Activity.Digest,
		"activity digest must come from the worktree's own log, not the parent repo's decoy log")
}

// TestReviewPrepareCommand_MismatchedBinding_NoActivityLog_REQ_EXECEV_F1 verifies that when
// review prepare is run from a worktree bound to one issue (task-02) but preparing for a
// different issue (task-01), the activity log from the mismatched worktree is NOT attached
// to the bundle. This prevents a bundle for issue A from carrying issue B's activity log
// and upstream record validation from accepting evidence against the wrong issue.
func TestReviewPrepareCommand_MismatchedBinding_NoActivityLog_REQ_EXECEV_F1(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a worktree bound to task-02
	worktreeDir := filepath.Join(repo, ".worktrees", "task-02")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-02", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	// Resolve the worktree's actual git dir.
	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

	// Write an activity log to the task-02 worktree's git dir
	activityContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make build",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"abc123","output_hash":"hash"}` + "\n")
	activityLogPath := filepath.Join(worktreeGitDir, "armature-activity.log")
	require.NoError(t, os.WriteFile(activityLogPath, activityContent, 0o600)) //nolint:gosec // G703: fixed test-controlled path

	// Create a commit range inside the worktree.
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "impl.go"), []byte("package main\n"), 0o644))
	run(t, worktreeDir, "git", "add", "impl.go")
	run(t, worktreeDir, "git", "commit", "-m", "implementation")
	baseCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD~1")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))
	headCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Run review prepare for task-01 (NOT task-02) from the worktree bound to task-02.
	// The binding resolves to task-02, but we're preparing for task-01.
	// The activity log should NOT be attached because the binding's issue ID doesn't match.
	out, err := runTrls(t, worktreeDir, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle))

	// The bundle should be for task-01
	assert.Equal(t, "task-01", bundle.Issue.ID)

	// The activity log should NOT be attached because the binding (task-02) doesn't match
	// the issue being prepared (task-01). This prevents bundle for task-01 from carrying
	// task-02's activity log, which would cause record validation to check against the wrong issue's evidence.
	assert.Nil(t, bundle.Activity, "activity log must not be attached when binding issue ID does not match the prepared issue ID")
}

// TestReviewPrepareCommand_EnvBoundSession_AttachesActivityLog_REQ_EXECEV verifies that
// review prepare attaches the activity log for a session bound via the ARMATURE_ISSUE_ID
// env var (no armature-issue-id file present), not just a file-based binding. Capture
// (cmd/armature/harness_hook.go's resolveIssueBinding) resolves bindings via file-then-env,
// so prepare's gate must use the same resolution or an env-bound session's legitimately
// captured activity is silently dropped even though it matches the issue being prepared.
func TestReviewPrepareCommand_EnvBoundSession_AttachesActivityLog_REQ_EXECEV(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	// Claim without --worktree flag semantics that write the binding file; instead we
	// simulate an env-only-bound session by claiming into worktreeDir and then removing
	// the armature-issue-id file, leaving only the env var as the binding source.
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

	// Remove the file-based binding so only the env var resolves the issue ID, and also
	// clear the legacy task-id file for the same reason.
	require.NoError(t, os.Remove(filepath.Join(worktreeGitDir, "armature-issue-id"))) //nolint:gosec // G703: fixed test-controlled path, not user input

	activityContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"go build ./...",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"envsha","output_hash":"env"}` + "\n")
	activityLogPath := filepath.Join(worktreeGitDir, "armature-activity.log")
	require.NoError(t, os.WriteFile(activityLogPath, activityContent, 0o600)) //nolint:gosec // G703: fixed test-controlled path

	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "impl.go"), []byte("package main\n"), 0o644))
	run(t, worktreeDir, "git", "add", "impl.go")
	run(t, worktreeDir, "git", "commit", "-m", "implementation")
	baseCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD~1")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))
	headCmd := newCmdInDir(worktreeDir, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	t.Setenv("ARMATURE_ISSUE_ID", "task-01")

	out, err := runTrls(t, worktreeDir, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle))

	require.NotNil(t, bundle.Activity, "activity section must be attached for an env-bound session whose ARMATURE_ISSUE_ID matches the prepared issue")
	assert.Equal(t, review.FingerprintActivity(activityContent), bundle.Activity.Digest)
}
