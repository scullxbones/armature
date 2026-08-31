package review

import (
	"fmt"
	"regexp"

	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/strictjson"
)

// gitSHAPattern is the published review-bundle schema pattern for delivery SHAs.
var gitSHAPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

// DecodeReviewBundle decodes one complete ReviewBundle artifact using the
// repository-wide strict JSON policy.
func DecodeReviewBundle(data []byte) (ReviewBundle, error) {
	var bundle ReviewBundle
	if err := strictjson.Decode(data, &bundle); err != nil {
		return ReviewBundle{}, fmt.Errorf("decode review bundle: %w", err)
	}
	if err := validateDecodedBundleContract(bundle); err != nil {
		return ReviewBundle{}, fmt.Errorf("decode review bundle: %w", err)
	}
	return bundle, nil
}

// validateDecodedBundleContract enforces published bundle-schema constraints that
// ReviewBundle.Valid and encoding/json zero values do not: issue.type enum,
// nonempty issue.title, 40-hex delivery SHAs, and a present
// delivery.changed_files array (omitted or JSON null both decode as a nil slice).
func validateDecodedBundleContract(rb ReviewBundle) error {
	if rb.Issue.Title == "" {
		return fmt.Errorf("review bundle: missing issue title")
	}
	if rb.Issue.Type != "" && !issuetype.IsValid(rb.Issue.Type) {
		return fmt.Errorf("review bundle: invalid issue type %q", rb.Issue.Type)
	}
	if rb.Delivery.ChangedFiles == nil {
		return fmt.Errorf("review bundle: missing delivery.changed_files")
	}
	if !gitSHAPattern.MatchString(rb.Delivery.BaseSHA) {
		return fmt.Errorf("review bundle: malformed delivery.base_sha")
	}
	if !gitSHAPattern.MatchString(rb.Delivery.HeadSHA) {
		return fmt.Errorf("review bundle: malformed delivery.head_sha")
	}
	return nil
}

// DecodeConformanceAssessment decodes one complete reviewer assessment using
// the repository-wide strict JSON policy.
func DecodeConformanceAssessment(data []byte) (ConformanceAssessment, error) {
	var assessment ConformanceAssessment
	if err := strictjson.Decode(data, &assessment); err != nil {
		return ConformanceAssessment{}, fmt.Errorf("decode conformance assessment: %w", err)
	}
	return assessment, nil
}
