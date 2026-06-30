package traceability_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/traceability"
)

func TestCoverageAllCited(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-1", SourceLinkCount: 2},
		{ID: "ISSUE-2", SourceLinkCount: 1},
		{ID: "ISSUE-3", SourceLinkCount: 3},
	}

	cov := traceability.Compute(refs)

	if cov.TotalNodes != 3 {
		t.Errorf("expected TotalNodes=3, got %d", cov.TotalNodes)
	}
	if cov.CitedNodes != 3 {
		t.Errorf("expected CitedNodes=3, got %d", cov.CitedNodes)
	}
	if cov.CoveragePct != 100.0 {
		t.Errorf("expected CoveragePct=100.0, got %f", cov.CoveragePct)
	}
	if len(cov.Uncited) != 0 {
		t.Errorf("expected empty Uncited, got %v", cov.Uncited)
	}
}

func TestCoverageNoneCited(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-A", SourceLinkCount: 0},
		{ID: "ISSUE-B", SourceLinkCount: 0},
	}

	cov := traceability.Compute(refs)

	if cov.TotalNodes != 2 {
		t.Errorf("expected TotalNodes=2, got %d", cov.TotalNodes)
	}
	if cov.CitedNodes != 0 {
		t.Errorf("expected CitedNodes=0, got %d", cov.CitedNodes)
	}
	if cov.CoveragePct != 0.0 {
		t.Errorf("expected CoveragePct=0.0, got %f", cov.CoveragePct)
	}
	if len(cov.Uncited) != 2 {
		t.Errorf("expected 2 Uncited entries, got %v", cov.Uncited)
	}
	uncitedSet := make(map[string]bool)
	for _, id := range cov.Uncited {
		uncitedSet[id] = true
	}
	for _, id := range []string{"ISSUE-A", "ISSUE-B"} {
		if !uncitedSet[id] {
			t.Errorf("expected %s in Uncited, got %v", id, cov.Uncited)
		}
	}
}

func TestCoveragePartial(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-1", SourceLinkCount: 1},
		{ID: "ISSUE-2", SourceLinkCount: 0},
		{ID: "ISSUE-3", SourceLinkCount: 0},
		{ID: "ISSUE-4", SourceLinkCount: 2},
	}

	cov := traceability.Compute(refs)

	if cov.TotalNodes != 4 {
		t.Errorf("expected TotalNodes=4, got %d", cov.TotalNodes)
	}
	if cov.CitedNodes != 2 {
		t.Errorf("expected CitedNodes=2, got %d", cov.CitedNodes)
	}
	if cov.CoveragePct != 50.0 {
		t.Errorf("expected CoveragePct=50.0, got %f", cov.CoveragePct)
	}
	if len(cov.Uncited) != 2 {
		t.Errorf("expected 2 Uncited entries, got %v", cov.Uncited)
	}
	uncitedSet := make(map[string]bool)
	for _, id := range cov.Uncited {
		uncitedSet[id] = true
	}
	for _, id := range []string{"ISSUE-2", "ISSUE-3"} {
		if !uncitedSet[id] {
			t.Errorf("expected %s in Uncited, got %v", id, cov.Uncited)
		}
	}
}

// TestCompute_AllSourceLinked verifies that issues with source links are fully
// covered and none are in the accepted-risk bucket.
func TestCompute_AllSourceLinked(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-1", SourceLinkCount: 1, CitationAcceptanceCount: 0},
		{ID: "ISSUE-2", SourceLinkCount: 2, CitationAcceptanceCount: 0},
	}

	cov := traceability.Compute(refs)

	if cov.TotalNodes != 2 {
		t.Errorf("expected TotalNodes=2, got %d", cov.TotalNodes)
	}
	if cov.CitedNodes != 2 {
		t.Errorf("expected CitedNodes=2, got %d", cov.CitedNodes)
	}
	if cov.AcceptedRiskNodes != 0 {
		t.Errorf("expected AcceptedRiskNodes=0, got %d", cov.AcceptedRiskNodes)
	}
	if cov.AcceptedRiskPct != 0.0 {
		t.Errorf("expected AcceptedRiskPct=0.0, got %f", cov.AcceptedRiskPct)
	}
	if len(cov.Uncited) != 0 {
		t.Errorf("expected empty Uncited, got %v", cov.Uncited)
	}
}

// TestCompute_MixedCitation verifies that issues with only acceptance (no source
// link) land in AcceptedRiskNodes and still appear in Uncited.
func TestCompute_MixedCitation(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-1", SourceLinkCount: 1, CitationAcceptanceCount: 0},
		{ID: "ISSUE-2", SourceLinkCount: 0, CitationAcceptanceCount: 1},
		{ID: "ISSUE-3", SourceLinkCount: 0, CitationAcceptanceCount: 0},
	}

	cov := traceability.Compute(refs)

	if cov.TotalNodes != 3 {
		t.Errorf("expected TotalNodes=3, got %d", cov.TotalNodes)
	}
	if cov.CitedNodes != 1 {
		t.Errorf("expected CitedNodes=1, got %d", cov.CitedNodes)
	}
	if cov.AcceptedRiskNodes != 1 {
		t.Errorf("expected AcceptedRiskNodes=1, got %d", cov.AcceptedRiskNodes)
	}
	expectedPct := float64(1) / float64(3) * 100.0
	if cov.AcceptedRiskPct != expectedPct {
		t.Errorf("expected AcceptedRiskPct=%f, got %f", expectedPct, cov.AcceptedRiskPct)
	}
	// ISSUE-2 and ISSUE-3 are uncited (no source link)
	if len(cov.Uncited) != 2 {
		t.Errorf("expected 2 Uncited entries, got %v", cov.Uncited)
	}
}

// TestCompute_BothSourceLinkAndAcceptance_CountsAsSourceLinked verifies that an
// issue with both a source link and an acceptance record is counted as cited, not
// as accepted risk.
func TestCompute_BothSourceLinkAndAcceptance_CountsAsSourceLinked(t *testing.T) {
	refs := []traceability.IssueRef{
		{ID: "ISSUE-1", SourceLinkCount: 1, CitationAcceptanceCount: 1},
	}

	cov := traceability.Compute(refs)

	if cov.CitedNodes != 1 {
		t.Errorf("expected CitedNodes=1, got %d", cov.CitedNodes)
	}
	if cov.AcceptedRiskNodes != 0 {
		t.Errorf("expected AcceptedRiskNodes=0, got %d", cov.AcceptedRiskNodes)
	}
	if len(cov.Uncited) != 0 {
		t.Errorf("expected empty Uncited, got %v", cov.Uncited)
	}
}
