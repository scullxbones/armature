package promotion_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
)

// cliDecide mirrors cmd/armature remediationDecide using real claim predicates
// (package promotion cannot import claim in non-test code).
func cliDecide(viewer string, now int64) func(string, promotion.Graph) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	return func(argv string, g promotion.Graph) error {
		argv = strings.TrimSpace(argv)
		if !strings.HasPrefix(argv, "arm ") {
			return nil
		}
		issue := promotion.IssueFlag(argv)
		if issue == "" {
			return nil
		}
		node := g[issue]
		switch {
		case strings.Contains(argv, "arm claim "):
			if promotion.WouldViolateTreeConsistency(issue, g) {
				return fmt.Errorf("tree consistency")
			}
			held := claim.LeaseFromClocks(node.Status, node.ClaimedBy, node.ClaimToken,
				node.ClaimedAt, node.LastActivity, 0, node.TTLMinutes, "")
			if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
				return fmt.Errorf("foreign live lease")
			}
		case strings.Contains(argv, "--to cancelled"):
			switch node.Status {
			case ops.StatusClaimed, ops.StatusInProgress:
				held := claim.LeaseFromClocks(node.Status, node.ClaimedBy, node.ClaimToken,
					node.ClaimedAt, node.LastActivity, 0, node.TTLMinutes, "")
				if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
					return claim.ErrNotClaimOwner
				}
				if held.Holder != "" && viewer != "" && held.Holder != viewer {
					return claim.ErrNotClaimOwner
				}
			}
		}
		return nil
	}
}

func TestCLIDecide_UsesClaimForeignLiveLeaseBlocks_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	now := int64(1_700_000_000)
	g := promotion.Graph{
		"E": {Status: ops.StatusMerged, Children: []string{"C"}},
		"C": {
			Status: ops.StatusClaimed, Parent: "E", ClaimedBy: "other",
			ClaimToken: "tok", ClaimedAt: now - 10, LastActivity: now - 10, TTLMinutes: 60,
		},
	}
	d := cliDecide("me", now)
	assertErr := func(argv string) {
		t.Helper()
		if err := d(argv, g); err == nil {
			t.Fatalf("expected error for %s", argv)
		}
	}
	assertErr("arm claim --issue C --worktree")
	assertErr("arm transition --issue C --to cancelled --force")

	// Expired: claim allowed; cancel still not-owner.
	g["C"] = promotion.Node{
		Status: ops.StatusClaimed, Parent: "E", ClaimedBy: "other",
		ClaimToken: "tok", ClaimedAt: now - 4000, LastActivity: now - 4000, TTLMinutes: 1,
	}
	if err := d("arm claim --issue C --worktree", g); err == nil {
		t.Fatal("claim under merged must still refuse")
	}
	// Remove merged ancestor for expired claim check.
	g2 := promotion.Graph{
		"C": {
			Status: ops.StatusClaimed, ClaimedBy: "other", ClaimToken: "tok",
			ClaimedAt: now - 4000, LastActivity: now - 4000, TTLMinutes: 1,
		},
	}
	if err := d("arm claim --issue C --worktree", g2); err != nil {
		t.Fatalf("expired foreign claim must pass ForeignLiveLeaseBlocks: %v", err)
	}
	if err := d("arm transition --issue C --to cancelled --force", g2); err == nil {
		t.Fatal("expired foreign cancel must still be NOT-CLAIM-OWNER")
	}
}
