package review_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
)

func TestFingerprintContract_Deterministic(t *testing.T) {
	t.Parallel()
	contract := review.Contract{
		DefinitionOfDone: "All tests pass and docs updated",
		Acceptance: []string{
			"Feature implemented correctly",
			"Code reviewed and approved",
		},
	}

	fp1 := review.FingerprintContract(contract)
	fp2 := review.FingerprintContract(contract)

	// Same input produces same fingerprint
	assert.Equal(t, fp1, fp2)
	// Fingerprint is not empty
	assert.NotEmpty(t, fp1)
	// Fingerprint has correct format
	assert.True(t, len(fp1) > 0)
}

func TestFingerprintContract_Different(t *testing.T) {
	t.Parallel()
	contract1 := review.Contract{
		DefinitionOfDone: "All tests pass",
		Acceptance:       []string{"Feature works"},
	}

	contract2 := review.Contract{
		DefinitionOfDone: "All tests pass",
		Acceptance:       []string{"Feature works", "Documented"},
	}

	fp1 := review.FingerprintContract(contract1)
	fp2 := review.FingerprintContract(contract2)

	// Different contracts produce different fingerprints
	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintContract_OrderMatters(t *testing.T) {
	t.Parallel()
	contract1 := review.Contract{
		DefinitionOfDone: "Definition",
		Acceptance: []string{
			"Acceptance 1",
			"Acceptance 2",
		},
	}

	contract2 := review.Contract{
		DefinitionOfDone: "Definition",
		Acceptance: []string{
			"Acceptance 2",
			"Acceptance 1",
		},
	}

	fp1 := review.FingerprintContract(contract1)
	fp2 := review.FingerprintContract(contract2)

	// Different order produces different fingerprints
	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintDelivery_Deterministic(t *testing.T) {
	t.Parallel()
	delivery := review.Delivery{
		BaseSHA:      "abc123def456",
		HeadSHA:      "fedcba654321",
		ChangedFiles: []string{"file1.go", "file2.go"},
	}

	fp1 := review.FingerprintDelivery(delivery)
	fp2 := review.FingerprintDelivery(delivery)

	// Same input produces same fingerprint
	assert.Equal(t, fp1, fp2)
	// Fingerprint is not empty
	assert.NotEmpty(t, fp1)
}

func TestFingerprintDelivery_Different(t *testing.T) {
	t.Parallel()
	delivery1 := review.Delivery{
		BaseSHA:      "abc123",
		HeadSHA:      "def456",
		ChangedFiles: []string{"file.go"},
	}

	delivery2 := review.Delivery{
		BaseSHA:      "abc123",
		HeadSHA:      "def457", // different head
		ChangedFiles: []string{"file.go"},
	}

	fp1 := review.FingerprintDelivery(delivery1)
	fp2 := review.FingerprintDelivery(delivery2)

	// Different deliveries produce different fingerprints
	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintResult_Deterministic(t *testing.T) {
	t.Parallel()
	result := review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Complete",
			},
			{
				ID:        "acceptance[0]",
				Status:    review.PartiallySatisfied,
				Rationale: "Partial",
				Citations: []review.Citation{
					{Path: "main.go", Line: 42},
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	fp1 := review.FingerprintResult(result)
	fp2 := review.FingerprintResult(result)

	assert.Equal(t, fp1, fp2)
	assert.NotEmpty(t, fp1)
}

func TestFingerprintResult_Different(t *testing.T) {
	t.Parallel()
	result1 := review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "Complete",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	result2 := review.ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []review.CriterionResult{
			{
				ID:              "definition_of_done",
				Status:          review.PartiallySatisfied,
				Rationale:       "Partial",
				MissingEvidence: "Not complete",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	fp1 := review.FingerprintResult(result1)
	fp2 := review.FingerprintResult(result2)

	assert.NotEqual(t, fp1, fp2)
}

func TestBundleID_Deterministic(t *testing.T) {
	t.Parallel()
	bundle := review.ReviewBundle{
		SchemaVersion: 1,
		Issue: review.IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: review.Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: review.Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
	}

	bundleID1 := review.ComputeBundleID(bundle)
	bundleID2 := review.ComputeBundleID(bundle)

	assert.Equal(t, bundleID1, bundleID2)
	assert.NotEmpty(t, bundleID1)
	// BundleID should have sha256: prefix
	assert.True(t, len(bundleID1) > 7)
}

func TestBundleID_Different(t *testing.T) {
	t.Parallel()
	bundle1 := review.ReviewBundle{
		SchemaVersion: 1,
		Issue: review.IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: review.Contract{
			DefinitionOfDone: "Definition 1",
		},
		Delivery: review.Delivery{
			BaseSHA: "abc123",
			HeadSHA: "def456",
		},
	}

	bundle2 := review.ReviewBundle{
		SchemaVersion: 1,
		Issue: review.IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: review.Contract{
			DefinitionOfDone: "Definition 2",
		},
		Delivery: review.Delivery{
			BaseSHA: "abc123",
			HeadSHA: "def456",
		},
	}

	bundleID1 := review.ComputeBundleID(bundle1)
	bundleID2 := review.ComputeBundleID(bundle2)

	assert.NotEqual(t, bundleID1, bundleID2)
}

func TestFingerprintFormat(t *testing.T) {
	t.Parallel()
	contract := review.Contract{
		DefinitionOfDone: "Test",
	}

	fp := review.FingerprintContract(contract)

	// Fingerprints should be hex-encoded SHA-256 hashes
	// SHA-256 produces 32 bytes = 64 hex characters
	assert.Len(t, fp, 64)
	// All characters should be valid hex
	for _, c := range fp {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"Fingerprint contains non-hex character: %c", c)
	}
}

func TestBundleIDFormat(t *testing.T) {
	t.Parallel()
	bundle := review.ReviewBundle{
		SchemaVersion: 1,
		Issue: review.IssueInfo{
			ID:   "TASK-1",
			Type: "task",
		},
		Contract: review.Contract{
			DefinitionOfDone: "Test",
		},
		Delivery: review.Delivery{
			BaseSHA: "abc",
			HeadSHA: "def",
		},
	}

	bundleID := review.ComputeBundleID(bundle)

	// Should have sha256: prefix followed by 64 hex characters
	assert.True(t, len(bundleID) > 7)
	assert.Contains(t, bundleID, ":")
	parts := len(bundleID) - len("sha256:")
	assert.Equal(t, 64, parts, "BundleID should have sha256:<64-hex-chars> format")
}
