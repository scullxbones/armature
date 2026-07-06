package review

import (
	"fmt"
	"strconv"
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

	// Validate structural correctness without diff-index citation checking.
	// When --bundle is provided, full diff-index citation validation is performed below.
	if errs := ValidateResultNoDiff(input.Assessment); len(errs) > 0 {
		msg := "assessment validation errors:"
		for _, e := range errs {
			msg += "\n  - " + e
		}
		return nil, fmt.Errorf("%s", msg)
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
			msg := "assessment citation validation errors:"
			for _, e := range errs {
				msg += "\n  - " + e
			}
			return nil, fmt.Errorf("%s", msg)
		}
	}

	// When the bundle has activity evidence, validate activity citations and populate entry details.
	// This ensures activity citations reference valid entry IDs and obey the upgrade-only rule.
	var activityEntryMap map[int]ActivityEntryDetails
	if input.Bundle != nil && input.Bundle.Activity != nil {
		// Build contract for activity citation validation
		contract := Contract{}
		if input.Issue != nil {
			criteria, err := ParseAcceptanceCriteria([]byte(input.Issue.Acceptance))
			if err != nil {
				return nil, fmt.Errorf("failed to parse acceptance criteria for activity validation: %w", err)
			}
			contract = Contract{
				DefinitionOfDone: input.Issue.DefinitionOfDone,
				Scope:            input.Issue.Scope,
				Acceptance:       criteria,
			}
		}

		if errs := ValidateActivityCitations(input.Assessment, input.Bundle.Activity, contract); len(errs) > 0 {
			msg := "activity citation validation errors:"
			for _, e := range errs {
				msg += "\n  - " + e
			}
			return nil, fmt.Errorf("%s", msg)
		}

		// Load activity entries for populating entry details
		activityEntryMap = LoadActivityEntries(input.Bundle.Activity.LogPath)

		// Populate activity entry details in citations
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
	if input.Bundle != nil {
		delivery = input.Bundle.Delivery
	}
	attestation := NewAttestation(input.Assessment, delivery)

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
			msg := "assessment coverage validation errors:"
			for _, e := range errs {
				msg += "\n  - " + e
			}
			return nil, fmt.Errorf("%s", msg)
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
