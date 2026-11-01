package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
)

// remediationDecide returns the CLI decision predicate used before printing any
// remediation argv (Options, MoreSteps, CLAIM-1 next_actions). It calls the same
// claim.ForeignLiveLeaseBlocks AcceptAt uses and requirePublishedOwner transition
// uses — not a promotion-local copy.
func remediationDecide(appCtx *config.Context, viewer string, now int64) func(string, promotion.Graph) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	return func(argv string, g promotion.Graph) error {
		argv = strings.TrimSpace(argv)
		if argv == "" || !strings.HasPrefix(argv, "arm ") {
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
				return fmt.Errorf("cannot claim %s: under a merged ancestor (tree consistency)", issue)
			}
			held := leaseFromPromotionNode(node)
			if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
				return fmt.Errorf("cannot claim %s: foreign live lease held by %s (claim --force only overrides scope overlap)", issue, held.Holder)
			}
			return nil
		case strings.Contains(argv, "--to cancelled"):
			switch node.Status {
			case ops.StatusClaimed, ops.StatusInProgress:
				if appCtx != nil {
					if err := requirePublishedOwner(appCtx, issue, viewer); err != nil {
						return err
					}
				} else {
					held := leaseFromPromotionNode(node)
					if claim.ForeignLiveLeaseBlocks(held, viewer, now) {
						return claim.ErrNotClaimOwner
					}
					if held.Holder != "" && viewer != "" && held.Holder != viewer {
						// Expired foreign: still not owner — transition refuses.
						return claim.ErrNotClaimOwner
					}
				}
			}
			return nil
		default:
			return nil
		}
	}
}

func leaseFromPromotionNode(n promotion.Node) claim.Lease {
	return claim.LeaseFromClocks(
		n.Status, n.ClaimedBy, n.ClaimToken,
		n.ClaimedAt, n.LastActivity, 0,
		n.TTLMinutes, "",
	)
}

// planOptsForRemediation builds PlanOpts with CLI Decide for print paths.
func planOptsForRemediation(appCtx *config.Context, viewer string) promotion.PlanOpts {
	now := time.Now().Unix()
	return promotion.PlanOpts{
		Viewer: viewer,
		Now:    now,
		Decide: remediationDecide(appCtx, viewer, now),
	}
}
