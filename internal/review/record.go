package review

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scullxbones/armature/internal/attestation"
)

type IssueData struct {
	DefinitionOfDone string
	Scope            []string
	Acceptance       string
}

type RecordInput struct {
	Assessment *ConformanceAssessment
	Bundle     *ReviewBundle
	Issue      *IssueData
	IssueID    string
}

type RecordResult struct {
	Attestation *attestation.AssessmentAttestation
	IsDuplicate bool
}

func Record(input RecordInput) (*RecordResult, error) {
	if input.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if input.IssueID == "" {
		return nil, fmt.Errorf("issue ID is required")
	}
	if err := input.Assessment.Valid(); err != nil {
		return nil, fmt.Errorf("assessment validation failed: %w", err)
	}

	clearAssessmentActivityDetails(input.Assessment)

	if err := validateRecordBundleIntegrity(input); err != nil {
		return nil, err
	}
	if hasActivityCitations(input.Assessment) && (input.Bundle == nil || input.Bundle.Activity == nil) {
		return nil, fmt.Errorf("assessment cites activity log entries but no bundle activity section is available to validate against")
	}
	if err := joinValidationErrors("assessment validation errors:", ValidateResultNoDiff(input.Assessment)); err != nil {
		return nil, err
	}
	if err := validateRecordBundleAgainstAssessment(input); err != nil {
		return nil, err
	}
	if err := attachValidatedActivityDetails(input); err != nil {
		return nil, err
	}

	var delivery Delivery
	var activityForAttestation *Activity
	if input.Bundle != nil {
		delivery = input.Bundle.Delivery
		activityForAttestation = input.Bundle.Activity
	}
	attestation := NewAttestation(input.Assessment, delivery, activityForAttestation)

	if err := validateRecordIssueContract(input); err != nil {
		return nil, err
	}

	return &RecordResult{
		Attestation: attestation,
		IsDuplicate: false,
	}, nil
}

func joinValidationErrors(prefix string, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	for _, e := range errs {
		sb.WriteString("\n  - ")
		sb.WriteString(e)
	}
	return fmt.Errorf("%s", sb.String())
}

func clearAssessmentActivityDetails(assessment *ConformanceAssessment) {
	for i := range assessment.Results {
		for j := range assessment.Results[i].Citations {
			assessment.Results[i].Citations[j].ClearActivityEntryDetails()
		}
	}
}

func validateRecordBundleIntegrity(input RecordInput) error {
	if input.Bundle == nil {
		return nil
	}
	recomputed, err := ComputeBundleID(*input.Bundle)
	if err != nil {
		return err
	}
	if recomputed != input.Bundle.BundleID {
		return fmt.Errorf(
			"bundle integrity check failed: recomputed bundle_id %s does not match bundle's "+
				"recorded bundle_id %s (bundle contents may have been altered since `arm review prepare` ran)",
			recomputed, input.Bundle.BundleID)
	}
	if err := ValidateGateEvidenceLogs(input.Bundle.GateEvidence); err != nil {
		return fmt.Errorf("gate evidence validation failed: %w", err)
	}
	return nil
}

func validateRecordBundleAgainstAssessment(input RecordInput) error {
	if input.Bundle == nil {
		return nil
	}
	if input.Bundle.Issue.ID != input.IssueID {
		return fmt.Errorf("bundle was prepared for issue %s, not %s",
			input.Bundle.Issue.ID, input.IssueID)
	}
	if input.Assessment.BundleID != input.Bundle.BundleID {
		return fmt.Errorf("assessment bundle_id %s does not match bundle bundle_id %s",
			input.Assessment.BundleID, input.Bundle.BundleID)
	}
	if input.Assessment.DeliveryFingerprint != input.Bundle.Fingerprints.Delivery {
		return fmt.Errorf("assessment delivery_fingerprint %s does not match bundle delivery_fingerprint %s",
			input.Assessment.DeliveryFingerprint, input.Bundle.Fingerprints.Delivery)
	}
	if input.Assessment.ContractFingerprint != input.Bundle.Fingerprints.Contract {
		return fmt.Errorf("assessment contract_fingerprint %s does not match bundle contract_fingerprint %s",
			input.Assessment.ContractFingerprint, input.Bundle.Fingerprints.Contract)
	}
	idx, err := BuildDiffIndex(input.Bundle.Delivery.Diff)
	if err != nil {
		return fmt.Errorf("build diff index: %w", err)
	}
	return joinValidationErrors("assessment citation validation errors:", ValidateResult(input.Assessment, idx))
}

func attachValidatedActivityDetails(input RecordInput) error {
	if input.Bundle == nil || input.Bundle.Activity == nil {
		return nil
	}
	activityEntryMap, digestErrs := ValidateActivityDigestAndLoadEntries(input.Bundle.Activity)
	if err := joinValidationErrors("activity log validation errors:", digestErrs); err != nil {
		return err
	}
	citationErrs := ValidateActivityCitations(input.Assessment, input.Bundle.Activity, activityEntryMap, input.Bundle.Delivery.HeadSHA)
	if err := joinValidationErrors("activity citation validation errors:", citationErrs); err != nil {
		return err
	}
	for i := range input.Assessment.Results {
		for j := range input.Assessment.Results[i].Citations {
			citation := &input.Assessment.Results[i].Citations[j]
			if citation.ActivityEntryID() == "" {
				continue
			}
			entryID, err := strconv.Atoi(citation.ActivityEntryID())
			if err != nil {
				continue
			}
			if details, ok := activityEntryMap[entryID]; ok {
				citation.SetActivityEntryDetails(FormatActivityEntryDetails(details))
			}
		}
	}
	return nil
}

func validateRecordIssueContract(input RecordInput) error {
	if input.Issue == nil {
		return nil
	}
	criteria, err := ParseAcceptanceCriteria([]byte(input.Issue.Acceptance))
	if err != nil {
		return fmt.Errorf("failed to parse acceptance criteria: %w", err)
	}
	contract := Contract{
		DefinitionOfDone: input.Issue.DefinitionOfDone,
		Scope:            input.Issue.Scope,
		Acceptance:       criteria,
	}
	issueContractFP := FingerprintContract(contract)
	if input.Assessment.ContractFingerprint != issueContractFP {
		return fmt.Errorf("assessment contract fingerprint %s does not match issue contract fingerprint %s",
			input.Assessment.ContractFingerprint, issueContractFP)
	}
	return joinValidationErrors("assessment coverage validation errors:", ValidateResultCoverage(input.Assessment, contract))
}

func RecordWithDuplicateCheck(input RecordInput, existingAttestations []attestation.AssessmentAttestation) (*RecordResult, error) {
	result, err := Record(input)
	if err != nil {
		return nil, err
	}

	for _, existingAtt := range existingAttestations {
		if IsDuplicate(result.Attestation, &existingAtt) {
			result.IsDuplicate = true
			return result, nil
		}
	}

	enrichDisagreementFields(result.Attestation, existingAttestations)
	return result, nil
}

func enrichDisagreementFields(att *attestation.AssessmentAttestation, existing []attestation.AssessmentAttestation) {
	if att == nil {
		return
	}
	att.EffectiveRating = att.Rating

	var citedBundle string
	var citedRating attestation.Rating
	haveCite := false

	for _, prior := range existing {
		if prior.DeliveryFingerprint != att.DeliveryFingerprint {
			continue
		}
		if prior.ResultFingerprint == att.ResultFingerprint {
			continue
		}
		if max, ok := attestation.MaxRating(att.EffectiveRating, prior.Rating); ok {
			att.EffectiveRating = max
		}
		if prior.Rating == att.Rating {
			continue
		}
		att.IsDisagreement = true
		if !haveCite || attestation.AtLeastAsSevere(prior.Rating, citedRating) {
			haveCite = true
			citedBundle = prior.BundleID
			citedRating = prior.Rating
		}
	}

	if att.IsDisagreement && haveCite {
		att.ConflictsWithBundleID = citedBundle
		att.ConflictsWithRating = ratingPointer(citedRating)
	}
}

func ratingPointer(r attestation.Rating) *attestation.Rating {
	return &r
}
