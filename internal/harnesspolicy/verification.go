package harnesspolicy

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	acceptanceCriteriaCheck = "acceptance-criteria"
	citationsCheck          = "citations"
)

func verifiableKeywords() []string {
	return []string{
		"passes",
		"green",
		"make check",
		"go test",
		"npm test",
		"pytest",
	}
}

type CitationCheck struct {
	SourceEntryID string
	Accepted      bool
}

// verificationResult is one acceptance or citation check. Callers read
// Passed and Message; the check name stays inside the package.
type verificationResult struct {
	name    string
	Passed  bool
	Message string
}

func (p IssuePolicy) Verify() []verificationResult {
	return []verificationResult{
		checkAcceptanceCriteria(p.Acceptance),
		checkCitations(p.Citations),
	}
}

func checkAcceptanceCriteria(acceptance json.RawMessage) verificationResult {
	result := verificationResult{name: acceptanceCriteriaCheck}

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

func checkCitations(checks []CitationCheck) verificationResult {
	result := verificationResult{name: citationsCheck}
	uncited := make([]string, 0)
	for _, check := range checks {
		if !check.Accepted {
			uncited = append(uncited, check.SourceEntryID)
		}
	}
	if len(uncited) == 0 {
		result.Passed = true
		result.Message = "all Citation Acceptances recorded"
		return result
	}

	result.Message = fmt.Sprintf(
		"missing Citation Acceptance for %s; run arm sources accept-citation",
		strings.Join(uncited, ", "),
	)
	return result
}

func isVerifiable(criterion string) bool {
	lower := strings.ToLower(criterion)
	for _, keyword := range verifiableKeywords() {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
