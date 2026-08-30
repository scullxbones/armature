package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
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

func countAssessmentAttestedOps(t *testing.T, repo string) int {
	t.Helper()
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	n := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		logged, err := ops.ReadLog(filepath.Join(opsDir, entry.Name()))
		require.NoError(t, err)
		for _, op := range logged {
			if op.Type == ops.OpAssessmentAttested {
				n++
			}
		}
	}
	return n
}

func TestReviewValidateMatchesRecordValidation_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, badCitationAssessment := prepareReviewValidateFixture(t)

	attestedBefore := countAssessmentAttestedOps(t, repo)
	validOut, err := runTrls(t, repo, "review", "validate", "--bundle", bundleFile, "--assessment", validAssessment)
	require.NoError(t, err, "validate must accept an assessment record would accept: %s", validOut)
	assert.Equal(t, attestedBefore, countAssessmentAttestedOps(t, repo), "validate must append no assessment-attested ops")

	recordOut, err := runTrls(t, repo, "review", "record", "--issue", "task-01", "--assessment", validAssessment, "--bundle", bundleFile, "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, recordOut, "recorded with rating", "first record after validate must append an op (validate is read-only)")
	assert.NotContains(t, recordOut, "already recorded", "idempotent duplicate means validate already attested")
	assert.Equal(t, attestedBefore+1, countAssessmentAttestedOps(t, repo), "record (not validate) must append the attestation op")

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
	report := requireAdvisoryValidateReport(t, stdout.String(), code)
	assert.Contains(t, strings.ToLower(report.Failures[0].Message), "citation")
	assert.Contains(t, strings.ToLower(report.Failures[0].Suggestion), "path-level")

	validStdout := new(bytes.Buffer)
	code = executeThenHandleRootError(t, validStdout, new(bytes.Buffer),
		"review", "validate",
		"--repo", repo,
		"--bundle", bundleFile,
		"--assessment", validAssessment,
		"--format", "json")
	assert.Equal(t, 0, code, "valid assessment must exit zero")
	validReport := decodeReviewValidateReport(t, validStdout.String())
	assert.True(t, validReport.Valid)
	assert.Empty(t, validReport.Failures)
}

func TestReviewValidateSuggestsSchemaAndCriterionID_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, _ := prepareReviewValidateFixture(t)

	t.Run("invalid schema_version", func(t *testing.T) {
		path := mutateAssessmentJSON(t, repo, validAssessment, "assessment_schema.json", func(obj map[string]any) {
			obj["schema_version"] = 99
		})
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "schema")
		assert.Contains(t, report.Failures[0].Suggestion, "schema_version")
	})

	t.Run("malformed JSON", func(t *testing.T) {
		path := filepath.Join(repo, "assessment_malformed.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "parse")
		assert.NotEmpty(t, report.Failures[0].Suggestion)
	})

	t.Run("unknown field is not a Command Failure", func(t *testing.T) {
		path := mutateAssessmentJSON(t, repo, validAssessment, "assessment_unknown_field.json", func(obj map[string]any) {
			obj["unexpected_reviewer_field"] = "extra"
		})
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		out := stdout.String()
		assert.NotContains(t, out, `"code":"REVIEW-1"`)
		if code != 0 {
			report := requireAdvisoryValidateReport(t, out, code)
			assert.NotEmpty(t, report.Failures[0].Suggestion)
			return
		}
		report := decodeReviewValidateReport(t, out)
		assert.True(t, report.Valid)
	})

	t.Run("criterion id acceptance_0", func(t *testing.T) {
		path := mutateAssessmentJSON(t, repo, validAssessment, "assessment_bad_id.json", func(obj map[string]any) {
			results, ok := obj["results"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, results)
			first, ok := results[0].(map[string]any)
			require.True(t, ok)
			first["id"] = "acceptance_0"
		})
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		joined := strings.ToLower(report.Failures[0].Message + " " + report.Failures[0].Suggestion)
		assert.Contains(t, joined, "acceptance_0")
		assert.Contains(t, report.Failures[0].Suggestion, "acceptance[0]")
	})
}

func TestReviewValidateRejectsStructurallyInvalidBundle_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, _ := prepareReviewValidateFixture(t)

	t.Run("schema_version 99 with recomputed bundle_id", func(t *testing.T) {
		invalidBundle, newID := rewriteBundleRecomputingID(t, repo, bundleFile, "bundle_schema99.json", func(b *review.ReviewBundle) {
			b.SchemaVersion = 99
		})
		assessment := mutateAssessmentJSON(t, repo, validAssessment, "assessment_schema99_bundle.json", func(obj map[string]any) {
			obj["bundle_id"] = newID
		})
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", invalidBundle, "--assessment", assessment, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "schema")
		assert.Contains(t, report.Failures[0].Suggestion, "schema_version")
	})

	t.Run("empty issue.type with recomputed bundle_id", func(t *testing.T) {
		invalidBundle, newID := rewriteBundleRecomputingID(t, repo, bundleFile, "bundle_empty_type.json", func(b *review.ReviewBundle) {
			b.Issue.Type = ""
		})
		assessment := mutateAssessmentJSON(t, repo, validAssessment, "assessment_empty_type_bundle.json", func(obj map[string]any) {
			obj["bundle_id"] = newID
		})
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", invalidBundle, "--assessment", assessment, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "type")
	})
}

func TestReviewValidateSuggestsPrepareOnBundleIntegrity_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, _ := prepareReviewValidateFixture(t)

	tampered := filepath.Join(repo, "bundle_tampered.json")
	data, err := os.ReadFile(bundleFile)
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(data, &obj))
	issue, ok := obj["issue"].(map[string]any)
	require.True(t, ok)
	issue["title"] = "tampered title"
	out, err := json.MarshalIndent(obj, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tampered, out, 0o644))

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"review", "validate", "--repo", repo, "--bundle", tampered, "--assessment", validAssessment, "--format", "json")
	report := requireAdvisoryValidateReport(t, stdout.String(), code)
	joined := strings.ToLower(report.Failures[0].Message + " " + report.Failures[0].Suggestion)
	assert.Contains(t, joined, "integrity")
	assert.Contains(t, report.Failures[0].Suggestion, "arm review prepare")
	assert.NotContains(t, strings.ToLower(report.Failures[0].Suggestion), "set bundle_id")
}

func TestReviewValidateRejectsInvalidCitationColumn_REQ_LNGHZN_S8_T1(t *testing.T) {
	repo, bundleFile, validAssessment, _ := prepareReviewValidateFixture(t)

	setColumn := func(t *testing.T, name string, column any) string {
		t.Helper()
		return mutateAssessmentJSON(t, repo, validAssessment, name, func(obj map[string]any) {
			results, ok := obj["results"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, results)
			first, ok := results[0].(map[string]any)
			require.True(t, ok)
			citations, ok := first["citations"].([]any)
			require.True(t, ok)
			require.NotEmpty(t, citations)
			cite, ok := citations[0].(map[string]any)
			require.True(t, ok)
			cite["column"] = column
		})
	}

	t.Run("column 0", func(t *testing.T) {
		path := setColumn(t, "assessment_column_0.json", 0)
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "column")
		assert.NotEmpty(t, report.Failures[0].Suggestion)
	})

	t.Run("negative column", func(t *testing.T) {
		path := setColumn(t, "assessment_column_neg.json", -3)
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", path, "--format", "json")
		report := requireAdvisoryValidateReport(t, stdout.String(), code)
		assert.Contains(t, strings.ToLower(report.Failures[0].Message), "column")
	})

	t.Run("column omitted still valid", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"review", "validate", "--repo", repo, "--bundle", bundleFile, "--assessment", validAssessment, "--format", "json")
		assert.Equal(t, 0, code)
		report := decodeReviewValidateReport(t, stdout.String())
		assert.True(t, report.Valid)
	})
}

func rewriteBundleRecomputingID(t *testing.T, repo, src, name string, mut func(*review.ReviewBundle)) (string, string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	bundle, err := review.DecodeReviewBundle(data)
	require.NoError(t, err)
	mut(&bundle)
	bundle.BundleID = review.ComputeBundleID(bundle)
	out, err := json.MarshalIndent(&bundle, "", "  ")
	require.NoError(t, err)
	dst := filepath.Join(repo, name)
	require.NoError(t, os.WriteFile(dst, out, 0o644)) //nolint:gosec // G703: dst is filepath.Join of the test repo and a fixed filename
	return dst, bundle.BundleID
}

func mutateAssessmentJSON(t *testing.T, repo, src, name string, mut func(map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(data, &obj))
	mut(obj)
	out, err := json.MarshalIndent(obj, "", "  ")
	require.NoError(t, err)
	path := filepath.Join(repo, name)
	require.NoError(t, os.WriteFile(path, out, 0o644))
	return path
}

func decodeReviewValidateReport(t *testing.T, out string) reviewValidateReport {
	t.Helper()
	var report reviewValidateReport
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &report), "stdout must be a reviewValidateReport, got %s", out)
	return report
}

func requireAdvisoryValidateReport(t *testing.T, out string, code int) reviewValidateReport {
	t.Helper()
	assert.Equal(t, 1, code, "invalid assessment must exit 1")
	assert.NotContains(t, out, `"code":"REVIEW-1"`, "advisory validation must not wrap as a Command Failure")
	report := decodeReviewValidateReport(t, out)
	require.False(t, report.Valid)
	require.NotEmpty(t, report.Failures)
	for _, failure := range report.Failures {
		assert.NotEmpty(t, failure.Suggestion, "each failure must include an auto-fix suggestion: %s", failure.Message)
	}
	return report
}
