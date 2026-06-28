package review_test

import (
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResult_Valid(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "all requirements met",
				Citations: []review.Citation{
					{Path: "file1.go", Line: 10},
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

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors")
}

func TestValidateResult_InvalidBundleID(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "", // Empty bundle ID
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "all requirements met",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx, err := review.BuildDiffIndex("")
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for empty bundle ID")
	assert.True(t, containsError(errs, "bundle ID"), "Expected 'bundle ID' in error message")
}

func TestValidateResult_InvalidCitation(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "all requirements met",
				Citations: []review.Citation{
					{Path: "file1.go", Line: 999}, // This line doesn't exist in the diff
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

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for invalid citation")
	assert.True(t, containsError(errs, "file1.go") || containsError(errs, "999"), "Expected file/line reference in error")
}

func TestValidateResult_InvalidCriterionResult(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:     "definition_of_done",
				Status: review.NotSatisfied,
				// Missing both rationale and missing evidence
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx, err := review.BuildDiffIndex("")
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.True(t, len(errs) > 0, "Expected validation errors for invalid criterion result")
}

func TestValidateResult_MultipleCitations(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "evidence from multiple files",
				Citations: []review.Citation{
					{Path: "file1.go", Line: 1},
					{Path: "file2.go", Line: 5},
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

	idx, err := review.BuildDiffIndex(diff)
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors for multiple citations")
}

func TestNewAttestation(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{ID: "def_of_done", Status: review.Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: review.PartiallySatisfied, Rationale: "partial"},
			{ID: "acceptance[1]", Status: review.NotSatisfied, Rationale: "not met", MissingEvidence: "not implemented"},
			{ID: "acceptance[2]", Status: review.Indeterminate, Rationale: "unclear"},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	att := review.NewAttestation(assessment)

	assert.NotNil(t, att)
	assert.Equal(t, "sha256:test123", att.BundleID)
	assert.Equal(t, "sha256:contract123", att.ContractFingerprint)
	assert.Equal(t, "sha256:delivery123", att.DeliveryFingerprint)
	// 1 satisfied, 1 partial, 1 not satisfied, 1 indeterminate
	// DeriveRating prioritizes: Red > Yellow > Green
	// Has NotSatisfied -> Red
	assert.Equal(t, review.Red, att.Rating)
	assert.Equal(t, 1, att.SatisfiedCount)
	assert.Equal(t, 1, att.PartiallySatisfiedCount)
	assert.Equal(t, 1, att.NotSatisfiedCount)
	assert.Equal(t, 1, att.IndeterminateCount)
}

func TestNewAttestation_AllGreen(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test456",
		Results: []review.CriterionResult{
			{ID: "def_of_done", Status: review.Satisfied, Rationale: "ok"},
			{ID: "acceptance[0]", Status: review.Satisfied, Rationale: "ok"},
		},
		ContractFingerprint: "sha256:contract456",
		DeliveryFingerprint: "sha256:delivery456",
	}

	att := review.NewAttestation(assessment)

	assert.Equal(t, review.Green, att.Rating)
	assert.Equal(t, 2, att.SatisfiedCount)
	assert.Equal(t, 0, att.PartiallySatisfiedCount)
	assert.Equal(t, 0, att.NotSatisfiedCount)
	assert.Equal(t, 0, att.IndeterminateCount)
}

func TestIsDuplicate_SameResultFingerprint(t *testing.T) {
	t.Parallel()
	// Same ResultFingerprint → identical content → duplicate regardless of BundleID.
	att1 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}

	assert.True(t, review.IsDuplicate(att1, att2))
}

func TestIsDuplicate_DifferentResultFingerprint(t *testing.T) {
	t.Parallel()
	// Same bundle but different ResultFingerprint → corrected assessment → not a duplicate.
	att1 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		ResultFingerprint: "sha256:fingerprint456",
	}

	assert.False(t, review.IsDuplicate(att1, att2))
}

func TestIsDuplicate_SameFingerprintDifferentSkillVersion(t *testing.T) {
	t.Parallel()
	// Same ResultFingerprint with different SkillVersion → still identical content → duplicate.
	att1 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		SkillVersion:      "v1.0.0",
		ResultFingerprint: "sha256:fingerprint123",
	}
	att2 := &review.AssessmentAttestation{
		BundleID:          "sha256:bundle123",
		SkillVersion:      "v2.0.0",
		ResultFingerprint: "sha256:fingerprint123",
	}

	assert.True(t, review.IsDuplicate(att1, att2))
}

func TestApplicable_Matching(t *testing.T) {
	t.Parallel()
	att := &review.AssessmentAttestation{
		BundleID: "sha256:bundle123",
	}

	assert.True(t, review.Applicable(att, "sha256:bundle123"))
}

func TestApplicable_NonMatching(t *testing.T) {
	t.Parallel()
	att := &review.AssessmentAttestation{
		BundleID: "sha256:bundle123",
	}

	assert.False(t, review.Applicable(att, "sha256:bundle456"))
}

func TestValidateResultNoDiff_Valid(t *testing.T) {
	t.Parallel()
	// An assessment with a file:line citation must succeed at record time (no diff available).
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "implementation complete",
				Citations: []review.Citation{
					{Path: "internal/review/types.go", Line: 42},
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	errs := review.ValidateResultNoDiff(assessment)
	assert.Empty(t, errs, "citation with file/line must not be rejected at record time")
}

func TestValidateResultNoDiff_EmptyBundleID(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		BundleID: "",
		Results: []review.CriterionResult{
			{ID: "def", Status: review.Satisfied, Rationale: "ok"},
		},
	}

	errs := review.ValidateResultNoDiff(assessment)
	assert.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "bundle ID"))
}

func TestValidateResultNoDiff_InvalidResult(t *testing.T) {
	t.Parallel()
	assessment := &review.ConformanceAssessment{
		BundleID: "sha256:bundle123",
		Results: []review.CriterionResult{
			{ID: "", Status: review.Satisfied, Rationale: ""},
		},
	}

	errs := review.ValidateResultNoDiff(assessment)
	assert.NotEmpty(t, errs)
}

// Helper function to check if an error message contains a substring
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
	// A NotSatisfied criterion with no citations but with MissingEvidence text should be valid
	assessment := &review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:test123",
		Results: []review.CriterionResult{
			{
				ID:              "definition_of_done",
				Status:          review.NotSatisfied,
				Rationale:       "requirement not met",
				MissingEvidence: "feature X not implemented",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	idx, err := review.BuildDiffIndex("")
	require.NoError(t, err)

	errs := review.ValidateResult(assessment, idx)
	assert.Len(t, errs, 0, "Expected no validation errors for NotSatisfied with MissingEvidence")
}
