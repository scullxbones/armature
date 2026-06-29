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

func TestReview_SingleBranchLifecycle(t *testing.T) {
	// Full end-to-end lifecycle test: bootstrap → create → claim → deliver → prepare → assess → record → rematerialize
	repo := setupRepoWithTask(t)

	// Initialize worker
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Claim the issue
	worktreePath := filepath.Join(t.TempDir(), "claim-worktree")
	_, err = runTrls(t, repo, "claim", "--issue", "task-01", "--worktree", worktreePath)
	require.NoError(t, err)

	// Materialize to establish state
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Transition to in-progress
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "in-progress")
	require.NoError(t, err)

	// Create commits for the delivery range — must include an actual file change.
	run(t, repo, "git", "commit", "--allow-empty", "-m", "commit 1")
	baseCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	baseOut, err := baseCmd.Output()
	require.NoError(t, err)
	base := strings.TrimSpace(string(baseOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "commit 2 — task-01 delivery")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	// Transition to done with outcome
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "done", "--outcome", "Implementation complete", "--force")
	require.NoError(t, err)

	// Prepare review bundle
	bundleOut, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(bundleOut)), &bundle)
	require.NoError(t, err, "output should be valid JSON")

	// Verify bundle structure
	assert.Equal(t, "task-01", bundle.Issue.ID)
	assert.Equal(t, review.SchemaVersion, bundle.SchemaVersion)
	assert.NotEmpty(t, bundle.BundleID)
	assert.NotEmpty(t, bundle.Fingerprints.Contract)
	assert.NotEmpty(t, bundle.Fingerprints.Delivery)

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
				Rationale: "Implementation verified and meets the task definition of done.",
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
	recordOut, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err)

	// Verify response indicates success
	assert.Contains(t, recordOut, "recorded")

	// Materialize again to include assessment ops
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Show the issue and verify review output
	showOut, err := runTrls(t, repo, "show", "--issue", "task-01")
	require.NoError(t, err)

	// Verify Review: line is in output
	assert.Contains(t, showOut, "Review:", "show output should include Review line")

	// Verify no raw assessment file is persisted in .armature/
	assessmentDir := filepath.Join(repo, ".armature", "assessments")
	_, err = os.Stat(assessmentDir)
	if err == nil {
		entries, err := os.ReadDir(assessmentDir)
		assert.Empty(t, entries, "no detailed assessment files should persist under .armature/")
		require.NoError(t, err)
	}
	// If the directory doesn't exist, that's also correct
}
