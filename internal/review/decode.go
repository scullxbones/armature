package review

import (
	"fmt"

	"github.com/scullxbones/armature/internal/gitoid"
	"github.com/scullxbones/armature/internal/issuetype"
	"github.com/scullxbones/armature/internal/strictjson"
)

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
	if !gitoid.Valid(rb.Delivery.BaseSHA) {
		return fmt.Errorf("review bundle: malformed delivery.base_sha")
	}
	if !gitoid.Valid(rb.Delivery.HeadSHA) {
		return fmt.Errorf("review bundle: malformed delivery.head_sha")
	}
	return nil
}

func DecodeConformanceAssessment(data []byte) (ConformanceAssessment, error) {
	var assessment ConformanceAssessment
	if err := strictjson.Decode(data, &assessment); err != nil {
		return ConformanceAssessment{}, fmt.Errorf("decode conformance assessment: %w", err)
	}
	return assessment, nil
}
