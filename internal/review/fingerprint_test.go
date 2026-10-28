package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFingerprintContract_Deterministic(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "All tests pass and docs updated",
		Acceptance: []string{
			"Feature implemented correctly",
			"Code reviewed and approved",
		},
	}

	fp1 := fingerprintContract(contract)
	fp2 := fingerprintContract(contract)

	assert.Equal(t, fp1, fp2)

	assert.NotEmpty(t, fp1)

	assert.True(t, len(fp1) > 0)
}

func TestFingerprintContract_Different(t *testing.T) {
	t.Parallel()
	contract1 := Contract{
		DefinitionOfDone: "All tests pass",
		Acceptance:       []string{"Feature works"},
	}

	contract2 := Contract{
		DefinitionOfDone: "All tests pass",
		Acceptance:       []string{"Feature works", "Documented"},
	}

	fp1 := fingerprintContract(contract1)
	fp2 := fingerprintContract(contract2)

	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintContract_OrderMatters(t *testing.T) {
	t.Parallel()
	contract1 := Contract{
		DefinitionOfDone: "Definition",
		Acceptance: []string{
			"Acceptance 1",
			"Acceptance 2",
		},
	}

	contract2 := Contract{
		DefinitionOfDone: "Definition",
		Acceptance: []string{
			"Acceptance 2",
			"Acceptance 1",
		},
	}

	fp1 := fingerprintContract(contract1)
	fp2 := fingerprintContract(contract2)

	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintDelivery_Deterministic(t *testing.T) {
	t.Parallel()
	delivery := Delivery{
		BaseSHA:      "abc123def456",
		HeadSHA:      "fedcba654321",
		ChangedFiles: []string{"file1.go", "file2.go"},
	}

	fp1 := fingerprintDelivery(delivery)
	fp2 := fingerprintDelivery(delivery)

	assert.Equal(t, fp1, fp2)

	assert.NotEmpty(t, fp1)
}

func TestFingerprintDelivery_Different(t *testing.T) {
	t.Parallel()
	delivery1 := Delivery{
		BaseSHA:      "abc123",
		HeadSHA:      "def456",
		ChangedFiles: []string{"file.go"},
	}

	delivery2 := Delivery{
		BaseSHA:      "abc123",
		HeadSHA:      "def457",
		ChangedFiles: []string{"file.go"},
	}

	fp1 := fingerprintDelivery(delivery1)
	fp2 := fingerprintDelivery(delivery2)

	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintResult_Deterministic(t *testing.T) {
	t.Parallel()
	result := ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Complete",
			},
			{
				ID:        "acceptance[0]",
				Status:    PartiallySatisfied,
				Rationale: "Partial",
				Citations: []Citation{
					FileCitation("main.go", 42, 0),
				},
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	fp1 := fingerprintResult(result)
	fp2 := fingerprintResult(result)

	assert.Equal(t, fp1, fp2)
	assert.NotEmpty(t, fp1)
}

func TestFingerprintResult_Different(t *testing.T) {
	t.Parallel()
	result1 := ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    Satisfied,
				Rationale: "Complete",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	result2 := ConformanceAssessment{
		SchemaVersion: 1,
		BundleID:      "sha256:abc123",
		Results: []CriterionResult{
			{
				ID:              "definition_of_done",
				Status:          PartiallySatisfied,
				Rationale:       "Partial",
				MissingEvidence: "Not complete",
			},
		},
		ContractFingerprint: "sha256:contract123",
		DeliveryFingerprint: "sha256:delivery123",
	}

	fp1 := fingerprintResult(result1)
	fp2 := fingerprintResult(result2)

	assert.NotEqual(t, fp1, fp2)
}

func TestBundleID_Deterministic(t *testing.T) {
	t.Parallel()
	bundle := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
	}

	bundleID1 := mustComputeBundleID(t, bundle)
	bundleID2 := mustComputeBundleID(t, bundle)

	assert.Equal(t, bundleID1, bundleID2)
	assert.NotEmpty(t, bundleID1)

	assert.True(t, len(bundleID1) > 7)
}

func TestBundleID_Different(t *testing.T) {
	t.Parallel()
	bundle1 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "Definition 1",
		},
		Delivery: Delivery{
			BaseSHA: "abc123",
			HeadSHA: "def456",
		},
	}

	bundle2 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "Definition 2",
		},
		Delivery: Delivery{
			BaseSHA: "abc123",
			HeadSHA: "def456",
		},
	}

	bundleID1 := mustComputeBundleID(t, bundle1)
	bundleID2 := mustComputeBundleID(t, bundle2)

	assert.NotEqual(t, bundleID1, bundleID2)
}

func TestFingerprintFormat(t *testing.T) {
	t.Parallel()
	contract := Contract{
		DefinitionOfDone: "Test",
	}

	fp := fingerprintContract(contract)

	assert.Len(t, fp, 64)

	for _, c := range fp {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"Fingerprint contains non-hex character: %c", c)
	}
}

func TestBundleIDFormat(t *testing.T) {
	t.Parallel()
	bundle := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:   "TASK-1",
			Type: "task",
		},
		Contract: Contract{
			DefinitionOfDone: "Test",
		},
		Delivery: Delivery{
			BaseSHA: "abc",
			HeadSHA: "def",
		},
	}

	bundleID := mustComputeBundleID(t, bundle)

	assert.True(t, len(bundleID) > 7)
	assert.Contains(t, bundleID, ":")
	parts := len(bundleID) - len("sha256:")
	assert.Equal(t, 64, parts, "BundleID should have sha256:<64-hex-chars> format")
}

func TestFingerprintActivity_Deterministic_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	logContent := []byte(`2026-01-15T10:30:45Z activity: command="make build" exit_code=0 head_sha=abc123 output_hash=def456
2026-01-15T10:30:46Z activity: command="make test" exit_code=0 head_sha=abc123 output_hash=ghi789`)

	fp1 := fingerprintActivity(logContent)
	fp2 := fingerprintActivity(logContent)

	assert.Equal(t, fp1, fp2)

	assert.NotEmpty(t, fp1)

	assert.Len(t, fp1, 64)
}

func TestFingerprintActivity_Different_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	logContent1 := []byte(`2026-01-15T10:30:45Z activity: command="make build" exit_code=0 head_sha=abc123 output_hash=def456`)
	logContent2 := []byte(`2026-01-15T10:30:45Z activity: command="make build" exit_code=1 head_sha=abc123 output_hash=def456`)

	fp1 := fingerprintActivity(logContent1)
	fp2 := fingerprintActivity(logContent2)

	assert.NotEqual(t, fp1, fp2)
}

func TestFingerprintActivity_Empty_REQ_EXECEV_T2(t *testing.T) {
	t.Parallel()

	logContent := []byte("")

	fp := fingerprintActivity(logContent)

	assert.NotEmpty(t, fp)
	assert.Len(t, fp, 64)
}

func TestBundleID_LogPath_Excluded(t *testing.T) {
	t.Parallel()

	bundle1 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
		Activity: &Activity{
			Digest:            "sha256:activity1",
			EntryCount:        5,
			DeliveryHeadCount: 3,
			EarlierCount:      2,
			LogPath:           "/home/user/clone1/.armature/armature-activity.log",
		},
	}

	bundle2 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
		Activity: &Activity{
			Digest:            "sha256:activity1",
			EntryCount:        5,
			DeliveryHeadCount: 3,
			EarlierCount:      2,
			LogPath:           "/home/user/clone2/.armature/armature-activity.log",
		},
	}

	bundleID1 := mustComputeBundleID(t, bundle1)
	bundleID2 := mustComputeBundleID(t, bundle2)

	assert.Equal(t, bundleID1, bundleID2, "ComputeBundleID should exclude LogPath from hash")
}

func TestBundleID_Activity_Digest_Included(t *testing.T) {
	t.Parallel()

	bundle1 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
		Activity: &Activity{
			Digest:            "sha256:activity1",
			EntryCount:        5,
			DeliveryHeadCount: 3,
			EarlierCount:      2,
			LogPath:           "/path/to/log1",
		},
	}

	bundle2 := ReviewBundle{
		SchemaVersion: 1,
		Issue: IssueInfo{
			ID:      "TASK-1",
			Type:    "task",
			Title:   "Test Task",
			Outcome: "Completed",
		},
		Contract: Contract{
			DefinitionOfDone: "All tests pass",
			Acceptance:       []string{"Feature works"},
		},
		Delivery: Delivery{
			BaseSHA:      "abc123",
			HeadSHA:      "def456",
			ChangedFiles: []string{"main.go"},
		},
		Activity: &Activity{
			Digest:            "sha256:activity2",
			EntryCount:        5,
			DeliveryHeadCount: 3,
			EarlierCount:      2,
			LogPath:           "/path/to/log1",
		},
	}

	bundleID1 := mustComputeBundleID(t, bundle1)
	bundleID2 := mustComputeBundleID(t, bundle2)

	assert.NotEqual(t, bundleID1, bundleID2, "ComputeBundleID should include Activity.Digest in hash")
}
