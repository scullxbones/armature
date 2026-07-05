package review

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordAssessmentDecision_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Create a minimal valid assessment
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation is complete.",
			},
		},
	}

	// Record without issue data (minimal path)
	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Attestation)
	assert.Equal(t, "bundle-123", result.Attestation.BundleID)
	assert.Equal(t, Green, result.Attestation.Rating)
	assert.False(t, result.IsDuplicate)
}

func TestRecord_WithBundle_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Create a complete bundle and matching assessment
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		Issue: IssueInfo{
			ID:    "task-01",
			Type:  "task",
			Title: "Test Task",
		},
		Contract: Contract{
			DefinitionOfDone: "Implementation complete",
			Acceptance: []string{
				"Feature works correctly",
			},
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
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation is complete.",
			},
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "Feature works as designed.",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		Bundle:     bundle,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "bundle-123", result.Attestation.BundleID)
	assert.Equal(t, "base123", result.Attestation.BaseSHA)
	assert.Equal(t, "head456", result.Attestation.HeadSHA)
	assert.Equal(t, Green, result.Attestation.Rating)
	assert.Equal(t, 2, result.Attestation.SatisfiedCount)
}

func TestRecord_WithIssueData_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Create acceptance criteria
	acceptanceCriteria := []string{"Feature works correctly"}
	acceptanceJSON, err := json.Marshal(acceptanceCriteria)
	require.NoError(t, err)

	assessment := &ConformanceAssessment{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		ContractFingerprint: FingerprintContract(Contract{
			DefinitionOfDone: "Implementation complete",
			Scope:            []string{"impl.go"},
			Acceptance:       acceptanceCriteria,
		}),
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation is complete.",
			},
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "Feature works.",
			},
		},
	}

	issueData := &IssueData{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       string(acceptanceJSON),
	}

	input := RecordInput{
		Assessment: assessment,
		Issue:      issueData,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, Green, result.Attestation.Rating)
}

func TestRecord_AssessmentNil_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	input := RecordInput{
		Assessment: nil,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "assessment is required")
}

func TestRecord_IssueIDEmpty_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "issue ID is required")
}

func TestRecord_AssessmentInvalid_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Missing required fields
	assessment := &ConformanceAssessment{
		SchemaVersion: SchemaVersion,
		BundleID:      "",
		Results:       []CriterionResult{},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestRecord_BundleIssueMismatch_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		Issue: IssueInfo{
			ID: "task-02",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		Bundle:     bundle,
		IssueID:    "task-01", // Mismatch with bundle.Issue.ID
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "bundle was prepared for issue task-02")
	assert.Contains(t, err.Error(), "not task-01")
}

func TestRecord_BundleIDMismatch_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		Issue: IssueInfo{
			ID: "task-01",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-999", // Mismatch
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		Bundle:     bundle,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "bundle_id")
	assert.Contains(t, err.Error(), "bundle-999")
	assert.Contains(t, err.Error(), "bundle-123")
}

func TestRecord_ContractFingerprintMismatch_RejectsWithoutBundle_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	acceptanceJSON, err := json.Marshal([]string{})
	require.NoError(t, err)

	issueData := &IssueData{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       string(acceptanceJSON),
	}

	input := RecordInput{
		Assessment: assessment,
		Issue:      issueData,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "contract fingerprint")
}

func TestRecord_CoverageMissingCriterion_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	acceptanceCriteria := []string{"Feature works correctly"}
	acceptanceJSON, err := json.Marshal(acceptanceCriteria)
	require.NoError(t, err)

	contract := Contract{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       acceptanceCriteria,
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: FingerprintContract(contract),
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
			// Missing acceptance[0]
		},
	}

	issueData := &IssueData{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       string(acceptanceJSON),
	}

	input := RecordInput{
		Assessment: assessment,
		Issue:      issueData,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "coverage")
	assert.Contains(t, err.Error(), "acceptance[0]")
}

func TestRecord_ValidCoverage_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	acceptanceCriteria := []string{"Feature works", "Edge cases handled"}
	acceptanceJSON, err := json.Marshal(acceptanceCriteria)
	require.NoError(t, err)

	contract := Contract{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       acceptanceCriteria,
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: FingerprintContract(contract),
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation is complete.",
			},
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "Feature works as designed.",
			},
			{
				ID:        "acceptance[1]",
				Status:    Satisfied,
				Rationale: "Edge cases properly handled.",
			},
		},
	}

	issueData := &IssueData{
		DefinitionOfDone: "Implementation complete",
		Scope:            []string{"impl.go"},
		Acceptance:       string(acceptanceJSON),
	}

	input := RecordInput{
		Assessment: assessment,
		Issue:      issueData,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, result.Attestation.SatisfiedCount)
}

func TestRecord_WithDiffIndexValidation_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	// Create a bundle with a diff
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		Issue: IssueInfo{
			ID: "task-01",
		},
		Contract: Contract{
			DefinitionOfDone: "Done",
		},
		Delivery: Delivery{
			BaseSHA: "base123",
			HeadSHA: "head456",
			Diff: `--- a/impl.go
+++ b/impl.go
@@ -1,0 +1,3 @@
+package main
+
+func New() {}`,
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
				Citations: []Citation{
					{Path: "impl.go", Line: 3},
				},
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		Bundle:     bundle,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRecord_InvalidCitationCoordinates_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bundle-123",
		Issue: IssueInfo{
			ID: "task-01",
		},
		Delivery: Delivery{
			BaseSHA: "base123",
			HeadSHA: "head456",
			Diff: `--- a/impl.go
+++ b/impl.go
@@ -1,0 +1,3 @@
+package main`,
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
				Citations: []Citation{
					{Path: "impl.go", Line: 9999}, // Does not exist in diff
				},
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		Bundle:     bundle,
		IssueID:    "task-01",
	}

	result, err := Record(input)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "citation")
}

func TestRecordWithDuplicateCheck_Duplicate_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	// Record once
	result1, err := Record(input)
	require.NoError(t, err)

	// Create an existing attestation with same result fingerprint
	existingAtts := []AssessmentAttestation{*result1.Attestation}

	// Try to record the same assessment again
	result2, err := RecordWithDuplicateCheck(input, existingAtts)
	require.NoError(t, err)
	assert.True(t, result2.IsDuplicate)
	assert.NotNil(t, result2.Attestation)
}

func TestRecordWithDuplicateCheck_NotDuplicate_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Done",
			},
		},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result1, err := Record(input)
	require.NoError(t, err)

	// Create a different existing attestation
	differentAtt := *result1.Attestation
	differentAtt.ResultFingerprint = "sha256:different"
	existingAtts := []AssessmentAttestation{differentAtt}

	// Try to record the assessment
	result2, err := RecordWithDuplicateCheck(input, existingAtts)
	require.NoError(t, err)
	assert.False(t, result2.IsDuplicate)
}

func TestRecordWithDuplicateCheck_Error_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()
	assessment := &ConformanceAssessment{
		SchemaVersion: SchemaVersion,
		BundleID:      "",
		Results:       []CriterionResult{},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result, err := RecordWithDuplicateCheck(input, nil)
	assert.Error(t, err)
	assert.Nil(t, result)
}
