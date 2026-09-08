// Package traceability validates that DAG issues are properly cited in git history.
package traceability

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/adapters"
)

// Confidence values on IssueRef. Empty Confidence is treated as Verified
// (legacy default). Grounding is gated on Confidence, not status (ADR 0021).
const (
	ConfidenceDraft    = "draft"
	ConfidenceVerified = "verified"
)

// IssueRef is a minimal description of an issue used for coverage computation.
// Callers populate this from their own state representation so that this package
// does not need to import materialize (which would create an import cycle).
type IssueRef struct {
	ID                      string
	SourceLinkCount         int
	CitationAcceptanceCount int
	Confidence              string
}

// Finding is a Graph Finding: a rule violation Compute reports, identified by
// a rule and the issue IDs it cites. Ungrounded Verified issues are Findings;
// ungrounded Draft issues are not (ADR 0021).
type Finding struct {
	Rule     string   `json:"rule"`
	Message  string   `json:"message"`
	CitedIDs []string `json:"cited_ids"`
}

// Coverage holds traceability metrics for the current materialized state.
// Headline totals and CoveragePct are the Verified band so the percentage
// stays meaningful; Draft is reported separately and does not mix in.
type Coverage struct {
	TotalNodes          int       `json:"total_nodes"`
	CitedNodes          int       `json:"cited_nodes"`
	CoveragePct         float64   `json:"coverage_pct"`
	AcceptedRiskNodes   int       `json:"accepted_risk_nodes"`
	AcceptedRiskPct     float64   `json:"accepted_risk_pct"`
	Uncited             []string  `json:"uncited"`
	Findings            []Finding `json:"findings"`
	DraftTotal          int       `json:"draft_total"`
	DraftCited          int       `json:"draft_cited"`
	DraftCoveragePct    float64   `json:"draft_coverage_pct"`
	VerifiedTotal       int       `json:"verified_total"`
	VerifiedCited       int       `json:"verified_cited"`
	VerifiedCoveragePct float64   `json:"verified_coverage_pct"`
}

// Compute calculates traceability coverage from a slice of IssueRef values.
// An issue is considered "cited" if its SourceLinkCount > 0.
// An issue is counted as "accepted risk" if it has no source link but has one
// or more CitationAcceptance records.
//
// Grounding is gated on Confidence (CITEGATE-T2 / ADR 0021): an ungrounded
// Draft is legal and is not a Graph Finding; an ungrounded Verified is.
func Compute(refs []IssueRef) Coverage {
	verifiedTotal := 0
	verifiedCited := 0
	acceptedRisk := 0
	draftTotal := 0
	draftCited := 0
	var uncited []string
	var findings []Finding

	for _, ref := range refs {
		if ref.Confidence == ConfidenceDraft {
			draftTotal++
			if ref.SourceLinkCount > 0 {
				draftCited++
			}
			continue
		}

		verifiedTotal++
		if ref.SourceLinkCount > 0 {
			verifiedCited++
			continue
		}
		if ref.CitationAcceptanceCount > 0 {
			acceptedRisk++
		}
		uncited = append(uncited, ref.ID)
		findings = append(findings, Finding{
			Rule:     "E7",
			Message:  fmt.Sprintf("uncited node: %s", ref.ID),
			CitedIDs: []string{ref.ID},
		})
	}

	sort.Strings(uncited)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Message < findings[j].Message
	})

	var pct float64
	if verifiedTotal > 0 {
		pct = float64(verifiedCited) / float64(verifiedTotal) * 100.0
	}

	var acceptedRiskPct float64
	if verifiedTotal > 0 {
		acceptedRiskPct = float64(acceptedRisk) / float64(verifiedTotal) * 100.0
	}

	var draftPct float64
	if draftTotal > 0 {
		draftPct = float64(draftCited) / float64(draftTotal) * 100.0
	}

	return Coverage{
		TotalNodes:          verifiedTotal,
		CitedNodes:          verifiedCited,
		CoveragePct:         pct,
		AcceptedRiskNodes:   acceptedRisk,
		AcceptedRiskPct:     acceptedRiskPct,
		Uncited:             uncited,
		Findings:            findings,
		DraftTotal:          draftTotal,
		DraftCited:          draftCited,
		DraftCoveragePct:    draftPct,
		VerifiedTotal:       verifiedTotal,
		VerifiedCited:       verifiedCited,
		VerifiedCoveragePct: pct,
	}
}

// Write serializes a Coverage value to the given path as JSON (atomic write via temp file).
func Write(path string, c Coverage) error {
	return adapters.WriteCoverageFile(path, c)
}

// Read deserializes a Coverage value from the given path.
// If the file does not exist, a zero Coverage is returned with no error.
func Read(path string) (Coverage, error) {
	data, err := adapters.ReadCoverageFile(path)
	if err != nil {
		return Coverage{}, err
	}
	if data == nil {
		return Coverage{}, nil
	}
	var c Coverage
	if err := json.Unmarshal(data, &c); err != nil {
		return Coverage{}, err
	}
	return c, nil
}
