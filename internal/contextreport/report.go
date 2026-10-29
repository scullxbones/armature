// Package contextreport prices fixture-measured structured CLI payloads
// by byte weight and the bytes/4 token estimate used by token_budget.
package contextreport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// bytesPerToken is the token_budget convention: character budget = tokens * 4.
const bytesPerToken = 4

// defaultTokenBudget matches render-context's built-in --budget default.
const defaultTokenBudget = 4000

const fixtureShowIssue = "FX-WORK"

const estimationMethod = "estimated tokens = bytes/4 (integer division), " +
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
	classInvocation = "invocation"
	classBundle     = "bundle"
)

type artifact struct {
	Path            string `json:"path"`
	Class           string `json:"class"`
	Bytes           int    `json:"bytes"`
	EstimatedTokens int    `json:"estimated_tokens"`
}

type Report struct {
	EstimationMethod     string     `json:"estimation_method"`
	Artifacts            []artifact `json:"artifacts"`
	TotalBytes           int        `json:"total_bytes"`
	TotalEstimatedTokens int        `json:"total_estimated_tokens"`
}

func estimateTokens(byteCount int) int {
	return byteCount / bytesPerToken
}

func price(path, class string, payload []byte) artifact {
	n := len(payload)
	return artifact{
		Path:            path,
		Class:           class,
		Bytes:           n,
		EstimatedTokens: estimateTokens(n),
	}
}

func finalize(artifacts []artifact) Report {
	report := Report{
		EstimationMethod: estimationMethod,
		Artifacts:        artifacts,
	}
	for _, a := range artifacts {
		report.TotalBytes += a.Bytes
		report.TotalEstimatedTokens += a.EstimatedTokens
	}
	return report
}

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

func RenderJSON(report Report) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}
