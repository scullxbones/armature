package promotion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

// ReadyIssue is the borrowed contract + assessment facts CheckReady needs.
type ReadyIssue struct {
	DefinitionOfDone string
	Acceptance       json.RawMessage
	Assessments      []AssessmentFact
}

// ReadyInput is the verify-only promotion-ready check (ADR 0026 N1 poka-yoke).
// It never authors assessments or runs a model.
type ReadyInput struct {
	BaseSHA      string
	HeadSHA      string // current delivery tip; stale assessments must not pass
	IssueIDs     []string
	Issues       map[string]ReadyIssue
	GateEvidence []ops.GateEvidence
	Stops        []ActiveStop // armed jidoka stops (ADR 0026)
	Graph        Graph        // issue tree for stop coverage; optional
}

// CheckReady verifies R2/R3/R4 for the requested scopes at HeadSHA.
func CheckReady(in ReadyInput) error {
	if in.HeadSHA == "" {
		return fmt.Errorf("promotion-ready: head SHA is required")
	}
	if in.BaseSHA == "" {
		return fmt.Errorf("promotion-ready: base SHA is required")
	}
	if len(in.IssueIDs) == 0 {
		return fmt.Errorf("promotion-ready: at least one issue scope is required")
	}
	if err := checkJidokaStops(in); err != nil {
		return err
	}
	if !hasCIFullGateEvidence(in.GateEvidence, in.HeadSHA) {
		return fmt.Errorf("promotion-ready: missing CI gate evidence for head %s (profile=full)", in.HeadSHA)
	}
	for _, id := range in.IssueIDs {
		issue, ok := in.Issues[id]
		if !ok {
			return fmt.Errorf("promotion-ready: unknown scope %q", id)
		}
		if emptyContract(issue.DefinitionOfDone, issue.Acceptance) {
			return fmt.Errorf("promotion-ready: empty contract on %s", id)
		}
	}
	if !hasDeliveryAssessment(in.Issues, in.IssueIDs, in.BaseSHA, in.HeadSHA) {
		return fmt.Errorf("promotion-ready: no valid assessment for delivery (%s, %s)", in.BaseSHA, in.HeadSHA)
	}
	return nil
}

func hasCIFullGateEvidence(evs []ops.GateEvidence, headSHA string) bool {
	for _, ev := range evs {
		if ev.PromotionReadyEvidence(headSHA) {
			return true
		}
	}
	return false
}

func hasDeliveryAssessment(issues map[string]ReadyIssue, ids []string, base, tip string) bool {
	for _, id := range ids {
		issue, ok := issues[id]
		if !ok {
			continue
		}
		for _, att := range issue.Assessments {
			if att.BaseSHA == base && att.HeadSHA == tip {
				return true
			}
		}
	}
	return false
}

func emptyContract(dod string, acceptance json.RawMessage) bool {
	if strings.TrimSpace(dod) == "" {
		return true
	}
	trimmed := bytes.TrimSpace(acceptance)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return true
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(trimmed, &arr); err == nil && len(arr) == 0 {
		return true
	}
	return false
}
