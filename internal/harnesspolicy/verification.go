package harnesspolicy

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	CheckAcceptanceCriteria = "acceptance-criteria"
	CheckCitations          = "citations"
)

var verifiableKeywords = []string{
	"passes",
	"green",
	"make check",
	"go test",
	"npm test",
	"pytest",
}

type CitationCheck struct {
	SourceEntryID string
	Accepted      bool
}

type VerificationRequest struct {
	Acceptance json.RawMessage
	Citations  []CitationCheck
}

type VerificationResult struct {
	Name    string
	Passed  bool
	Message string
}

type VerificationService struct{}

func NewVerificationService() VerificationService {
	return VerificationService{}
}

func (VerificationService) Run(request VerificationRequest) []VerificationResult {
	service := VerificationService{}
	return []VerificationResult{
		service.CheckAcceptanceCriteria(request.Acceptance),
		service.CheckCitations(request.Citations),
	}
}

func (VerificationService) CheckAcceptanceCriteria(acceptance json.RawMessage) VerificationResult {
	result := VerificationResult{Name: CheckAcceptanceCriteria}

	if len(acceptance) == 0 || string(acceptance) == "null" {
		result.Message = "acceptance array is empty or absent"
		return result
	}

	var items []string
	if err := json.Unmarshal(acceptance, &items); err != nil {
		result.Message = fmt.Sprintf("acceptance array is not parseable: %v", err)
		return result
	}
	if len(items) == 0 {
		result.Message = "acceptance array is empty or absent"
		return result
	}

	for _, item := range items {
		if isVerifiable(item) {
			result.Passed = true
			result.Message = "at least one machine-verifiable acceptance criterion present"
			return result
		}
	}

	result.Message = fmt.Sprintf(
		"all %d acceptance criteria are unverifiable (human-only); add at least one machine-checkable criterion",
		len(items),
	)
	return result
}

func (VerificationService) CheckCitations(checks []CitationCheck) VerificationResult {
	result := VerificationResult{Name: CheckCitations}
	uncited := make([]string, 0)
	for _, check := range checks {
		if !check.Accepted {
			uncited = append(uncited, check.SourceEntryID)
		}
	}
	if len(uncited) == 0 {
		result.Passed = true
		result.Message = "all sources cited"
		return result
	}

	result.Message = fmt.Sprintf("uncited source(s): %s", strings.Join(uncited, ", "))
	return result
}

func isVerifiable(criterion string) bool {
	lower := strings.ToLower(criterion)
	for _, keyword := range verifiableKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
