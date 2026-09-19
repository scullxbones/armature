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
	"github.com/scullxbones/armature/internal/output"
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

	out, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle)
	require.NoError(t, err, "output should be valid JSON")

	assert.Equal(t, "task-01", bundle.Issue.ID)
	assert.Equal(t, review.SchemaVersion, bundle.SchemaVersion)
	assert.NotEmpty(t, bundle.BundleID)
	assert.NotEmpty(t, bundle.Fingerprints.Contract)
	assert.NotEmpty(t, bundle.Fingerprints.Delivery)
	assert.Equal(t, base, bundle.Delivery.BaseSHA)
	assert.Equal(t, head, bundle.Delivery.HeadSHA)
}

func TestReviewPrepareArtifactModeIsStdoutOnly_REQ_AOC_S1_T3(t *testing.T) {
	t.Parallel()

	cmd := newReviewPrepareCmd()
	require.Equal(t, output.CitationReviewBundleSchema, cmd.Annotations[output.ArtifactCitationKey])
	require.Equal(t, output.ChannelArtifactOutput, output.Classify(cmd.Annotations),
		"review prepare with --output unset is Artifact Output")
	require.Equal(t, output.ChannelAgentFacing, output.ClassifyFlags(cmd.Annotations, map[string]bool{"output": true}),
		"review prepare --output <file> remains agent-facing")
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

	outputFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", outputFile)
	require.NoError(t, err)

	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	err = json.Unmarshal(data, &bundle)
	require.NoError(t, err)
	assert.Equal(t, "task-01", bundle.Issue.ID)
}

func TestReviewRecordCommand_Success(t *testing.T) {
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

	out, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err)

	assert.Contains(t, out, "recorded")
}

func TestReviewRecordCommand_WithCitation(t *testing.T) {
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

	_, err = runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err, "duplicate record should succeed (idempotent)")

	assert.Contains(t, out, "duplicate")
}

func TestReviewRecordCommand_BundleValidatesCitationCoordinates(t *testing.T) {
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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

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
					{Path: "impl.go", Line: 9999},
				},
			},
		},
	}

	assessmentFile := filepath.Join(repo, "assessment_bad_cite.json")
	assessmentJSON, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentFile, assessmentJSON, 0o644))

	_, err = runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.NoError(t, err, "record without --bundle must not perform diff-index citation checking")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject citations not present in the diff")
	assert.Contains(t, err.Error(), "citation")
}

func TestReviewRecordCommand_ContractFingerprintMismatch(t *testing.T) {
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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

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

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched bundle_id")
	assert.Contains(t, err.Error(), "bundle_id")
}

func TestReviewRecordCommand_DeliveryFingerprintMismatch(t *testing.T) {
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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

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

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched delivery_fingerprint")
	assert.Contains(t, err.Error(), "delivery_fingerprint")
}

func TestReviewRecordCommand_BundleContractFingerprintMismatch(t *testing.T) {
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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

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

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01",
		"--assessment", assessmentFile, "--bundle", bundleFile})
	err = cmd.Execute()
	require.Error(t, err, "record with --bundle must reject assessment with mismatched contract_fingerprint")
	assert.Contains(t, err.Error(), "contract_fingerprint")
}

func TestReviewRecordCommand_BundleIssueMismatch(t *testing.T) {
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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	assert.Equal(t, "task-01", bundle.Issue.ID)

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

	bundleFile := filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

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

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

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

func TestReviewRecordCommand_AllowsUnknownAssessmentRootField_REQ_TOPTIER_S3(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	assessmentFile := filepath.Join(repo, "assessment.json")
	require.NoError(t, os.WriteFile(assessmentFile, []byte(`{"unexpected":true}`), 0o644))

	_, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", assessmentFile)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown field")
}

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

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	assessmentFile := filepath.Join(repo, "assessment.json")
	invalidAssessment := map[string]any{
		"schema_version": review.SchemaVersion,
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

func TestReviewCommitsCommand_JSONFormat_REQ_AOC_S2_T4(t *testing.T) {
	repo := setupRepoWithTask(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	cmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetArgs([]string{"review", "commits", "task-01", "--repo", repo, "--format", "json"})
	require.NoError(t, cmd.Execute())

	decoded := decodeContractEnvelope(t, outBuf.String(), "commits")
	var entries []adapters.LogEntry
	require.NoError(t, json.Unmarshal(decoded["commits"], &entries))
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

	run(t, repo, "git", "checkout", "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): add feature")

	out, err := runTrls(t, repo, "review", "commits", "task-01", "--branch", "task/task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "Found 1 commit(s) for issue task-01")
}

func TestReviewCommitsCommand_DefaultBranchResolvesFromWorktree(t *testing.T) {
	repo := setupRepoWithTask(t)

	parentBranch := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	require.NotEmpty(t, parentBranch)

	worktreeDir := t.TempDir()
	run(t, repo, "git", "worktree", "add", worktreeDir, "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "impl.go"), []byte("package main\n"), 0o644))
	run(t, worktreeDir, "git", "add", "impl.go")
	run(t, worktreeDir, "git", "commit", "-m", "feat(task-01): add feature")

	parentOut, err := runTrls(t, repo, "review", "commits", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, parentOut, "No commits found for issue task-01",
		"parent repo's checked-out branch should not see the worktree-only commit")

	cmd := newRootCmd()
	outBuf := new(bytes.Buffer)
	cmd.SetOut(outBuf)
	cmd.SetArgs([]string{"review", "commits", "task-01", "--repo", worktreeDir, "--format", "human"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, outBuf.String(), "Found 1 commit(s) for issue task-01",
		"review commits from inside a worktree should find the worktree's own commit without an explicit --branch")
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err, "git %v failed", args)
	return string(out)
}

func TestReviewPrepare_CoordinatorWaveScope(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--title", "Task B", "--type", "task", "--id", "task-02",
		"--dod", "Task B implementation complete")
	require.NoError(t, err)

	waveBaseSHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	waveBaseSHAOut, err := waveBaseSHACmd.Output()
	require.NoError(t, err)
	waveBaseSHA := strings.TrimSpace(string(waveBaseSHAOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file_a.go"), []byte("package main\n\nfunc A() {}\n"), 0o644))
	run(t, repo, "git", "add", "file_a.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): implement file_a.go")

	taskASHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	taskASHAOut, err := taskASHACmd.Output()
	require.NoError(t, err)
	taskASHA := strings.TrimSpace(string(taskASHAOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file_b.go"), []byte("package main\n\nfunc B() {}\n"), 0o644))
	run(t, repo, "git", "add", "file_b.go")
	run(t, repo, "git", "commit", "-m", "feat(task-02): implement file_b.go")

	taskBSHACmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	taskBSHAOut, err := taskBSHACmd.Output()
	require.NoError(t, err)
	taskBSHA := strings.TrimSpace(string(taskBSHAOut))

	assert.NotEqual(t, waveBaseSHA, taskASHA, "TASK-A should have created a new commit")
	assert.NotEqual(t, taskASHA, taskBSHA, "TASK-B should have created a new commit")

	oldBundleA, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", waveBaseSHA, "--head", taskBSHA)
	require.NoError(t, err, "prepare bundle with wave range should succeed")

	var bundleA review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(oldBundleA)), &bundleA)
	require.NoError(t, err)

	assert.Equal(t, waveBaseSHA, bundleA.Delivery.BaseSHA, "bundle should use wave base as-is (broken behavior)")
	assert.Equal(t, taskBSHA, bundleA.Delivery.HeadSHA, "bundle head is combined wave HEAD (broken behavior)")

	newBundleA, err := runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", waveBaseSHA, "--head", taskASHA)
	require.NoError(t, err)

	var correctBundleA review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(newBundleA)), &correctBundleA)
	require.NoError(t, err)

	assert.Equal(t, waveBaseSHA, correctBundleA.Delivery.BaseSHA, "TASK-A bundle should use wave base")
	assert.Equal(t, taskASHA, correctBundleA.Delivery.HeadSHA, "TASK-A bundle should use task-specific head")

	newBundleB, err := runTrls(t, repo, "review", "prepare", "--issue", "task-02", "--base", taskASHA, "--head", taskBSHA)
	require.NoError(t, err)

	var correctBundleB review.ReviewBundle
	err = json.Unmarshal([]byte(strings.TrimSpace(newBundleB)), &correctBundleB)
	require.NoError(t, err)

	assert.Equal(t, taskASHA, correctBundleB.Delivery.BaseSHA, "TASK-B bundle should use previous task's head as base")
	assert.Equal(t, taskBSHA, correctBundleB.Delivery.HeadSHA, "TASK-B bundle should use task-specific head")

	assert.NotEqual(t, bundleA.Fingerprints.Delivery, correctBundleA.Fingerprints.Delivery,
		"delivery fingerprint should differ when using different commit ranges")
}

func TestReviewPrepareCommand_FindsActivityLogInDeliveryWorktree_REQ_EXECEV_C1(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

	decoyContent := []byte(`{"timestamp":"2020-01-01T00:00:00Z","command":"rm -rf /",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"deadbeef","output_hash":"decoy"}` + "\n")
	decoyLogPath := filepath.Join(repo, ".git", "armature-activity.log")
	require.NoError(t, os.WriteFile(decoyLogPath, decoyContent, 0o600))

	realContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"go build ./...",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"realsha","output_hash":"real"}` + "\n")
	realLogPath := filepath.Join(worktreeGitDir, "armature-activity.log")
	require.NoError(t, os.WriteFile(realLogPath, realContent, 0o600)) //nolint:gosec // G703: fixed test-controlled path, not user input

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

func TestReviewPrepareCommand_MismatchedBinding_NoActivityLog_REQ_EXECEV_F1(t *testing.T) {
	repo := setupRepoWithTwoTasks(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-02")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-02", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

	activityContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make build",` +
		`"exit_code":0,"exit_code_known":true,"head_sha":"abc123","output_hash":"hash"}` + "\n")
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

	out, err := runTrls(t, worktreeDir, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)

	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &bundle))

	assert.Equal(t, "task-01", bundle.Issue.ID)

	assert.Nil(t, bundle.Activity, "activity log must not be attached when binding issue ID does not match the prepared issue ID")
}

func TestReviewPrepareCommand_EnvBoundSession_AttachesActivityLog_REQ_EXECEV(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	worktreeDir := filepath.Join(repo, ".worktrees", "task-01")
	claimCmd := newRootCmd()
	claimCmd.SetOut(new(bytes.Buffer))
	claimCmd.SetArgs([]string{"claim", "--repo", repo, "task-01", "--worktree"})
	require.NoError(t, claimCmd.Execute())

	gitFile := filepath.Join(worktreeDir, ".git")
	gitFileContent, err := os.ReadFile(gitFile)
	require.NoError(t, err)
	worktreeGitDir := strings.TrimSpace(strings.TrimPrefix(string(gitFileContent), "gitdir: "))
	if !filepath.IsAbs(worktreeGitDir) {
		worktreeGitDir = filepath.Join(worktreeDir, worktreeGitDir)
	}

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

func TestReviewPrepareOutputModeEmitsEnvelope_REQ_AOC_S2_T4(t *testing.T) {
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
	run(t, repo, "git", "commit", "-m", "commit 2")
	headCmd := newCmdInDir(repo, "git", "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headOut))

	outputFile := filepath.Join(repo, "bundle.json")
	out, err := runTrls(t, repo, "review", "prepare",
		"--issue", "task-01", "--base", base, "--head", head,
		"--output", outputFile, "--format", "json")
	require.NoError(t, err)

	decoded := decodeContractEnvelope(t, out, "bundles")
	var bundles []reviewBundleWriteRow
	require.NoError(t, json.Unmarshal(decoded["bundles"], &bundles))
	require.Len(t, bundles, 1)
	assert.Equal(t, outputFile, bundles[0].Path)
	assert.Equal(t, "task-01", bundles[0].Issue)
	assert.FileExists(t, outputFile)

	var onDisk review.ReviewBundle
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &onDisk))
	assert.Equal(t, "task-01", onDisk.Issue.ID)
	assert.Equal(t, onDisk.BundleID, bundles[0].BundleID)

	artifactOut, err := runTrls(t, repo, "review", "prepare",
		"--issue", "task-01", "--base", base, "--head", head)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(artifactOut)), &onDisk))
	assert.Equal(t, "task-01", onDisk.Issue.ID)
	assert.NotContains(t, artifactOut, `"help"`)
}
