package review

import (
	"fmt"
	"strconv"
	"strings"
)

// IssueData holds the minimal issue information needed for recording.
type IssueData struct {
	DefinitionOfDone string
	Scope            []string
	Acceptance       string // JSON-encoded acceptance criteria
}

// RecordInput bundles the inputs for a conformance assessment recording.
type RecordInput struct {
	// Assessment is the parsed conformance assessment from the reviewer.
	Assessment *ConformanceAssessment
	// Bundle is optional; when provided, enables fingerprint and diff-index citation validation.
	Bundle *ReviewBundle
	// Issue contains the issue metadata needed for contract validation.
	Issue *IssueData
	// IssueID is the target issue identifier.
	IssueID string
}

// RecordResult is the successful outcome of recording a conformance assessment.
type RecordResult struct {
	// Attestation is the compact durable record of the review.
	Attestation *AssessmentAttestation
	// IsDuplicate indicates whether this assessment duplicates an existing one.
	IsDuplicate bool
}

// Record performs the complete conformance assessment recording workflow:
// - Validates the assessment structure
// - Verifies fingerprints against the bundle (if provided)
// - Validates coverage against the issue contract
// - Creates the durable attestation
//
// Returns RecordResult on success, or an error describing any validation failure.
// The error messages are human-readable and suitable for direct output to the user.
func Record(input RecordInput) (*RecordResult, error) {
	if input.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if input.IssueID == "" {
		return nil, fmt.Errorf("issue ID is required")
	}

	// Validate assessment structure (JSON schema and field validity).
	if err := input.Assessment.Valid(); err != nil {
		return nil, fmt.Errorf("assessment validation failed: %w", err)
	}

	// Reset any reviewer-supplied ActivityEntryDetails unconditionally, before any
	// other validation runs. This field is only ever legitimately populated from the
	// activity log below; an inbound value is either stale (from a prior record) or
	// adversarial (model-authored prose dressed as harness-verified execution facts)
	// and must never survive to the published report.
	for i := range input.Assessment.Results {
		for j := range input.Assessment.Results[i].Citations {
			input.Assessment.Results[i].Citations[j].ActivityEntryDetails = ""
		}
	}

	// Recompute the bundle's identity from its actual on-disk contents and reject
	// if it no longer matches the bundle's own recorded BundleID. Every other check
	// below only compares fields *within* the same bundle (e.g. assessment.BundleID
	// == bundle.BundleID), so a hand-edited bundle file with internally-consistent
	// but altered fields (e.g. Delivery.HeadSHA blanked to dodge the HeadSHA-citation
	// gate, or Activity.Digest recomputed to match a self-authored log) would pass
	// every one of them. This check anchors the bundle to the identity Prepare
	// actually computed for it.
	if input.Bundle != nil {
		if recomputed := ComputeBundleID(*input.Bundle); recomputed != input.Bundle.BundleID {
			return nil, fmt.Errorf(
				"bundle integrity check failed: recomputed bundle_id %s does not match bundle's "+
					"recorded bundle_id %s (bundle contents may have been altered since `arm review prepare` ran)",
				recomputed, input.Bundle.BundleID)
		}
		if err := ValidateGateEvidenceLogs(input.Bundle.GateEvidence); err != nil {
			return nil, fmt.Errorf("gate evidence validation failed: %w", err)
		}
	}

	// Activity citations are only meaningful when validated against a bundle's
	// Activity section; without one there is nothing to check a claimed entry ID
	// against, so an assessment citing activity evidence must be rejected outright
	// rather than silently accepted with zero validation.
	if hasActivityCitations(input.Assessment) && (input.Bundle == nil || input.Bundle.Activity == nil) {
		return nil, fmt.Errorf("assessment cites activity log entries but no bundle activity section is available to validate against")
	}

	// Validate structural correctness without diff-index citation checking.
	// When --bundle is provided, full diff-index citation validation is performed below.
	if errs := ValidateResultNoDiff(input.Assessment); len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString("assessment validation errors:")
		for _, e := range errs {
			sb.WriteString("\n  - ")
			sb.WriteString(e)
		}
		return nil, fmt.Errorf("%s", sb.String())
	}

	// When the bundle is available, verify that the bundle was prepared for the correct issue.
	// This prevents assessment results from one issue (e.g., TASK-A) from being recorded onto
	// another issue (e.g., TASK-B) even if they share the same contract fingerprint.
	if input.Bundle != nil {
		if input.Bundle.Issue.ID != input.IssueID {
			return nil, fmt.Errorf("bundle was prepared for issue %s, not %s",
				input.Bundle.Issue.ID, input.IssueID)
		}
	}

	// When the bundle is available, verify that the assessment's bundle_id and fingerprints
	// match the loaded bundle. This ensures the assessment was produced for this specific delivery
	// and prevents mismatched assessments from being recorded.
	if input.Bundle != nil {
		if input.Assessment.BundleID != input.Bundle.BundleID {
			return nil, fmt.Errorf("assessment bundle_id %s does not match bundle bundle_id %s",
				input.Assessment.BundleID, input.Bundle.BundleID)
		}
		if input.Assessment.DeliveryFingerprint != input.Bundle.Fingerprints.Delivery {
			return nil, fmt.Errorf("assessment delivery_fingerprint %s does not match bundle delivery_fingerprint %s",
				input.Assessment.DeliveryFingerprint, input.Bundle.Fingerprints.Delivery)
		}
		if input.Assessment.ContractFingerprint != input.Bundle.Fingerprints.Contract {
			return nil, fmt.Errorf("assessment contract_fingerprint %s does not match bundle contract_fingerprint %s",
				input.Assessment.ContractFingerprint, input.Bundle.Fingerprints.Contract)
		}
	}

	// When the bundle is available, perform diff-index citation coordinate validation.
	// This ensures every citation references a line that actually appears in the delivery diff.
	if input.Bundle != nil {
		idx, err := BuildDiffIndex(input.Bundle.Delivery.Diff)
		if err != nil {
			return nil, fmt.Errorf("build diff index: %w", err)
		}
		if errs := ValidateResult(input.Assessment, idx); len(errs) > 0 {
			var sb strings.Builder
			sb.WriteString("assessment citation validation errors:")
			for _, e := range errs {
				sb.WriteString("\n  - ")
				sb.WriteString(e)
			}
			return nil, fmt.Errorf("%s", sb.String())
		}
	}

	// When the bundle has activity evidence, validate activity citations and populate entry details.
	// This ensures activity citations reference valid entry IDs and obey the upgrade-only rule.
	if input.Bundle != nil && input.Bundle.Activity != nil {
		// Load activity entries and validate digest in a single file read (PR #71 fix).
		// This avoids the TOCTOU race where ValidateActivityDigest and LoadActivityEntries
		// each did independent reads, allowing the file to change between them.
		activityEntryMap, digestErrs := ValidateActivityDigestAndLoadEntries(input.Bundle.Activity)

		// Always enforce the digest check, regardless of whether this assessment
		// cites activity entries: NewAttestation below stamps activity.Digest into
		// the durable attestation unconditionally, so if the on-disk log no longer
		// matches that digest, recording must fail rather than attest to a digest
		// that was never actually re-verified against disk (PR #71 finding #4).
		if len(digestErrs) > 0 {
			var sb strings.Builder
			sb.WriteString("activity log validation errors:")
			for _, e := range digestErrs {
				sb.WriteString("\n  - ")
				sb.WriteString(e)
			}
			return nil, fmt.Errorf("%s", sb.String())
		}

		if errs := ValidateActivityCitations(input.Assessment, input.Bundle.Activity, activityEntryMap, input.Bundle.Delivery.HeadSHA); len(errs) > 0 {
			var sb strings.Builder
			sb.WriteString("activity citation validation errors:")
			for _, e := range errs {
				sb.WriteString("\n  - ")
				sb.WriteString(e)
			}
			return nil, fmt.Errorf("%s", sb.String())
		}

		// Populate activity entry details in citations from the log only (the
		// inbound value was already reset to "" above).
		for i := range input.Assessment.Results {
			for j := range input.Assessment.Results[i].Citations {
				citation := &input.Assessment.Results[i].Citations[j]
				if citation.ActivityEntryID != "" {
					// Convert entry ID string to int for lookup
					entryID, err := strconv.Atoi(citation.ActivityEntryID)
					if err == nil {
						if details, ok := activityEntryMap[entryID]; ok {
							citation.ActivityEntryDetails = FormatActivityEntryDetails(details)
						}
					}
				}
			}
		}
	}

	// Create attestation, populating BaseSHA/HeadSHA from the bundle delivery when available.
	var delivery Delivery
	var activityForAttestation *Activity
	if input.Bundle != nil {
		delivery = input.Bundle.Delivery
		activityForAttestation = input.Bundle.Activity
	}
	attestation := NewAttestation(input.Assessment, delivery, activityForAttestation)

	// If issue data is provided, verify the assessment's contract fingerprint and coverage.
	if input.Issue != nil {
		// Build contract from issue for coverage validation and fingerprint check.
		criteria, err := ParseAcceptanceCriteria([]byte(input.Issue.Acceptance))
		if err != nil {
			return nil, fmt.Errorf("failed to parse acceptance criteria: %w", err)
		}
		contract := Contract{
			DefinitionOfDone: input.Issue.DefinitionOfDone,
			Scope:            input.Issue.Scope,
			Acceptance:       criteria,
		}

		// Verify the assessment's contract fingerprint matches the issue's contract.
		// A mismatch indicates the assessment was produced against a different (possibly stale
		// or fabricated) contract and must be rejected.
		issueContractFP := FingerprintContract(contract)
		if input.Assessment.ContractFingerprint != issueContractFP {
			return nil, fmt.Errorf("assessment contract fingerprint %s does not match issue contract fingerprint %s",
				input.Assessment.ContractFingerprint, issueContractFP)
		}

		// Validate that assessment covers all expected criteria
		if errs := ValidateResultCoverage(input.Assessment, contract); len(errs) > 0 {
			var sb strings.Builder
			sb.WriteString("assessment coverage validation errors:")
			for _, e := range errs {
				sb.WriteString("\n  - ")
				sb.WriteString(e)
			}
			return nil, fmt.Errorf("%s", sb.String())
		}
	}

	return &RecordResult{
		Attestation: attestation,
		IsDuplicate: false,
	}, nil
}

// RecordWithDuplicateCheck extends Record with duplicate attestation checking.
// It checks whether the given attestation already exists in the issue's assessment history
// and returns a RecordResult indicating idempotence.
//
// This is a helper for command-line usage where we need to distinguish idempotent duplicate
// calls from genuine new recordings.
func RecordWithDuplicateCheck(input RecordInput, existingAttestations []AssessmentAttestation) (*RecordResult, error) {
	result, err := Record(input)
	if err != nil {
		return nil, err
	}

	// Check for duplicate attestations
	for _, existingAtt := range existingAttestations {
		if IsDuplicate(result.Attestation, &existingAtt) {
			// Idempotent: duplicate is acceptable
			result.IsDuplicate = true
			return result, nil
		}
	}

	return result, nil
}
