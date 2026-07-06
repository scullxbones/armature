package review

import (
	"encoding/json"
	"os"
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
	bundle.BundleID = ComputeBundleID(*bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
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
	assert.Equal(t, bundle.BundleID, result.Attestation.BundleID)
	assert.Equal(t, "base123", result.Attestation.BaseSHA)
	assert.Equal(t, "head456", result.Attestation.HeadSHA)
	assert.Equal(t, Green, result.Attestation.Rating)
	assert.Equal(t, 2, result.Attestation.SatisfiedCount)
}

// TestRecord_BundleIntegrityTampered_REQ_EXECEV verifies that Record recomputes
// ComputeBundleID from the loaded bundle's actual contents and rejects the
// bundle if it no longer matches the bundle's recorded BundleID. This guards
// against a hand-edited bundle file (e.g. Delivery.HeadSHA blanked out to skip
// the HeadSHA-citation gate) that would otherwise pass every check that only
// compares fields *within* the same untrusted bundle file.
func TestRecord_BundleIntegrityTampered_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Test Task"},
		Contract:      Contract{DefinitionOfDone: "Implementation complete"},
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
	bundle.BundleID = ComputeBundleID(*bundle)

	// Tamper with the bundle after its BundleID was computed (e.g. blank out
	// HeadSHA to try to dodge the HeadSHA-citation gate) without recomputing the ID.
	bundle.Delivery.HeadSHA = ""

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "Done"},
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
	assert.Contains(t, err.Error(), "integrity")
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
		Issue: IssueInfo{
			ID: "task-02",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}
	bundle.BundleID = ComputeBundleID(*bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
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
		Issue: IssueInfo{
			ID: "task-01",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaa",
			Delivery: "sha256:bbbb",
		},
	}
	bundle.BundleID = ComputeBundleID(*bundle)

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
	assert.Contains(t, err.Error(), bundle.BundleID)
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
	bundle.BundleID = ComputeBundleID(*bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
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
	bundle.BundleID = ComputeBundleID(*bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
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

// TestRecord_ActivityDigestPopulatedInAttestation_REQ_EXECEV verifies that
// AssessmentAttestation.ActivityDigest is populated from the bundle's Activity
// section (M3 / ADR-0008: "the digest enters the attestation").
func TestRecord_ActivityDigestPopulatedInAttestation_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"
	logContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make build","exit_code":0,` +
		`"exit_code_known":true,"head_sha":"head","output_hash":"h1"}` + "\n")
	require.NoError(t, os.WriteFile(logPath, logContent, 0o600))
	digest := FingerprintActivity(logContent)

	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Test"},
		Contract:      Contract{DefinitionOfDone: "Done", Acceptance: []string{"Works"}},
		Delivery:      Delivery{BaseSHA: "base", HeadSHA: "head", Diff: "--- a/f.go\n+++ b/f.go\n@@ -1,0 +1,1 @@\n+package main"},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Activity: &Activity{Digest: digest, EntryCount: 1, LogPath: logPath},
	}
	bundle.BundleID = ComputeBundleID(*bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{{Path: "f.go", Line: 1}}},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ran", Citations: []Citation{{ActivityEntryID: "0"}}},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.NoError(t, err)
	assert.Equal(t, digest, result.Attestation.ActivityDigest, "attestation must carry the bundle's activity digest")
}

// TestRecord_RejectsActivityCitationsWithoutBundleActivity_REQ_EXECEV verifies that
// an assessment citing activity log entries is rejected outright when no bundle
// Activity section is available to validate against (M4) -- both when the bundle
// itself is nil and when the bundle has no Activity section.
func TestRecord_RejectsActivityCitationsWithoutBundleActivity_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-no-activity",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{{ActivityEntryID: "0"}}},
		},
	}

	t.Run("nil bundle", func(t *testing.T) {
		t.Parallel()
		_, err := Record(RecordInput{Assessment: assessment, IssueID: "task-01"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity")
	})

	t.Run("bundle without activity section", func(t *testing.T) {
		t.Parallel()
		bundle := &ReviewBundle{
			SchemaVersion: SchemaVersion,
			Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Test"},
			Contract:      Contract{DefinitionOfDone: "Done"},
			Delivery:      Delivery{BaseSHA: "base", HeadSHA: "head"},
			Fingerprints: Fingerprints{
				Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		}
		bundle.BundleID = ComputeBundleID(*bundle)
		_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity")
	})
}

// TestRecord_AlwaysResetsInboundActivityEntryDetails_REQ_EXECEV verifies that a
// reviewer-supplied ActivityEntryDetails value is always discarded at record time,
// even when there is no bundle activity section to populate it from -- it must
// never survive into the recorded/published citation as model-authored prose
// dressed as harness-verified fact (M10).
func TestRecord_AlwaysResetsInboundActivityEntryDetails_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-reset",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{
				ID: "acceptance[0]", Status: Satisfied, Rationale: "ok",
				Citations: []Citation{{Path: "f.go", Line: 1, ActivityEntryDetails: "fabricated: exit_code=0 all tests passed"}},
			},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, IssueID: "task-01"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, assessment.Results[0].Citations[0].ActivityEntryDetails,
		"inbound ActivityEntryDetails must be reset, not passed through")
}

// TestCitationValid_RejectsMutualExclusivity_REQ_EXECEV verifies that a citation
// with both Path and ActivityEntryID set is rejected (M5): such a citation would
// otherwise be skipped by diff-index validation (since activity citations are
// validated separately) while still counting as a diff citation for
// upgrade-only-rule purposes, letting a fabricated Path escape verification.
func TestCitationValid_RejectsMutualExclusivity_REQ_EXECEV(t *testing.T) {
	t.Parallel()
	result := CriterionResult{
		ID:        "acceptance[0]",
		Status:    Satisfied,
		Rationale: "ok",
		Citations: []Citation{{Path: "f.go", Line: 1, ActivityEntryID: "0"}},
	}
	err := result.Valid()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// TestParseActivityLogFile_IDsAreLinePositionNotSequentialCount_REQ_EXECEV verifies
// that entry IDs are assigned by physical line number, so a malformed or blank
// line does not shift the IDs of entries that come after it (m1). Without this,
// a citation naming entry "2" (meant for the third physical line) could resolve
// to the wrong parsed entry once an earlier line failed to parse.
func TestParseActivityLogFile_IDsAreLinePositionNotSequentialCount_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"
	content := `{"timestamp":"t0","command":"first","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}
this line is not valid JSON at all
{"timestamp":"t2","command":"third","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}
`
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o600))

	entries, _, err := parseActivityLogFile(logPath)
	require.NoError(t, err)
	require.Len(t, entries, 2, "the malformed line should be skipped, leaving 2 valid entries")

	first, ok := entries[0]
	require.True(t, ok, "the first entry must keep ID 0 (physical line 0)")
	assert.Equal(t, "first", first.Command)

	_, malformedPresent := entries[1]
	assert.False(t, malformedPresent, "the malformed physical line 1 must not produce an entry")

	third, ok := entries[2]
	require.True(t, ok, "the third entry must be at ID 2 (physical line 2), not shifted to ID 1")
	assert.Equal(t, "third", third.Command)
}

// TestParseActivityLogFile_HandlesOversizedLine_REQ_EXECEV verifies that a single
// large activity log line (larger than bufio.Scanner's default 64KB token limit)
// does not fail the entire scan and silently drop the whole activity section (M9).
func TestParseActivityLogFile_HandlesOversizedLine_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"

	// Build a line whose JSON-encoded size exceeds bufio.Scanner's default 64KB
	// token limit but stays within the raised buffer.
	bigOutput := make([]byte, 100*1024)
	for i := range bigOutput {
		bigOutput[i] = 'x'
	}
	line := activityLogLine{
		Timestamp:     "2026-01-15T10:30:45Z",
		Command:       "make build",
		ExitCode:      0,
		ExitCodeKnown: true,
		HeadSHA:       "abc123",
		OutputHash:    "hash",
		OutputHead:    string(bigOutput),
	}
	data, err := json.Marshal(line)
	require.NoError(t, err)
	require.Greater(t, len(data), 64*1024, "test line must exceed the default scanner token limit")
	require.NoError(t, os.WriteFile(logPath, append(data, '\n'), 0o600))

	entries, _, err := parseActivityLogFile(logPath)
	require.NoError(t, err, "an oversized line must not fail the whole scan")
	require.Len(t, entries, 1)
	assert.Equal(t, "make build", entries[0].Command)
}
