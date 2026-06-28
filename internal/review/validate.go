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
			if !idx.ContainsLine(citation.Path, citation.Line) {
				errs = append(errs, fmt.Sprintf("criterion result %s: citation references %s:%d which is not in diff", result.ID, citation.Path, citation.Line))
			}
		}
	}

	return errs
}

// NewAttestation creates an AssessmentAttestation from a validated ConformanceAssessment.
func NewAttestation(assessment *ConformanceAssessment) *AssessmentAttestation {
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
		Rating:                  rating,
		ResultFingerprint:       resultFingerprint,
		SatisfiedCount:          satisfied,
		PartiallySatisfiedCount: partiallySatisfied,
		NotSatisfiedCount:       notSatisfied,
		IndeterminateCount:      indeterminate,
	}

	return att
}

// IsDuplicate returns true if two attestations have the same BundleID and ReviewerID.
// Since ReviewerID is not present in AssessmentAttestation, we check only BundleID.
func IsDuplicate(a, b *AssessmentAttestation) bool {
	if a == nil || b == nil {
		return false
	}
	return a.BundleID == b.BundleID
}

// Applicable returns true if the attestation's BundleID matches the given bundle.
func Applicable(att *AssessmentAttestation, bundleID string) bool {
	if att == nil {
		return false
	}
	return att.BundleID == bundleID
}
