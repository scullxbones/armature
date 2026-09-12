// Package contextreport prices fixture-measured structured CLI payloads
// by byte weight and the bytes/4 token estimate used by token_budget.
package contextreport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BytesPerToken is the token_budget convention: character budget = tokens * 4.
const BytesPerToken = 4

// DefaultTokenBudget matches render-context's built-in --budget default.
const DefaultTokenBudget = 4000

// FixtureShowIssue is the in-progress task priced for show, render-context, and review.
const FixtureShowIssue = "FX-WORK"

// EstimationMethod is documented in both human and JSON output so a reader
// does not have to look up how estimated_tokens was computed.
const EstimationMethod = "estimated tokens = bytes/4 (integer division), " +
	"matching the token_budget convention used by render-context " +
	"(character budget = tokens * 4). " +
	"Primary rows are json/agent stdout for list, ready, show, " +
	"render-context, and review. list, ready, and show are compact AOC " +
	"envelopes from WriteListEnvelope, WriteReadyEnvelope, and " +
	"WriteShowEnvelope. Human show still appends FormatSpend; that " +
	"adjunct is not priced here because json/agent show does not emit it. " +
	"Measured against the embedded fixture graph under " +
	"internal/contextreport/testdata/."

const (
	ClassInvocation = "invocation"
	ClassBundle     = "bundle"
)

// Artifact is one priced runtime payload.
type Artifact struct {
	Path            string `json:"path"`
	Class           string `json:"class"`
	Bytes           int    `json:"bytes"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

// Report is the runtime context-weight inventory.
type Report struct {
	EstimationMethod     string     `json:"estimation_method"`
	Artifacts            []Artifact `json:"artifacts"`
	TotalBytes           int        `json:"total_bytes"`
	TotalEstimatedTokens int        `json:"total_estimated_tokens"`
}

// EstimateTokens applies the token_budget bytes/4 heuristic.
func EstimateTokens(byteCount int) int {
	return byteCount / BytesPerToken
}

func price(path, class string, payload []byte) Artifact {
	n := len(payload)
	return Artifact{
		Path:            path,
		Class:           class,
		Bytes:           n,
		EstimatedTokens: EstimateTokens(n),
	}
}

func finalize(artifacts []Artifact) Report {
	report := Report{
		EstimationMethod: EstimationMethod,
		Artifacts:        artifacts,
	}
	for _, a := range artifacts {
		report.TotalBytes += a.Bytes
		report.TotalEstimatedTokens += a.EstimatedTokens
	}
	return report
}

// RenderHuman prints a table plus the estimation method.
func RenderHuman(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Context report (fixture-measured main-path CLI)\n")
	fmt.Fprintf(&b, "Estimation method: %s\n\n", report.EstimationMethod)
	fmt.Fprintf(&b, "%-28s %-12s %10s %10s\n", "PATH", "CLASS", "BYTES", "EST_TOKENS")
	for _, a := range report.Artifacts {
		fmt.Fprintf(&b, "%-28s %-12s %10d %10d\n", a.Path, a.Class, a.Bytes, a.EstimatedTokens)
	}
	fmt.Fprintf(&b, "%-28s %-12s %10d %10d\n", "TOTAL", "", report.TotalBytes, report.TotalEstimatedTokens)
	return b.String()
}

// RenderJSON encodes the report, including estimation_method.
func RenderJSON(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
