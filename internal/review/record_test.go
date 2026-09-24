package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustComputeBundleID(t *testing.T, bundle ReviewBundle) string {
	t.Helper()
	id, err := ComputeBundleID(bundle)
	require.NoError(t, err)
	return id
}

func errsContain(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func mustUnmarshalCitation(t *testing.T, raw string) Citation {
	t.Helper()
	var c Citation
	require.NoError(t, json.Unmarshal([]byte(raw), &c))
	return c
}

func citedSatisfied(id, rationale string) CriterionResult {
	return CriterionResult{
		ID:        id,
		Status:    Satisfied,
		Rationale: rationale,
		Citations: []Citation{FileCitation("impl.go", 1, 0)},
	}
}

func TestRecordAssessmentDecision_REQ_ARCHIMP_S18_T1(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-123",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			citedSatisfied("definition_of_done", "Implementation is complete."),
		},
	}

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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			citedSatisfied("definition_of_done", "Implementation is complete."),
			citedSatisfied("acceptance[0]", "Feature works as designed."),
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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	bundle.Delivery.HeadSHA = ""

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			citedSatisfied("definition_of_done", "Done"),
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
			citedSatisfied("definition_of_done", "Implementation is complete."),
			citedSatisfied("acceptance[0]", "Feature works."),
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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			citedSatisfied("definition_of_done", "Done"),
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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-999",
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: "sha256:bbbb",
		Results: []CriterionResult{
			citedSatisfied("definition_of_done", "Done"),
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
			citedSatisfied("definition_of_done", "Done"),
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
			citedSatisfied("definition_of_done", "Done"),
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
			citedSatisfied("definition_of_done", "Implementation is complete."),
			citedSatisfied("acceptance[0]", "Feature works as designed."),
			citedSatisfied("acceptance[1]", "Edge cases properly handled."),
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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

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
					FileCitation("impl.go", 3, 0),
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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

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
					FileCitation("impl.go", 9999, 0),
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
			citedSatisfied("definition_of_done", "Done"),
		},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result1, err := Record(input)
	require.NoError(t, err)

	existingAtts := []AssessmentAttestation{*result1.Attestation}

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
			citedSatisfied("definition_of_done", "Done"),
		},
	}

	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}

	result1, err := Record(input)
	require.NoError(t, err)

	differentAtt := *result1.Attestation
	differentAtt.ResultFingerprint = "sha256:different"
	existingAtts := []AssessmentAttestation{differentAtt}

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
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("f.go", 1, 0)}},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ran", Citations: []Citation{ActivityCitation("0")}},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.NoError(t, err)
	assert.Equal(t, digest, result.Attestation.ActivityDigest, "attestation must carry the bundle's activity digest")
}

func TestRecord_RejectsActivityCitationsWithoutBundleActivity_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            "bundle-no-activity",
		ContractFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeliveryFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{ActivityCitation("0")}},
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
		bundle.BundleID = mustComputeBundleID(t, *bundle)
		_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "activity")
	})
}

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
				Citations: []Citation{func() Citation {
					c := FileCitation("f.go", 1, 0)
					c.SetActivityEntryDetails("fabricated: exit_code=0 all tests passed")
					return c
				}()},
			},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, IssueID: "task-01"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, assessment.Results[0].Citations[0].ActivityEntryDetails(),
		"inbound ActivityEntryDetails must be reset, not passed through")
}

func TestCitationValid_RejectsMutualExclusivity_REQ_EXECEV(t *testing.T) {
	t.Parallel()
	result := CriterionResult{
		ID:        "acceptance[0]",
		Status:    Satisfied,
		Rationale: "ok",
		Citations: []Citation{mustUnmarshalCitation(t, `{"path":"f.go","line":1,"activity_entry_id":"0"}`)},
	}
	err := result.Valid()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestParseActivityLogFile_IDsAreLinePositionNotSequentialCount_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"
	content := `{"timestamp":"t0","command":"first","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}

{"timestamp":"t2","command":"third","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}
`
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o600))

	entries, _, err := parseActivityLogFile(logPath)
	require.NoError(t, err)
	require.Len(t, entries, 2, "blank lines consume physical IDs but are not entries")

	first, ok := entries[0]
	require.True(t, ok, "the first entry must keep ID 0 (physical line 0)")
	assert.Equal(t, "first", first.Command)

	_, blankPresent := entries[1]
	assert.False(t, blankPresent, "the blank physical line 1 must not produce an entry")

	third, ok := entries[2]
	require.True(t, ok, "the third entry must be at ID 2 (physical line 2), not shifted to ID 1")
	assert.Equal(t, "third", third.Command)
}

func TestParseActivityLogFile_HandlesOversizedLine_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"

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

func TestRecord_ActivityCitationsWithDigestValidation_TOCTOU_Fix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"
	logContent := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make test","exit_code":0,` +
		`"exit_code_known":true,"head_sha":"head","output_hash":"h1"}` + "\n")
	require.NoError(t, os.WriteFile(logPath, logContent, 0o600))
	digest := FingerprintActivity(logContent)

	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-02", Type: "task", Title: "Activity Test"},
		Contract:      Contract{DefinitionOfDone: "Done", Acceptance: []string{"Test passed"}},
		Delivery: Delivery{
			BaseSHA: "base",
			HeadSHA: "head",
			Diff:    "--- a/t.go\n+++ b/t.go\n@@ -1,0 +1,1 @@\n+func Test()",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Activity: &Activity{
			Digest:            digest,
			EntryCount:        1,
			DeliveryHeadCount: 1,
			LogPath:           logPath,
		},
	}
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation ready",
				Citations: []Citation{FileCitation("t.go", 1, 0)},
			},
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "Tests pass",
				Citations: []Citation{ActivityCitation("0")},
			},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-02"})
	require.NoError(t, err, "Record should succeed with valid activity citations")
	require.NotNil(t, result)
	require.NotNil(t, result.Attestation)
	assert.Equal(t, digest, result.Attestation.ActivityDigest, "attestation must carry activity digest")
	assert.Equal(t, Green, result.Attestation.Rating, "activity citation should contribute to passing rating")
	assert.False(t, result.IsDuplicate)

	activityCitation := &assessment.Results[1].Citations[0]
	assert.NotEmpty(t, activityCitation.ActivityEntryDetails(), "activity entry details must be populated from the log")
	assert.Contains(t, activityCitation.ActivityEntryDetails(), "entry 0")
	assert.Contains(t, activityCitation.ActivityEntryDetails(), "make test")
	assert.Contains(t, activityCitation.ActivityEntryDetails(), "exit_code=0")
}

func TestValidateActivityDigestAndLoadEntries_FailurePaths(t *testing.T) {
	t.Parallel()

	t.Run("digest mismatch reports an error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		logPath := dir + "/armature-activity.log"
		original := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make test","exit_code":0,` +
			`"exit_code_known":true,"head_sha":"head"}` + "\n")
		require.NoError(t, os.WriteFile(logPath, original, 0o600))
		recordedDigest := FingerprintActivity(original)

		tampered := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"rm -rf /","exit_code":0,` +
			`"exit_code_known":true,"head_sha":"head"}` + "\n")
		require.NoError(t, os.WriteFile(logPath, tampered, 0o600))

		activity := &Activity{Digest: recordedDigest, EntryCount: 1, LogPath: logPath}
		entries, errs := ValidateActivityDigestAndLoadEntries(activity)
		require.NotEmpty(t, errs, "digest mismatch must be reported regardless of whether citations exist")
		assert.True(t, errsContain(errs, "digest mismatch"))

		assert.Len(t, entries, 1)
	})

	t.Run("unreadable log reports an error and returns no entries", func(t *testing.T) {
		t.Parallel()
		activity := &Activity{
			Digest:     "sha256:doesnotmatter",
			EntryCount: 1,
			LogPath:    t.TempDir() + "/does-not-exist.log",
		}
		entries, errs := ValidateActivityDigestAndLoadEntries(activity)
		require.NotEmpty(t, errs)
		assert.True(t, errsContain(errs, "missing or unreadable"))
		assert.Empty(t, entries)
	})

	t.Run("nil activity returns no entries and no errors", func(t *testing.T) {
		t.Parallel()
		entries, errs := ValidateActivityDigestAndLoadEntries(nil)
		assert.Empty(t, errs)
		assert.Empty(t, entries)
	})
}

func TestRecord_RejectsDigestMismatchEvenWithoutActivityCitations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := dir + "/armature-activity.log"
	original := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"make test","exit_code":0,` +
		`"exit_code_known":true,"head_sha":"head"}` + "\n")
	require.NoError(t, os.WriteFile(logPath, original, 0o600))
	recordedDigest := FingerprintActivity(original)

	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-03", Type: "task", Title: "No Activity Citations"},
		Contract:      Contract{DefinitionOfDone: "Done", Acceptance: []string{"Test passed"}},
		Delivery: Delivery{
			BaseSHA: "base",
			HeadSHA: "head",
			Diff:    "--- a/t.go\n+++ b/t.go\n@@ -1,0 +1,1 @@\n+func Test()",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Activity: &Activity{
			Digest:            recordedDigest,
			EntryCount:        1,
			DeliveryHeadCount: 1,
			LogPath:           logPath,
		},
	}
	bundle.BundleID = mustComputeBundleID(t, *bundle)

	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Implementation ready",
				Citations: []Citation{FileCitation("t.go", 1, 0)},
			},
			{
				ID:        "acceptance[0]",
				Status:    Satisfied,
				Rationale: "Tests pass",

				Citations: []Citation{FileCitation("t.go", 1, 0)},
			},
		},
	}

	result, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-03"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, recordedDigest, result.Attestation.ActivityDigest)

	tampered := []byte(`{"timestamp":"2026-01-15T10:30:45Z","command":"rm -rf /","exit_code":0,` +
		`"exit_code_known":true,"head_sha":"head"}` + "\n")
	require.NoError(t, os.WriteFile(logPath, tampered, 0o600))

	_, err = Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-03"})
	require.Error(t, err, "Record must reject a digest mismatch even when no activity citations are present, "+
		"since the attestation stamps activity.Digest unconditionally")
	assert.Contains(t, err.Error(), "digest mismatch")
}

func gateEvidenceRecordFixture(t *testing.T, ev ops.GateEvidence) (*ReviewBundle, *ConformanceAssessment) {
	t.Helper()
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue:         IssueInfo{ID: "task-01", Type: "task", Title: "Gate digest"},
		Contract:      Contract{DefinitionOfDone: "Done", Acceptance: []string{"Works"}},
		Delivery: Delivery{
			BaseSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			HeadSHA: ev.HeadSHA,
			Diff:    "--- a/f.go\n+++ b/f.go\n@@ -1,0 +1,1 @@\n+package main",
		},
		Fingerprints: Fingerprints{
			Contract: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Delivery: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		GateEvidence: []ops.GateEvidence{ev},
	}
	bundle.BundleID = mustComputeBundleID(t, *bundle)
	assessment := &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []CriterionResult{
			{ID: "definition_of_done", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("f.go", 1, 0)}},
			{ID: "acceptance[0]", Status: Satisfied, Rationale: "ok", Citations: []Citation{FileCitation("f.go", 1, 0)}},
		},
	}
	return bundle, assessment
}

func TestRecord_GateEvidenceHashVerified_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "full.log")
	content := []byte("gate output\n")
	require.NoError(t, os.WriteFile(logPath, content, 0o600))
	sum := sha256.Sum256(content)
	ev := ops.GateEvidence{
		Profile:    "full",
		Command:    []string{"true"},
		HeadSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:      1,
		End:        2,
		Exit:       0,
		OutputHash: hex.EncodeToString(sum[:]),
		LogPath:    logPath,
	}
	bundle, assessment := gateEvidenceRecordFixture(t, ev)
	_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.NoError(t, err)
}

func TestRecord_GateEvidenceTamperedLogFails_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "full.log")
	content := []byte("gate output\n")
	require.NoError(t, os.WriteFile(logPath, content, 0o600))
	sum := sha256.Sum256(content)
	ev := ops.GateEvidence{
		Profile:    "full",
		Command:    []string{"true"},
		HeadSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:      1,
		End:        2,
		Exit:       0,
		OutputHash: hex.EncodeToString(sum[:]),
		LogPath:    logPath,
	}
	bundle, assessment := gateEvidenceRecordFixture(t, ev)
	require.NoError(t, os.WriteFile(logPath, []byte("tampered\n"), 0o600))
	_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gate")
}

func TestRecord_GateEvidenceMissingHashFails_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "full.log")
	require.NoError(t, os.WriteFile(logPath, []byte("gate output\n"), 0o600))
	ev := ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:   1,
		End:     2,
		Exit:    0,
		LogPath: logPath,
	}
	bundle, assessment := gateEvidenceRecordFixture(t, ev)
	_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output_hash")
}

func TestRecord_GateEvidenceMissingLogFails_REQ_LNGHZN_S10_T3(t *testing.T) {
	t.Parallel()
	ev := ops.GateEvidence{
		Profile:    "full",
		Command:    []string{"true"},
		HeadSHA:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Start:      1,
		End:        2,
		Exit:       0,
		OutputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		LogPath:    filepath.Join(t.TempDir(), "missing.log"),
	}
	bundle, assessment := gateEvidenceRecordFixture(t, ev)
	_, err := Record(RecordInput{Assessment: assessment, Bundle: bundle, IssueID: "task-01"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gate")
}

const disagreementDeliveryFP = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func disagreementAssessment(bundleID, rationale string, status CriterionStatus) *ConformanceAssessment {
	result := CriterionResult{
		ID:        "definition_of_done",
		Status:    status,
		Rationale: rationale,
	}
	if status == Satisfied {
		result.Citations = []Citation{FileCitation("impl.go", 1, 0)}
	} else {
		result.MissingEvidence = "not demonstrated"
	}
	return &ConformanceAssessment{
		SchemaVersion:       SchemaVersion,
		BundleID:            bundleID,
		ContractFingerprint: "sha256:aaaa",
		DeliveryFingerprint: disagreementDeliveryFP,
		Results:             []CriterionResult{result},
	}
}

func recordDisagreementAttestation(t *testing.T, bundleID, rationale string, status CriterionStatus) AssessmentAttestation {
	t.Helper()
	result, err := Record(RecordInput{
		Assessment: disagreementAssessment(bundleID, rationale, status),
		IssueID:    "task-01",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Attestation)
	return *result.Attestation
}

func TestReviewRecord_HandlesConflictingRatings_REQ_TOPTIER_S13(t *testing.T) {
	t.Parallel()

	t.Run("no prior", func(t *testing.T) {
		t.Parallel()
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-new", "complete", Satisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, nil)
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.False(t, result.IsDuplicate)
		assert.Equal(t, Green, result.Attestation.Rating)
		assert.Equal(t, Green, result.Attestation.EffectiveRating)
		assert.False(t, result.Attestation.IsDisagreement)
		assert.Empty(t, result.Attestation.ConflictsWithBundleID)
		assert.Nil(t, result.Attestation.ConflictsWithRating)
	})

	t.Run("agreeing prior", func(t *testing.T) {
		t.Parallel()
		prior := recordDisagreementAttestation(t, "bundle-prior", "first green review", Satisfied)
		snapshot := prior
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-new", "second green review", Satisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{prior})
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.False(t, result.IsDuplicate)
		assert.Equal(t, Green, result.Attestation.Rating)
		assert.Equal(t, Green, result.Attestation.EffectiveRating)
		assert.False(t, result.Attestation.IsDisagreement)
		assert.Empty(t, result.Attestation.ConflictsWithBundleID)
		assert.Equal(t, snapshot, prior, "prior Assessment Attestation must not be rewritten")
	})

	t.Run("disagreeing prior", func(t *testing.T) {
		t.Parallel()
		prior := recordDisagreementAttestation(t, "bundle-green", "green review", Satisfied)
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-red", "red review", NotSatisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{prior})
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.False(t, result.IsDuplicate)
		assert.Equal(t, Red, result.Attestation.Rating)
		assert.Equal(t, Red, result.Attestation.EffectiveRating)
		assert.True(t, result.Attestation.IsDisagreement)
		assert.Equal(t, "bundle-green", result.Attestation.ConflictsWithBundleID)
		require.NotNil(t, result.Attestation.ConflictsWithRating)
		assert.Equal(t, Green, *result.Attestation.ConflictsWithRating)
		assert.Equal(t, Green, prior.Rating)
		assert.False(t, prior.IsDisagreement)
		assert.Equal(t, Rating(0), prior.EffectiveRating)
	})

	t.Run("three mixed highest severity not most recent", func(t *testing.T) {
		t.Parallel()
		redPrior := recordDisagreementAttestation(t, "bundle-red", "older red", NotSatisfied)
		yellowPrior := recordDisagreementAttestation(t, "bundle-yellow", "newer yellow", PartiallySatisfied)
		otherDelivery := recordDisagreementAttestation(t, "bundle-other", "other delivery green", Satisfied)
		otherDelivery.DeliveryFingerprint = "sha256:other-delivery"
		priors := []AssessmentAttestation{redPrior, yellowPrior, otherDelivery}
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-green", "incoming green", Satisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, priors)
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.False(t, result.IsDuplicate)
		assert.Equal(t, Green, result.Attestation.Rating)
		assert.Equal(t, Red, result.Attestation.EffectiveRating)
		assert.True(t, result.Attestation.IsDisagreement)
		assert.Equal(t, "bundle-red", result.Attestation.ConflictsWithBundleID)
		require.NotNil(t, result.Attestation.ConflictsWithRating)
		assert.Equal(t, Red, *result.Attestation.ConflictsWithRating)
		assert.Equal(t, Red, priors[0].Rating)
		assert.Equal(t, Yellow, priors[1].Rating)
		assert.False(t, priors[0].IsDisagreement)
		assert.False(t, priors[1].IsDisagreement)
	})

	t.Run("equal severity cites most recently appended", func(t *testing.T) {
		t.Parallel()
		older := recordDisagreementAttestation(t, "bundle-yellow-old", "older yellow", PartiallySatisfied)
		newer := recordDisagreementAttestation(t, "bundle-yellow-new", "newer yellow", PartiallySatisfied)
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-green", "incoming green", Satisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{older, newer})
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.True(t, result.Attestation.IsDisagreement)
		assert.Equal(t, Yellow, result.Attestation.EffectiveRating)
		assert.Equal(t, "bundle-yellow-new", result.Attestation.ConflictsWithBundleID)
		require.NotNil(t, result.Attestation.ConflictsWithRating)
		assert.Equal(t, Yellow, *result.Attestation.ConflictsWithRating)
	})

	t.Run("incoming red cites disagreeing green not same-rating red", func(t *testing.T) {
		t.Parallel()
		greenPrior := recordDisagreementAttestation(t, "bundle-green", "older green", Satisfied)
		redPrior := recordDisagreementAttestation(t, "bundle-red-prior", "newer same-rating red", NotSatisfied)
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-red-new", "incoming red", NotSatisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{greenPrior, redPrior})
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.False(t, result.IsDuplicate)
		assert.Equal(t, Red, result.Attestation.Rating)
		assert.Equal(t, Red, result.Attestation.EffectiveRating)
		assert.True(t, result.Attestation.IsDisagreement)
		assert.Equal(t, "bundle-green", result.Attestation.ConflictsWithBundleID)
		require.NotNil(t, result.Attestation.ConflictsWithRating)
		assert.Equal(t, Green, *result.Attestation.ConflictsWithRating)
		assert.Equal(t, Green, greenPrior.Rating)
		assert.Equal(t, Red, redPrior.Rating)
		assert.False(t, greenPrior.IsDisagreement)
		assert.False(t, redPrior.IsDisagreement)
	})

	t.Run("exact duplicate prior excluded", func(t *testing.T) {
		t.Parallel()
		duplicateOfIncoming := recordDisagreementAttestation(t, "bundle-dup", "same content", Satisfied)
		disagreeing := recordDisagreementAttestation(t, "bundle-red", "conflicting red", NotSatisfied)
		input := RecordInput{
			Assessment: disagreementAssessment("bundle-dup", "same content", Satisfied),
			IssueID:    "task-01",
		}
		result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{disagreeing, duplicateOfIncoming})
		require.NoError(t, err)
		require.NotNil(t, result.Attestation)
		assert.True(t, result.IsDuplicate, "exact ResultFingerprint match takes the duplicate path")
		assert.False(t, result.Attestation.IsDisagreement)
		assert.Empty(t, result.Attestation.ConflictsWithBundleID)
		assert.Equal(t, Rating(0), result.Attestation.EffectiveRating)
	})
}

func TestReviewRecord_ConformanceRatingNeverOverwritten_REQ_TOPTIER_S13_T1(t *testing.T) {
	t.Parallel()
	prior := recordDisagreementAttestation(t, "bundle-red", "prior red", NotSatisfied)
	assessment := disagreementAssessment("bundle-green", "incoming green", Satisfied)
	input := RecordInput{
		Assessment: assessment,
		IssueID:    "task-01",
	}
	result, err := RecordWithDuplicateCheck(input, []AssessmentAttestation{prior})
	require.NoError(t, err)
	require.NotNil(t, result.Attestation)
	assert.Equal(t, DeriveRating(assessment.Results), result.Attestation.Rating)
	assert.Equal(t, Green, result.Attestation.Rating)
	assert.Equal(t, Red, result.Attestation.EffectiveRating)
	assert.NotEqual(t, result.Attestation.Rating, result.Attestation.EffectiveRating)
	assert.Equal(t, Red, prior.Rating)
}

func TestEnrichDisagreementFields_NilAttestation(t *testing.T) {
	t.Parallel()
	enrichDisagreementFields(nil, []AssessmentAttestation{{Rating: Red}})
}
