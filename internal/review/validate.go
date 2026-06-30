package review

import (
	"fmt"
)

// ValidateResult checks that a ConformanceAssessment is well-formed:
// - bundle ID is non-empty
// - all CriterionResults are valid (call result.Valid())
// - all Citations reference lines present in the DiffIndex
// Returns a slice of validation error strings (empty = valid).
func ValidateResult(assessment *ConformanceAssessment, idx *DiffIndex) []string {
	var errs []string

	// Check bundle ID is non-empty
	if assessment.BundleID == "" {
		errs = append(errs, "bundle ID is empty")
	}

	// Validate each criterion result
	for i, result := range assessment.Results {
		if err := result.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("criterion result %d: %v", i, err))
		}

		// Validate citations reference lines present in the diff
		for _, citation := range result.Citations {
			// If Line is omitted (0), validate that the file is in the diff
			if citation.Line == 0 {
				if !idx.ContainsFile(citation.Path) {
					errs = append(errs, fmt.Sprintf("criterion result %s: citation references %s which is not in diff", result.ID, citation.Path))
				}
			} else {
				// If Line is specified, validate the specific line
				if !idx.ContainsLine(citation.Path, citation.Line) {
					errs = append(errs, fmt.Sprintf("criterion result %s: citation references %s:%d which is not in diff", result.ID, citation.Path, citation.Line))
				}
			}
		}
	}

	return errs
}

// ValidateResultNoDiff checks a ConformanceAssessment for structural validity without
// performing diff-index citation coordinate checking. This variant is appropriate at
// record time when the diff is not available; coordinate checking is a prepare-time concern.
func ValidateResultNoDiff(assessment *ConformanceAssessment) []string {
	var errs []string

	if assessment.BundleID == "" {
		errs = append(errs, "bundle ID is empty")
	}

	for i, result := range assessment.Results {
		if err := result.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("criterion result %d: %v", i, err))
		}
	}

	return errs
}

// NewAttestation creates an AssessmentAttestation from a validated ConformanceAssessment and
// its corresponding Delivery. The delivery's BaseSHA and HeadSHA are recorded in the attestation
// so the durable record captures the exact commit range that was reviewed.
func NewAttestation(assessment *ConformanceAssessment, delivery Delivery) *AssessmentAttestation {
	// Derive rating and counts from results
	rating := DeriveRating(assessment.Results)
	satisfied, partiallySatisfied, notSatisfied, indeterminate := CountCriteria(assessment.Results)

	// Compute fingerprint of the results for idempotence detection
	resultFingerprint := FingerprintResult(*assessment)

	att := &AssessmentAttestation{
		SchemaVersion:           SchemaVersion,
		BundleID:                assessment.BundleID,
		ContractFingerprint:     assessment.ContractFingerprint,
		DeliveryFingerprint:     assessment.DeliveryFingerprint,
		BaseSHA:                 delivery.BaseSHA,
		HeadSHA:                 delivery.HeadSHA,
		Rating:                  rating,
		ResultFingerprint:       resultFingerprint,
		SatisfiedCount:          satisfied,
		PartiallySatisfiedCount: partiallySatisfied,
		NotSatisfiedCount:       notSatisfied,
		IndeterminateCount:      indeterminate,
	}

	return att
}

// IsDuplicate returns true if two attestations have the same ResultFingerprint,
// indicating they represent identical review content. This allows the same bundle
// to be re-assessed with a corrected result while remaining idempotent for identical content.
func IsDuplicate(a, b *AssessmentAttestation) bool {
	if a == nil || b == nil {
		return false
	}
	return a.ResultFingerprint == b.ResultFingerprint
}

// Applicable returns true if the attestation's BundleID matches the given bundle.
func Applicable(att *AssessmentAttestation, bundleID string) bool {
	if att == nil {
		return false
	}
	return att.BundleID == bundleID
}

// ValidateResultCoverage checks that a ConformanceAssessment covers all expected criterion IDs
// from the contract and contains no duplicates. Returns a slice of validation error strings (empty = valid).
func ValidateResultCoverage(assessment *ConformanceAssessment, contract Contract) []string {
	var errs []string

	// Build expected criterion IDs from contract
	expectedIDs := make(map[string]bool)
	if contract.DefinitionOfDone != "" {
		expectedIDs["definition_of_done"] = true
	}
	for i := range contract.Acceptance {
		expectedIDs[fmt.Sprintf("acceptance[%d]", i)] = true
	}

	// Track submitted IDs, check for duplicates, and flag unexpected IDs.
	submittedIDs := make(map[string]bool)
	for _, result := range assessment.Results {
		if submittedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf("criterion result: duplicate ID %q", result.ID))
		}
		submittedIDs[result.ID] = true

		// Flag IDs that are not in the expected set.
		if !expectedIDs[result.ID] {
			errs = append(errs, fmt.Sprintf("unexpected criterion ID %s: not in contract", result.ID))
		}
	}

	// Check for missing expected IDs
	for id := range expectedIDs {
		if !submittedIDs[id] {
			errs = append(errs, fmt.Sprintf("criterion result: missing expected ID %q", id))
		}
	}

	return errs
}
