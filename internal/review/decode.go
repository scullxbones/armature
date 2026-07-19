package review

import (
	"fmt"

	"github.com/scullxbones/armature/internal/strictjson"
)

// DecodeReviewBundle decodes one complete ReviewBundle artifact using the
// repository-wide strict JSON policy.
func DecodeReviewBundle(data []byte) (ReviewBundle, error) {
	var bundle ReviewBundle
	if err := strictjson.Decode(data, &bundle); err != nil {
		return ReviewBundle{}, fmt.Errorf("decode review bundle: %w", err)
	}
	return bundle, nil
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
