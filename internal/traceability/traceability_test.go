package traceability_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/traceability"
)

func TestCoverageAllCited(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// Empty Confidence is Verified (CITEGATE-T2); unlabeled uncited nodes remain Graph Findings.
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
	t.Parallel()
	// Empty Confidence is Verified (CITEGATE-T2); mixed unlabeled coverage is the Verified band.
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestUngroundedDraftIsNotAGraphFinding_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "DRAFT-1", Confidence: traceability.ConfidenceDraft, SourceLinkCount: 0},
	}

	cov := traceability.Compute(refs)

	if len(cov.Findings) != 0 {
		t.Errorf("ungrounded Draft must not be a Graph Finding, got %v", cov.Findings)
	}
	if containsID(cov.Uncited, "DRAFT-1") {
		t.Errorf("ungrounded Draft must not appear in Uncited, got %v", cov.Uncited)
	}
}

func TestUngroundedVerifiedIsAGraphFinding_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "VER-1", Confidence: traceability.ConfidenceVerified, SourceLinkCount: 0},
	}

	cov := traceability.Compute(refs)

	if len(cov.Findings) != 1 {
		t.Fatalf("ungrounded Verified must be a Graph Finding, got %v", cov.Findings)
	}
	if !containsID(cov.Findings[0].CitedIDs, "VER-1") {
		t.Errorf("Graph Finding must cite VER-1, got %v", cov.Findings[0].CitedIDs)
	}
	if !containsID(cov.Uncited, "VER-1") {
		t.Errorf("ungrounded Verified must appear in Uncited, got %v", cov.Uncited)
	}
}

func TestDraftCitationStatusIsPreserved_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "DRAFT-UNCITED", Confidence: traceability.ConfidenceDraft, SourceLinkCount: 0},
		{ID: "DRAFT-CITED", Confidence: traceability.ConfidenceDraft, SourceLinkCount: 1},
	}

	cov := traceability.Compute(refs)

	// Drafts stay out of the headline Uncited list and out of Findings...
	if len(cov.Findings) != 0 {
		t.Errorf("draft must not be a Graph Finding, got %v", cov.Findings)
	}
	if containsID(cov.Uncited, "DRAFT-UNCITED") {
		t.Errorf("draft must not appear in Uncited, got %v", cov.Uncited)
	}
	// ...but their citation status is still reported, so dag summary can require
	// per-node acknowledgment for an unlinked draft.
	if !containsID(cov.DraftUncited, "DRAFT-UNCITED") {
		t.Errorf("expected DRAFT-UNCITED in DraftUncited, got %v", cov.DraftUncited)
	}
	if containsID(cov.DraftUncited, "DRAFT-CITED") {
		t.Errorf("source-linked draft must not appear in DraftUncited, got %v", cov.DraftUncited)
	}
}

func TestInferredIsNotInVerifiedBand_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "INF-1", Confidence: traceability.ConfidenceInferred, SourceLinkCount: 0},
	}

	cov := traceability.Compute(refs)

	if len(cov.Findings) != 0 {
		t.Errorf("ungrounded Inferred must not be a Graph Finding, got %v", cov.Findings)
	}
	if containsID(cov.Uncited, "INF-1") {
		t.Errorf("ungrounded Inferred must not appear in Uncited, got %v", cov.Uncited)
	}
	if cov.VerifiedTotal != 0 {
		t.Errorf("expected VerifiedTotal=0, got %d", cov.VerifiedTotal)
	}
	if cov.InferredTotal != 1 || cov.InferredCited != 0 {
		t.Errorf("inferred band: total=%d cited=%d, want 1/0", cov.InferredTotal, cov.InferredCited)
	}
	if cov.DraftTotal != 0 {
		t.Errorf("expected Inferred to stay out of the Draft band, got DraftTotal=%d", cov.DraftTotal)
	}
}

func TestAcceptedRiskIsNotAGraphFinding_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "VER-1", Confidence: traceability.ConfidenceVerified, SourceLinkCount: 0, CitationAcceptanceCount: 1},
	}

	cov := traceability.Compute(refs)

	if cov.AcceptedRiskNodes != 1 {
		t.Errorf("expected AcceptedRiskNodes=1, got %d", cov.AcceptedRiskNodes)
	}
	if !containsID(cov.Uncited, "VER-1") {
		t.Errorf("accepted-risk node must still appear in Uncited, got %v", cov.Uncited)
	}
	if len(cov.Findings) != 0 {
		t.Errorf("accepted-risk node must not be an E7 Finding, got %v", cov.Findings)
	}
}

func TestCoverageSeparatesDraftFromVerified_REQ_CITEGATE_T2(t *testing.T) {
	t.Parallel()
	refs := []traceability.IssueRef{
		{ID: "DRAFT-1", Confidence: traceability.ConfidenceDraft, SourceLinkCount: 0},
		{ID: "VER-1", Confidence: traceability.ConfidenceVerified, SourceLinkCount: 1},
	}

	cov := traceability.Compute(refs)

	if cov.VerifiedTotal != 1 || cov.VerifiedCited != 1 || cov.VerifiedCoveragePct != 100.0 {
		t.Errorf("verified coverage: total=%d cited=%d pct=%f, want 1/1 100%%",
			cov.VerifiedTotal, cov.VerifiedCited, cov.VerifiedCoveragePct)
	}
	if cov.DraftTotal != 1 || cov.DraftCited != 0 || cov.DraftCoveragePct != 0.0 {
		t.Errorf("draft coverage: total=%d cited=%d pct=%f, want 1/0 0%%",
			cov.DraftTotal, cov.DraftCited, cov.DraftCoveragePct)
	}
	// Mixing Draft and Verified would report 50% and make the percentage lie.
	if cov.CoveragePct != 100.0 {
		t.Errorf("CoveragePct must be verified-only (100), got %f", cov.CoveragePct)
	}
	if cov.TotalNodes != 1 || cov.CitedNodes != 1 {
		t.Errorf("headline totals must be verified-only: TotalNodes=%d CitedNodes=%d, want 1/1",
			cov.TotalNodes, cov.CitedNodes)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
