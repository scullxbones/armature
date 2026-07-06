package review

import (
	"fmt"
	"os"
	"strconv"
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
			// Activity citations reference the activity log, not the diff; they are validated
			// separately by ValidateActivityCitations/ValidateActivityDigest and have no Path.
			if citation.ActivityEntryID != "" {
				continue
			}

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
// activity is optional (nil when the bundle had no Activity section); when present, its digest
// is carried into the attestation per ADR-0008 ("the digest enters the attestation").
func NewAttestation(assessment *ConformanceAssessment, delivery Delivery, activity *Activity) *AssessmentAttestation {
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

	if activity != nil {
		att.ActivityDigest = activity.Digest
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

// hasActivityCitations reports whether any citation in the assessment references an activity
// log entry (as opposed to a diff line).
func hasActivityCitations(assessment *ConformanceAssessment) bool {
	for _, result := range assessment.Results {
		for _, citation := range result.Citations {
			if citation.ActivityEntryID != "" {
				return true
			}
		}
	}
	return false
}

// ValidateActivityDigest verifies that the activity log's current on-disk content matches the
// digest recorded in the bundle's Activity section. This guards against a tampered or rotated
// activity log being accepted at record time: the digest recorded during `arm review prepare`
// must still match what is on disk when `arm review record` runs.
//
// If activity is nil, there is nothing to validate and no errors are returned. If the log file
// is missing or unreadable, or its recomputed digest does not match activity.Digest, a
// validation error is returned.
func ValidateActivityDigest(activity *Activity) []string {
	var errs []string

	if activity == nil {
		return errs
	}

	content, err := os.ReadFile(activity.LogPath)
	if err != nil {
		errs = append(errs, fmt.Sprintf(
			"activity log missing or unreadable at %q: %v (log must be present and unmodified since prepare)",
			activity.LogPath, err))
		return errs
	}

	actualDigest := FingerprintActivity(content)
	if actualDigest != activity.Digest {
		errs = append(errs, fmt.Sprintf(
			"activity log digest mismatch: bundle recorded %s but log at %q now has digest %s "+
				"(the log changed since `arm review prepare` ran — this may be a new entry appended by "+
				"further worktree activity, a stale/rotated log, or a tampered file; re-run prepare "+
				"against the current log before recording)",
			activity.Digest, activity.LogPath, actualDigest))
	}

	return errs
}

// ValidateActivityCitations validates activity citations in a ConformanceAssessment against
// parsed activity log entries. It checks:
// - All cited entry IDs exist in the log
// - No activity entry with an unknown exit code can support a Satisfied criterion status
// - Activity-citations-only cannot support Satisfied or PartiallySatisfied on implementation criteria
// - All cited entries match the delivery's HeadSHA (reject entries from earlier commits)
// Returns a slice of validation error strings (empty = valid).
// deliveryHeadSHA is the expected commit SHA for the delivery; entries whose HeadSHA does
// not match this value are rejected (they represent evidence from earlier commits).
func ValidateActivityCitations(assessment *ConformanceAssessment, activity *Activity, entries map[int]ActivityEntryDetails, deliveryHeadSHA string) []string {
	var errs []string

	if activity == nil {
		// No activity section to validate against
		return errs
	}

	// Track which criteria have activity citations only
	activityOnlyByID := make(map[string]bool)

	// Validate each activity citation in the assessment
	for _, result := range assessment.Results {
		hasActivityCitation := false
		hasDiffCitation := false

		for _, citation := range result.Citations {
			if citation.ActivityEntryID != "" {
				hasActivityCitation = true

				// Reject citations referencing the "activity index" (use of "index" terminology)
				// Index would be something like "index:0" or "activity_index"
				// We only accept raw entry IDs which are numeric strings
				entryID, err := strconv.Atoi(citation.ActivityEntryID)
				if err != nil {
					errs = append(errs, fmt.Sprintf("criterion result %s: invalid activity entry ID %q (must be numeric)", result.ID, citation.ActivityEntryID))
					continue
				}

				entry, ok := entries[entryID]
				if !ok {
					errs = append(errs, fmt.Sprintf("criterion result %s: unknown activity entry ID %d (not present in the activity log)",
						result.ID, entryID))
					continue
				}

				supportsPositiveStatus := result.Status == Satisfied || result.Status == PartiallySatisfied

				// Reject entries from earlier commits when they are being used to support a
				// Satisfied/PartiallySatisfied status: the cited entry must have been executed
				// at the delivery's HEAD commit, not at an earlier commit, to serve as evidence
				// that the current delivery passes. Citing an earlier-commit entry to support a
				// NotSatisfied status ("this was already broken before this commit too") is a
				// legitimate use and is not blocked here.
				if supportsPositiveStatus && deliveryHeadSHA != "" && entry.HeadSHA != deliveryHeadSHA {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d was executed at head_sha=%q but delivery head_sha=%q; "+
							"entries from earlier commits cannot be used as evidence for the current delivery",
						result.ID, entryID, entry.HeadSHA, deliveryHeadSHA))
					continue
				}

				// An entry with no recorded exit code (harness omitted it) cannot be used
				// as evidence that a criterion is fully satisfied — "unknown" and "succeeded"
				// must remain distinguishable outcomes for verified behavioral evidence.
				if !entry.ExitCodeKnown && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has an unknown exit code and cannot support satisfied status",
						result.ID, entryID))
				}

				// An entry with a known but nonzero exit code represents a failed command
				// execution and cannot be used as evidence that a criterion passed.
				if entry.ExitCodeKnown && entry.ExitCode != 0 && result.Status == Satisfied {
					errs = append(errs, fmt.Sprintf(
						"criterion result %s: activity entry %d has a failed exit code (%d) and cannot support satisfied status",
						result.ID, entryID, entry.ExitCode))
				}
			}

			if citation.Path != "" {
				hasDiffCitation = true
			}
		}

		// Track if this criterion has only activity citations
		if hasActivityCitation && !hasDiffCitation {
			activityOnlyByID[result.ID] = true
		}
	}

	// Check upgrade-only rule: activity-citations-only cannot satisfy or partially
	// satisfy implementation criteria (ADR-0008 rule 1).
	for criterionID := range activityOnlyByID {
		// Find the corresponding result
		var result *CriterionResult
		for i := range assessment.Results {
			if assessment.Results[i].ID == criterionID {
				result = &assessment.Results[i]
				break
			}
		}

		if result == nil {
			continue
		}

		// Check if this is an implementation criterion
		// Implementation criteria are: definition_of_done
		// (Acceptance criteria and custom criteria are behavioral and can be satisfied by activity evidence)
		isImplementationCriterion := (criterionID == "definition_of_done")

		// If activity-citations-only is used on an implementation criterion with a
		// Satisfied or PartiallySatisfied status, reject it.
		if isImplementationCriterion && (result.Status == Satisfied || result.Status == PartiallySatisfied) {
			msg := fmt.Sprintf(
				"criterion result %s: activity citations alone cannot support %s on implementation criterion (upgrade-only rule)",
				result.ID, result.Status,
			)
			errs = append(errs, msg)
		}
	}

	return errs
}
