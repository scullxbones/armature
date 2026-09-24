package review

import (
	"fmt"
	"strconv"
	"strings"
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
	Attestation *AssessmentAttestation
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

	for i := range input.Assessment.Results {
		for j := range input.Assessment.Results[i].Citations {
			input.Assessment.Results[i].Citations[j].ActivityEntryDetails = ""
		}
	}

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

	if hasActivityCitations(input.Assessment) && (input.Bundle == nil || input.Bundle.Activity == nil) {
		return nil, fmt.Errorf("assessment cites activity log entries but no bundle activity section is available to validate against")
	}

	if errs := ValidateResultNoDiff(input.Assessment); len(errs) > 0 {
		var sb strings.Builder
		sb.WriteString("assessment validation errors:")
		for _, e := range errs {
			sb.WriteString("\n  - ")
			sb.WriteString(e)
		}
		return nil, fmt.Errorf("%s", sb.String())
	}

	if input.Bundle != nil {
		if input.Bundle.Issue.ID != input.IssueID {
			return nil, fmt.Errorf("bundle was prepared for issue %s, not %s",
				input.Bundle.Issue.ID, input.IssueID)
		}
	}

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

	if input.Bundle != nil && input.Bundle.Activity != nil {
		activityEntryMap, digestErrs := ValidateActivityDigestAndLoadEntries(input.Bundle.Activity)

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

		for i := range input.Assessment.Results {
			for j := range input.Assessment.Results[i].Citations {
				citation := &input.Assessment.Results[i].Citations[j]
				if citation.ActivityEntryID != "" {
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

	var delivery Delivery
	var activityForAttestation *Activity
	if input.Bundle != nil {
		delivery = input.Bundle.Delivery
		activityForAttestation = input.Bundle.Activity
	}
	attestation := NewAttestation(input.Assessment, delivery, activityForAttestation)

	if input.Issue != nil {
		criteria, err := ParseAcceptanceCriteria([]byte(input.Issue.Acceptance))
		if err != nil {
			return nil, fmt.Errorf("failed to parse acceptance criteria: %w", err)
		}
		contract := Contract{
			DefinitionOfDone: input.Issue.DefinitionOfDone,
			Scope:            input.Issue.Scope,
			Acceptance:       criteria,
		}

		issueContractFP := FingerprintContract(contract)
		if input.Assessment.ContractFingerprint != issueContractFP {
			return nil, fmt.Errorf("assessment contract fingerprint %s does not match issue contract fingerprint %s",
				input.Assessment.ContractFingerprint, issueContractFP)
		}

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

func RecordWithDuplicateCheck(input RecordInput, existingAttestations []AssessmentAttestation) (*RecordResult, error) {
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

func enrichDisagreementFields(att *AssessmentAttestation, existing []AssessmentAttestation) {
	if att == nil {
		return
	}
	att.EffectiveRating = att.Rating

	var citedBundle string
	var citedRating Rating
	haveCite := false

	for _, prior := range existing {
		if prior.DeliveryFingerprint != att.DeliveryFingerprint {
			continue
		}
		if prior.ResultFingerprint == att.ResultFingerprint {
			continue
		}
		att.EffectiveRating = MaxRating(att.EffectiveRating, prior.Rating)
		if prior.Rating == att.Rating {
			continue
		}
		att.IsDisagreement = true
		if !haveCite || ratingSeverity(prior.Rating) >= ratingSeverity(citedRating) {
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

func ratingPointer(r Rating) *Rating {
	return &r
}
