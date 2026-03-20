package traceability_test

import (
	"testing"

	"github.com/scullxbones/trellis/internal/traceability"
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
