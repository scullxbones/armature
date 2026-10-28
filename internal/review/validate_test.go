package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/attestation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResult_Valid(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "all requirements met",
				Citations: []Citation{
					FileCitation("file1.go", 10, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/file1.go
+++ b/file1.go
@@ -8,5 +8,6 @@
 line 8
 line 9
+line 10 (new)
 line 11
 line 12
 line 13
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors")
}

func TestValidateResult_InvalidBundleID(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "all requirements met",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx := buildDiffIndex("")

	errs := validateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for empty bundle ID")
	assert.True(t, containsError(errs, "bundle ID"), "Expected 'bundle ID' in error message")
}

func TestValidateResult_InvalidCitation(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "all requirements met",
				Citations: []Citation{
					FileCitation("file1.go", 999, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
+new line
 line 1
 line 2
 line 3
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for invalid citation")
	assert.True(t, containsError(errs, "file1.go") || containsError(errs, "999"), "Expected file/line reference in error")
}

func TestValidateResult_SuggestsCitationDowngrade_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "all requirements met",
				Citations: []Citation{
					FileCitation("file1.go", 104, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
+new line
 line 1
 line 2
 line 3
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "file1.go:104"), "expected out-of-bounds line in the error")
	assert.True(t, containsError(errs, "path-level"), "expected auto-fix suggestion to downgrade to path-level")
}

func TestValidateResult_InvalidCriterionResult(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:     "definition_of_done",
				Status: NotSatisfied,
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx := buildDiffIndex("")

	errs := validateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for invalid criterion result")
}

func TestValidateResult_MultipleCitations(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "evidence from multiple files",
				Citations: []Citation{
					FileCitation("file1.go", 1, 0),
					FileCitation("file2.go", 5, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/file1.go
+++ b/file1.go
@@ -1,2 +1,3 @@
+new line
 line 1
 line 2
--- a/file2.go
+++ b/file2.go
@@ -3,3 +3,4 @@
 line 3
 line 4
+line 5 (new)
 line 6
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors for multiple citations")
}

func TestNewAttestation(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "def_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: PartiallySatisfied, Rationale: "partial"},
			{ID: "acceptance[1]", Status: NotSatisfied, Rationale: "not met", MissingEvidence: "not implemented"},
			{ID: "acceptance[2]", Status: Indeterminate, Rationale: "unclear"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}
	delivery := Delivery{
		BaseSHA: "aaa000",
		HeadSHA: "bbb111",
	}

	att := newAttestation(assessment, delivery, nil)

	assert.NotNil(t, att)
	assert.Equal(t, "sha256:test123", att.BundleID)
	assert.Equal(t, "sha256:contract123", att.ContractFingerprint)
	assert.Equal(t, "sha256:delivery123", att.DeliveryFingerprint)

	assert.Equal(t, attestation.Red, att.Rating)
	assert.Equal(t, 1, att.SatisfiedCount)
	assert.Equal(t, 1, att.PartiallySatisfiedCount)
	assert.Equal(t, 1, att.NotSatisfiedCount)
	assert.Equal(t, 1, att.IndeterminateCount)
}

func TestNewAttestation_AllGreen(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test456",
		Results: []CriterionResult{
			{ID: "def_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract456",
		DeliveryFingerprint: "sha256:delivery456",
	}
	delivery := Delivery{}

	att := newAttestation(assessment, delivery, nil)

	assert.Equal(t, attestation.Green, att.Rating)
	assert.Equal(t, 2, att.SatisfiedCount)
	assert.Equal(t, 0, att.PartiallySatisfiedCount)
	assert.Equal(t, 0, att.NotSatisfiedCount)
	assert.Equal(t, 0, att.IndeterminateCount)
}

func TestNewAttestation_PopulatesSHAs(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test789",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "complete"},
		},
		ContractFingerprint: "sha256:contract789",
		DeliveryFingerprint: "sha256:delivery789",
	}
	delivery := Delivery{
		BaseSHA:      "abc123abc123abc123abc123abc123abc123abc1",
		HeadSHA:      "def456def456def456def456def456def456def4",
		ChangedFiles: []string{"main.go"},
	}

	att := newAttestation(assessment, delivery, nil)

	assert.Equal(t, delivery.BaseSHA, att.BaseSHA, "BaseSHA must be populated from delivery")
	assert.Equal(t, delivery.HeadSHA, att.HeadSHA, "HeadSHA must be populated from delivery")
}

func TestIsDuplicate_SameResultFingerprint(t *testing.T) {
	t.Parallel()

	att1 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}

	assert.True(t, isDuplicate(att1, att2))
}

func TestIsDuplicate_DifferentResultFingerprint(t *testing.T) {
	t.Parallel()

	att1 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint456",
	}

	assert.False(t, isDuplicate(att1, att2))
}

func TestIsDuplicate_SameFingerprintDifferentSkillVersion(t *testing.T) {
	t.Parallel()

	att1 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		SkillVersion:      "v1.0.0",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &attestation.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		SkillVersion:      "v2.0.0",
		ResultFingerprint: "sha256:fingerprint123",
	}

	assert.True(t, isDuplicate(att1, att2))
}

func TestValidateResultNoDiff_Valid(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "implementation complete",
				Citations: []Citation{
					FileCitation("internal/review/types.go", 42, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultNoDiff(assessment)
	assert.Empty(t, errs, "citation with file/line must not be rejected at record time")
}

func TestValidateResultNoDiff_EmptyBundleID(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		BundleID: "",
		Results: []CriterionResult{
			{ID: "def", Status: Satisfied, Rationale: "ok"},
		},
	}

	errs := validateResultNoDiff(assessment)
	assert.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "bundle ID"))
}

func TestValidateResultNoDiff_InvalidResult(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		BundleID: "sha256:bundle123",
		Results: []CriterionResult{
			{ID: "", Status: Satisfied, Rationale: ""},
		},
	}

	errs := validateResultNoDiff(assessment)
	assert.NotEmpty(t, errs)
}

func TestValidateAssessment_AttachesSuggestions_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	t.Run("unsupported schema version", func(t *testing.T) {
		t.Parallel()
		err := ValidateAssessment(RecordInput{
			IssueID: "task-01",
			Assessment: &ConformanceAssessment{
				SchemaVersion:       99,
				BundleID:            "bundle-123",
				ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Results: []CriterionResult{
					{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("impl.go", 1, 0)}},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema version")
		assert.Contains(t, err.Error(), "(suggestion:")
		assert.Contains(t, err.Error(), "schema_version")
	})

	t.Run("fingerprint mismatch", func(t *testing.T) {
		t.Parallel()
		bundle := &ReviewBundle{
			SchemaVersion: SchemaVersion,
			Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Test"},
			Contract:      Contract{DefinitionOfDone: "Done"},
			Delivery:      Delivery{BaseSHA: "base", HeadSHA: "head", Diff: "--- a/impl.go\n+++ b/impl.go\n@@ -1,0 +1,1 @@\n+package main"},
			Fingerprints: Fingerprints{
				Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		}
		bundle.BundleID = mustComputeBundleID(t, *bundle)
		err := ValidateAssessment(RecordInput{
			IssueID: "task-01",
			Bundle:  bundle,
			Assessment: &ConformanceAssessment{
				SchemaVersion:       SchemaVersion,
				BundleID:            bundle.BundleID,
				ContractFingerprint: bundle.Fingerprints.Contract,
				DeliveryFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
				Results: []CriterionResult{
					{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("impl.go", 1, 0)}},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delivery_fingerprint")
		assert.Contains(t, err.Error(), "(suggestion:")
	})

	t.Run("coverage missing criterion", func(t *testing.T) {
		t.Parallel()
		acceptance := []string{"Feature works correctly"}
		acceptanceJSON, err := json.Marshal(acceptance)
		require.NoError(t, err)
		contract := Contract{DefinitionOfDone: "Implementation complete", Acceptance: acceptance}
		err = ValidateAssessment(RecordInput{
			IssueID: "task-01",
			Issue: &IssueData{
				DefinitionOfDone: contract.DefinitionOfDone,
				Acceptance:       string(acceptanceJSON),
			},
			Assessment: &ConformanceAssessment{
				SchemaVersion:       SchemaVersion,
				BundleID:            "bundle-123",
				ContractFingerprint: fingerprintContract(contract),
				DeliveryFingerprint: "sha256:bbbb",
				Results: []CriterionResult{
					{ID: "definition_of_done", Status: Satisfied, Rationale: "Done", Citations: []Citation{FileCitation("impl.go", 1, 0)}},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "acceptance[0]")
		assert.Contains(t, err.Error(), "(suggestion:")
	})

	t.Run("activity citations without activity section", func(t *testing.T) {
		t.Parallel()
		err := ValidateAssessment(RecordInput{
			IssueID: "task-01",
			Assessment: &ConformanceAssessment{
				SchemaVersion:       SchemaVersion,
				BundleID:            "bundle-no-activity",
				ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Results: []CriterionResult{
					{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{ActivityCitation("0")}},
				},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity")
		assert.Contains(t, err.Error(), "(suggestion:")
	})

	t.Run("activity log digest mismatch", func(t *testing.T) {
		t.Parallel()
		logPath := filepath.Join(t.TempDir(), "armature-activity.log")
		original := []byte(activityLogLineJSON(t, map[string]any{"command": "make build"}) + "\n")
		require.NoError(t, os.WriteFile(logPath, original, 0o600))
		input := validValidateInputWithActivity(t, &Activity{
			Digest:     fingerprintActivity(original),
			EntryCount: 1,
			LogPath:    logPath,
		})
		require.NoError(t, os.WriteFile(logPath, []byte(activityLogLineJSON(t, map[string]any{"command": "curl evil.example"})+"\n"), 0o600))

		err := ValidateAssessment(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity log digest mismatch")
		assert.Contains(t, err.Error(), "(suggestion: re-run arm review prepare so activity.digest matches the on-disk log)")
		assert.NotContains(t, err.Error(), "fix the assessment to satisfy this check")
	})

	t.Run("activity log missing or unreadable", func(t *testing.T) {
		t.Parallel()
		err := ValidateAssessment(validValidateInputWithActivity(t, &Activity{
			Digest:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			EntryCount: 1,
			LogPath:    filepath.Join(t.TempDir(), "does-not-exist.log"),
		}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity log missing or unreadable")
		assert.Contains(t, err.Error(), "(suggestion: restore the activity log or re-run arm review prepare)")
		assert.NotContains(t, err.Error(), "fix the assessment to satisfy this check")
	})
}

func validValidateInputWithActivity(t *testing.T, activity *Activity) RecordInput {
	t.Helper()
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Test"},
		Contract:      Contract{DefinitionOfDone: "Done"},
		Delivery:      Delivery{BaseSHA: "base", HeadSHA: "head", Diff: "--- a/impl.go\n+++ b/impl.go\n@@ -1,0 +1,1 @@\n+package main"},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Activity: activity,
	}
	bundle.BundleID = mustComputeBundleID(t, *bundle)
	return RecordInput{
		IssueID: "task-01",
		Bundle:  bundle,
		Assessment: &ConformanceAssessment{
			SchemaVersion:       SchemaVersion,
			BundleID:            bundle.BundleID,
			ContractFingerprint: bundle.Fingerprints.Contract,
			DeliveryFingerprint: bundle.Fingerprints.Delivery,
			Results: []CriterionResult{
				{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("impl.go", 1, 0)}},
			},
		},
	}
}

func TestValidateResultNoDiff_InvalidCriterionIDFormat_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		BundleID: "sha256:bundle123",
		Results: []CriterionResult{
			{ID: "acceptance_0", Status: Satisfied, Rationale: "ok"},
		},
	}

	errs := validateResultNoDiff(assessment)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "acceptance_0"), "expected invalid id in the error")
	assert.True(t, containsError(errs, "acceptance[0]"), "expected suggestion of the canonical criterion-ID format")
}

func containsError(errs []string, substr string) bool {
	for _, err := range errs {
		if strings.Contains(err, substr) {
			return true
		}
	}
	return false
}

func TestValidateResult_NoCitations_NotSatisfied(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:              "definition_of_done",
				Status:          NotSatisfied,
				Rationale:       "requirement not met",
				MissingEvidence: "feature X not implemented",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx := buildDiffIndex("")

	errs := validateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors for NotSatisfied with MissingEvidence")
}

func TestValidateResultCoverage_Valid(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Task must be complete",
		Acceptance:       []string{"User can login", "User can logout"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[1]", Status: Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.Empty(t, errs, "expected no validation errors for complete assessment")
}

func TestValidateResultCoverage_MissingAcceptance(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Task must be complete",
		Acceptance:       []string{"User can login", "User can logout"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.NotEmpty(t, errs, "expected validation errors for missing criterion")
	assert.True(t, containsError(errs, "acceptance[1]"), "expected error about missing acceptance[1]")
}

func TestValidateResultCoverage_MissingDefinitionOfDone(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Task must be complete",
		Acceptance:       []string{"User can login"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.NotEmpty(t, errs, "expected validation errors for missing definition_of_done")
	assert.True(t, containsError(errs, "definition_of_done"), "expected error about missing definition_of_done")
}

func TestValidateResultCoverage_DuplicateID(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Task must be complete",
		Acceptance:       []string{"User can login"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "duplicate"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.NotEmpty(t, errs, "expected validation errors for duplicate ID")
	assert.True(t, containsError(errs, "acceptance[0]") && containsError(errs, "duplicate"), "expected error about duplicate")
}

func TestValidateResultCoverage_UnexpectedCriterionID(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Task must be complete",
		Acceptance:       []string{"User can login"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
			{ID: "acceptance[99]", Status: Satisfied, Rationale: "extra"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.NotEmpty(t, errs, "expected validation errors for unexpected criterion ID")
	assert.True(t, containsError(errs, "acceptance[99]"), "expected error mentioning the unexpected ID")
	assert.True(t, containsError(errs, "unexpected"), "expected error to say 'unexpected'")
}

func TestValidateResultCoverage_EmptyDefinitionOfDone(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "",
		Acceptance:       []string{"User can login"},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateResultCoverage(assessment, contract)
	assert.Empty(t, errs, "expected no validation errors when definition_of_done is empty")
}

func TestValidateResult_PathOnlyCitation_FileInDiff(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "file was modified",
				Citations: []Citation{
					FileCitation("internal/review/test.go", 0, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/internal/review/test.go
+++ b/internal/review/test.go
@@ -5,7 +5,7 @@ package review
 func TestFunc() {
 	x := 1
-	y := 2
+	y := 3
 	return x + y
 }
 extra line
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors for path-only citation with file in diff")
}

func TestValidateResult_PathOnlyCitation_FileNotInDiff(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "file was modified",
				Citations: []Citation{
					FileCitation("nonexistent.go", 0, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	diff := `--- a/internal/review/test.go
+++ b/internal/review/test.go
@@ -1,2 +1,3 @@
+new line
 line 1
 line 2
`

	idx := buildDiffIndex(diff)

	errs := validateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for path-only citation with file not in diff")
	assert.True(t, containsError(errs, "nonexistent.go"), "Expected file reference in error message")
}

func knownExitEntries(ids ...int) map[int]activityEntryDetails {
	entries := make(map[int]activityEntryDetails)
	for _, id := range ids {
		entries[id] = activityEntryDetails{EntryID: id, Command: "make build", ExitCode: 0, ExitCodeKnown: true}
	}
	return entries
}

func TestValidateActivityCitations_ValidEntryID_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.Empty(t, errs, "Valid activity citation by entry ID should not produce errors")
}

func TestValidateActivityCitations_InvalidEntryID_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("10"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.NotEmpty(t, errs, "Unknown entry ID should produce validation errors")
	assert.True(t, containsError(errs, "unknown activity entry ID"), "Error should mention unknown entry ID")
}

func TestValidateActivityCitations_NonNumericEntryID_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("not-a-number"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.NotEmpty(t, errs, "Non-numeric entry ID should produce validation errors")
	assert.True(t, containsError(errs, "invalid activity entry ID"), "Error should mention invalid entry ID")
}

func TestValidateActivityCitations_ActivityOnlySatisfiesImplementation_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "implementation complete",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.NotEmpty(t, errs, "Activity-only satisfied on implementation criterion should produce errors")
	assert.True(t, containsError(errs, "upgrade-only rule"), "Error should mention upgrade-only rule")
}

func TestValidateActivityCitations_ActivityOnlyPartiallyImplementation_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    PartiallySatisfied,
				Rationale: "partially implemented",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.NotEmpty(t, errs, "Activity-only partially_satisfied on implementation criterion should be rejected")
	assert.True(t, containsError(errs, "upgrade-only rule"), "Error should mention upgrade-only rule")
}

func TestValidateActivityCitations_ActivityOnlyAcceptance_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test executed successfully",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.Empty(t, errs, "Activity-only satisfied on acceptance criterion should be allowed")
}

func TestValidateActivityCitations_MixedCitations_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "implementation complete",
				Citations: []Citation{
					FileCitation("main.go", 10, 0),
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        5,
		DeliveryHeadCount: 3,
		EarlierCount:      2,
		LogPath:           "armature-activity.log",
	}

	entries := knownExitEntries(0, 1, 2, 3, 4)

	errs := validateActivityCitations(assessment, activity, entries, "")
	assert.Empty(t, errs, "Mixed citations should not trigger upgrade-only rule")
}

func TestValidateActivityCitations_NilActivity_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					FileCitation("main.go", 10, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := validateActivityCitations(assessment, nil, nil, "")
	assert.Empty(t, errs, "Validation against nil activity should succeed")
}

func TestValidateActivityCitations_HeadSHAMismatch_REQ_EXECEV_F2(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        2,
		DeliveryHeadCount: 1,
		EarlierCount:      1,
		LogPath:           "armature-activity.log",
	}

	entries := map[int]activityEntryDetails{
		0: {EntryID: 0, Command: "make build", ExitCode: 0, ExitCodeKnown: true, HeadSHA: "earlier_sha"},
		1: {EntryID: 1, Command: "make test", ExitCode: 0, ExitCodeKnown: true, HeadSHA: "delivery_sha"},
	}

	deliveryHeadSHA := "delivery_sha"

	errs := validateActivityCitations(assessment, activity, entries, deliveryHeadSHA)
	assert.NotEmpty(t, errs, "Activity entry with mismatched HeadSHA should be rejected")
	assert.True(t, containsError(errs, "head_sha") || containsError(errs, "HeadSHA") || containsError(errs, "earlier commit"),
		"Error should mention the HeadSHA mismatch")
}

func TestValidateActivityCitations_HeadSHAMatch_REQ_EXECEV_F2(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("1"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:            "sha256:abc123",
		EntryCount:        2,
		DeliveryHeadCount: 1,
		EarlierCount:      1,
		LogPath:           "armature-activity.log",
	}

	entries := map[int]activityEntryDetails{
		0: {EntryID: 0, Command: "make build", ExitCode: 0, ExitCodeKnown: true, HeadSHA: "earlier_sha"},
		1: {EntryID: 1, Command: "make test", ExitCode: 0, ExitCodeKnown: true, HeadSHA: "delivery_sha"},
	}

	deliveryHeadSHA := "delivery_sha"

	errs := validateActivityCitations(assessment, activity, entries, deliveryHeadSHA)
	assert.Empty(t, errs, "Activity entry with matching HeadSHA should be accepted")
}

func TestValidateActivityCitations_FailedExitCodeCannotSatisfy_REQ_EXECEV(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{Digest: "sha256:abc123", EntryCount: 1, LogPath: "armature-activity.log"}
	entries := map[int]activityEntryDetails{
		0: {EntryID: 0, Command: "make test", ExitCode: 1, ExitCodeKnown: true, HeadSHA: "head"},
	}

	errs := validateActivityCitations(assessment, activity, entries, "head")
	assert.NotEmpty(t, errs, "a failed (nonzero, known) exit code must not support a Satisfied citation")
	assert.True(t, containsError(errs, "exit code") || containsError(errs, "exit_code"),
		"error should mention the failed exit code")
}

func TestValidateActivityCitations_FailedExitCodeCanSupportNotSatisfied_REQ_EXECEV(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    NotSatisfied,
				Rationale: "test still fails",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{Digest: "sha256:abc123", EntryCount: 1, LogPath: "armature-activity.log"}
	entries := map[int]activityEntryDetails{
		0: {EntryID: 0, Command: "make test", ExitCode: 1, ExitCodeKnown: true, HeadSHA: "head"},
	}

	errs := validateActivityCitations(assessment, activity, entries, "head")
	assert.Empty(t, errs, "a failed exit code citing NotSatisfied should be accepted")
}

func TestValidateActivityCitations_HeadSHAMismatchAllowedForNotSatisfied_REQ_EXECEV(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    NotSatisfied,
				Rationale: "was already broken before this commit too",
				Citations: []Citation{
					ActivityCitation("0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{Digest: "sha256:abc123", EntryCount: 1, LogPath: "armature-activity.log"}
	entries := map[int]activityEntryDetails{
		0: {EntryID: 0, Command: "make test", ExitCode: 1, ExitCodeKnown: true, HeadSHA: "earlier_sha"},
	}

	errs := validateActivityCitations(assessment, activity, entries, "delivery_sha")
	assert.Empty(t, errs, "an earlier-commit citation supporting NotSatisfied must not be blocked by the HeadSHA gate")
}

func TestValidateActivityCitations_IndexReferenceRejected_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []CriterionResult{
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "test passed",
				Citations: []Citation{
					ActivityCitation("index:0"),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	activity := &Activity{
		Digest:     "sha256:abc123",
		EntryCount: 5,
		LogPath:    "armature-activity.log",
	}

	errs := validateActivityCitations(assessment, activity, knownExitEntries(0, 1, 2, 3, 4), "")
	assert.NotEmpty(t, errs, "index-terminology reference should be rejected")
	assert.True(t, containsError(errs, "invalid activity entry ID"),
		"error should reject the non-numeric index reference")
}

func TestActivityDigestMismatchRejected_REQ_EXECEV_T3(t *testing.T) {
	t.Parallel()

	t.Run("digest still matches when log is unmodified", func(t *testing.T) {
		t.Parallel()
		logPath := filepath.Join(t.TempDir(), "armature-activity.log")
		content := []byte(
			`{"timestamp":"2026-01-15T10:30:45Z","command":"make build","exit_code":0,` +
				`"exit_code_known":true,"head_sha":"abc123","output_hash":"h"}` + "\n")
		require.NoError(t, os.WriteFile(logPath, content, 0o600))

		activity := &Activity{
			Digest:     fingerprintActivity(content),
			EntryCount: 1,
			LogPath:    logPath,
		}

		_, errs := validateActivityDigestAndLoadEntries(activity)
		assert.Empty(t, errs, "unmodified log should match recorded digest")
	})

	t.Run("digest mismatch rejected when log content changes", func(t *testing.T) {
		t.Parallel()
		logPath := filepath.Join(t.TempDir(), "armature-activity.log")
		originalContent := []byte(`2026-01-15T10:30:45Z activity: command="make build" exit_code=0 head_sha=abc123` + "\n")
		require.NoError(t, os.WriteFile(logPath, originalContent, 0o600))
		recordedDigest := fingerprintActivity(originalContent)

		require.NoError(t, os.WriteFile(logPath, []byte(`2026-01-15T10:30:45Z activity: command="rm -rf /" exit_code=0 head_sha=abc123`+"\n"), 0o600))

		activity := &Activity{
			Digest:     recordedDigest,
			EntryCount: 1,
			LogPath:    logPath,
		}

		_, errs := validateActivityDigestAndLoadEntries(activity)
		assert.NotEmpty(t, errs, "tampered log content should be rejected")
		assert.True(t, containsError(errs, "digest mismatch"), "error should mention digest mismatch")
	})

	t.Run("missing log rejected", func(t *testing.T) {
		t.Parallel()
		missingActivity := &Activity{
			Digest:     "sha256:doesnotmatter",
			EntryCount: 1,
			LogPath:    filepath.Join(t.TempDir(), "does-not-exist.log"),
		}
		_, errs := validateActivityDigestAndLoadEntries(missingActivity)
		assert.NotEmpty(t, errs, "missing log should be rejected")
		assert.True(t, containsError(errs, "missing or unreadable"), "error should mention the log is missing")
	})

	t.Run("citation rejected end-to-end at record time via Record", func(t *testing.T) {
		t.Parallel()
		e2eLogPath := filepath.Join(t.TempDir(), "armature-activity.log")
		e2eContent := []byte(activityLogLineJSON(t, map[string]any{"command": "make build", "head_sha": "head456"}) + "\n")
		require.NoError(t, os.WriteFile(e2eLogPath, e2eContent, 0o600))
		e2eDigest := fingerprintActivity(e2eContent)

		bundle := &ReviewBundle{
			SchemaVersion: SchemaVersion,
			Issue: IssueInfo{
				ID:    "task-01",
				Type:  "task",
				Title: "Test Task",
			},
			Contract: Contract{
				DefinitionOfDone: "Implementation complete",
				Acceptance:       []string{"Feature works correctly"},
			},
			Delivery: Delivery{
				BaseSHA: "base123",
				HeadSHA: "head456",
				Diff:    "--- a/impl.go\n+++ b/impl.go\n@@ -1,0 +1,1 @@\n+package main",
			},
			Fingerprints: Fingerprints{
				Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
			Activity: &Activity{
				Digest:     e2eDigest,
				EntryCount: 1,
				LogPath:    e2eLogPath,
			},
		}
		bundle.BundleID = mustComputeBundleID(t, *bundle)

		assessment := &ConformanceAssessment{
			SchemaVersion:       SchemaVersion,
			BundleID:            bundle.BundleID,
			ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Results: []CriterionResult{
				{
					ID:        "definition_of_done",
					Status:    PartiallySatisfied,
					Rationale: "Some evidence of completion.",
					Citations: []Citation{

						FileCitation("impl.go", 1, 0),
						ActivityCitation("0"),
					},
				},
				{
					ID:        "acceptance[0]",
					Status:    Satisfied,
					Rationale: "Feature works as designed.",
					Citations: []Citation{
						ActivityCitation("0"),
					},
				},
			},
		}

		_, err := record(RecordInput{
			Assessment: assessment,
			Bundle:     bundle,
			IssueID:    "task-01",
		})
		require.NoError(t, err, "record should succeed when the activity log matches the bundle digest")

		tamperedContent := []byte(activityLogLineJSON(t, map[string]any{"command": "curl evil.example", "head_sha": "head456"}) + "\n")
		require.NoError(t, os.WriteFile(e2eLogPath, tamperedContent, 0o600))

		_, err = record(RecordInput{
			Assessment: assessment,
			Bundle:     bundle,
			IssueID:    "task-01",
		})
		require.Error(t, err, "record should reject activity citations when the log digest no longer matches the bundle")
		assert.Contains(t, err.Error(), "digest mismatch")
	})
}

func TestSuggestValidateFix_BundleIntegrityBeforeBundleIDMismatch_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	integrity := "bundle integrity check failed: recomputed bundle_id sha256:aaa " +
		"does not match bundle's recorded bundle_id sha256:bbb " +
		"(bundle contents may have been altered since `arm review prepare` ran)"
	got := ClassifyValidateFix(integrity).Suggestion
	assert.Contains(t, got, "arm review prepare")
	assert.NotContains(t, strings.ToLower(got), "set bundle_id")

	mismatch := "assessment bundle_id sha256:aaa does not match bundle bundle_id sha256:bbb"
	got = ClassifyValidateFix(mismatch).Suggestion
	assert.Contains(t, got, "set bundle_id")
	assert.NotContains(t, got, "arm review prepare")

	column := "parse assessment JSON: decode conformance assessment: citation: column must be >= 1, got 0"
	got = ClassifyValidateFix(column).Suggestion
	assert.Contains(t, got, "1-based")

	lineNull := "parse assessment JSON: decode conformance assessment: citation: line must be an integer, not null"
	got = ClassifyValidateFix(lineNull).Suggestion
	assert.Contains(t, strings.ToLower(got), "null")
	assert.NotContains(t, got, "schema_version")
}

func TestSuggestValidateFix_SpecificDecodeBeforeGenericParse_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	status := "parse assessment JSON: decode conformance assessment: invalid criterion status: passed"
	got := ClassifyValidateFix(status).Suggestion
	assert.Contains(t, got, "satisfied")
	assert.NotContains(t, got, "schema_version")

	missing := `parse assessment JSON: decode conformance assessment: criterion result: missing required field "status"`
	got = ClassifyValidateFix(missing).Suggestion
	assert.Contains(t, strings.ToLower(got), "required field")
	assert.NotContains(t, got, "schema_version")
}

func TestSuggestValidateFix_BundleValidFailuresSuggestPrepare_REQ_LNGHZN_S8_T1(t *testing.T) {
	t.Parallel()

	emptyType := "review bundle: missing issue type"
	got := ClassifyValidateFix(emptyType).Suggestion
	assert.Contains(t, got, "arm review prepare")
	assert.NotContains(t, strings.ToLower(got), "fix the assessment")

	emptyTitle := "review bundle: missing issue title"
	got = ClassifyValidateFix(emptyTitle).Suggestion
	assert.Contains(t, got, "arm review prepare")
	assert.NotContains(t, strings.ToLower(got), "fix the assessment")

	schema := "review bundle: unsupported schema version 99"
	got = ClassifyValidateFix(schema).Suggestion
	assert.Contains(t, got, "arm review prepare")
	assert.NotContains(t, got, "set schema_version")
}

func TestSuggestValidateFix_IssueContractMismatchSuggestsPrepare_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()

	issue := "assessment contract fingerprint aaa does not match issue contract fingerprint bbb"
	got := ClassifyValidateFix(issue).Suggestion
	assert.Contains(t, got, "arm review prepare")
	assert.NotContains(t, strings.ToLower(got), "copy fingerprints")

	bundle := "assessment contract_fingerprint aaa does not match bundle contract_fingerprint bbb"
	got = ClassifyValidateFix(bundle).Suggestion
	assert.Contains(t, strings.ToLower(got), "copy fingerprints.contract")
	assert.NotContains(t, got, "arm review prepare")
}

func TestClassifyValidateFix_AssessmentVsSetup_REQ_LNGHZN_S8_T2(t *testing.T) {
	t.Parallel()

	setup := ClassifyValidateFix("review bundle: missing issue type")
	assert.False(t, setup.Fixable, "bundle structural errors are not assessment-rewritable")
	assert.Contains(t, setup.Suggestion, "arm review prepare")

	parseBundle := ClassifyValidateFix("parse bundle JSON: decode review bundle: unexpected EOF")
	assert.False(t, parseBundle.Fixable)

	staleContract := ClassifyValidateFix("assessment contract fingerprint aaa does not match issue contract fingerprint bbb")
	assert.False(t, staleContract.Fixable)
	assert.Contains(t, staleContract.Suggestion, "arm review prepare")

	rationale := ClassifyValidateFix("criterion result 0: missing rationale")
	assert.True(t, rationale.Fixable, "missing rationale is fixed by rewriting the assessment")
	assert.Contains(t, strings.ToLower(rationale.Suggestion), "rationale")

	copyFP := ClassifyValidateFix("assessment contract_fingerprint aaa does not match bundle contract_fingerprint bbb")
	assert.True(t, copyFP.Fixable)
	assert.Contains(t, strings.ToLower(copyFP.Suggestion), "copy fingerprints.contract")

	dropActivity := ClassifyValidateFix("criterion result acceptance[1]: cites activity log entries but bundle has no bundle activity section")
	assert.True(t, dropActivity.Fixable, "dropping activity_entry_id citations is an assessment rewrite")

	satisfiedNoCitations := ClassifyValidateFix("criterion result acceptance[1]: citations required for status satisfied")
	assert.True(t, satisfiedNoCitations.Fixable, "evidence-free satisfied is an assessment rewrite, not a setup error")
	assert.Contains(t, strings.ToLower(satisfiedNoCitations.Suggestion), "citation")
	assert.NotContains(t, strings.ToLower(satisfiedNoCitations.Suggestion), "or [] with missing_evidence")
}
