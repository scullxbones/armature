package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepareReviewValidateFixture(t *testing.T) (repo, bundleFile, validAssessment, badCitationAssessment string) {
	t.Helper()
	repo = setupRepoWithTask(t)

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

	bundleFile = filepath.Join(repo, "bundle.json")
	_, err = runTrls(t, repo, "review", "prepare", "--issue", "task-01", "--base", base, "--head", head, "--output", bundleFile)
	require.NoError(t, err)

	bundleData, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	valid := review.ConformanceAssessment{
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
	validAssessment = filepath.Join(repo, "assessment_valid.json")
	validJSON, err := json.MarshalIndent(&valid, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(validAssessment, validJSON, 0o644))

	bad := valid
	bad.Results = []review.CriterionResult{
		{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "Implementation verified.",
			Citations: []review.Citation{
				{Path: "impl.go", Line: 9999},
			},
		},
	}
	badCitationAssessment = filepath.Join(repo, "assessment_bad_cite.json")
	badJSON, err := json.MarshalIndent(&bad, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(badCitationAssessment, badJSON, 0o644))

	return repo, bundleFile, validAssessment, badCitationAssessment
}

func TestReviewValidateMatchesRecordValidation_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, badCitationAssessment := prepareReviewValidateFixture(t)

	validOut, err := runTrls(t, repo, "review", "validate", "--bundle", bundleFile, "--assessment", validAssessment)
	require.NoError(t, err, "validate must accept an assessment record would accept: %s", validOut)

	recordOut, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", validAssessment, "--bundle", bundleFile)
	require.NoError(t, err)
	assert.Contains(t, recordOut, "recorded", "first record after validate must append an op (validate is read-only)")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", badCitationAssessment})
	validateErr := cmd.Execute()
	require.Error(t, validateErr, "validate must reject an out-of-bounds citation")

	cmd = newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"review", "record", "--repo", repo, "--issue", "task-01", "--assessment", badCitationAssessment, "--bundle", bundleFile})
	recordErr := cmd.Execute()
	require.Error(t, recordErr, "record must reject the same out-of-bounds citation")
	assert.Contains(t, validateErr.Error(), "citation")
	assert.Contains(t, recordErr.Error(), "citation")
}

func TestReviewValidateSuggestsCitationDowngrade_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, badCitationAssessment := prepareReviewValidateFixture(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"review", "validate",
		"--repo", repo,
		"--bundle", bundleFile,
		"--assessment", badCitationAssessment,
		"--format", "json")
	assert.Equal(t, 1, code, "invalid assessment must exit non-zero")
	out := stdout.String()
	assert.NotContains(t, out, `"code":"REVIEW-1"`, "advisory validation must not wrap as a Command Failure")
	assert.Contains(t, strings.ToLower(out), "citation")
	assert.Contains(t, strings.ToLower(out), "path-level")

	validStdout := new(bytes.Buffer)
	code = executeThenHandleRootError(t, validStdout, new(bytes.Buffer),
		"review", "validate",
		"--repo", repo,
		"--bundle", bundleFile,
		"--assessment", validAssessment,
		"--format", "json")
	assert.Equal(t, 0, code, "valid assessment must exit zero")
}
