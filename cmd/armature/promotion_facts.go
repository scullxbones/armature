package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
)

func promotionIssueFacts(issue materialize.Issue) promotion.IssueFacts {
	f := promotion.IssueFacts{
		ID:                issue.ID,
		Status:            issue.Status,
		Base:              issue.Base,
		Tip:               issue.Tip,
		IntegrationBranch: issue.IntegrationBranch,
		Children:          append([]string(nil), issue.Children...),
	}
	for _, att := range issue.AssessmentAttestations {
		f.AssessmentAttestations = append(f.AssessmentAttestations, promotion.AssessmentFact{
			BaseSHA: att.BaseSHA,
			HeadSHA: att.HeadSHA,
		})
	}
	return f
}

func promotionGraphFromIndex(index materialize.Index) promotion.Graph {
	g := make(promotion.Graph, len(index))
	for id, entry := range index {
		g[id] = promotion.Node{
			Status:    entry.Status,
			Parent:    entry.Parent,
			Children:  append([]string(nil), entry.Children...),
			ClaimedBy: entry.Assignee,
		}
	}
	return g
}

// promotionGraphFromIssues includes delivery Base/Tip when known so remediation
// can emit filled arm delivery record argv instead of <sha> templates.
func promotionGraphFromIssues(issues map[string]*materialize.Issue) promotion.Graph {
	g := make(promotion.Graph, len(issues))
	for id, issue := range issues {
		if issue == nil {
			continue
		}
		g[id] = nodeFromMaterializeIssue(issue)
	}
	return g
}

func nodeFromMaterializeIssue(issue *materialize.Issue) promotion.Node {
	last := issue.ClaimedAt
	if issue.LastHeartbeat > last {
		last = issue.LastHeartbeat
	}
	if issue.LastClaimingWorkerActivity > last {
		last = issue.LastClaimingWorkerActivity
	}
	return promotion.Node{
		Status:       issue.Status,
		Parent:       issue.Parent,
		Children:     append([]string(nil), issue.Children...),
		Base:         issue.Base,
		Tip:          issue.Tip,
		ClaimedBy:    issue.ClaimedBy,
		ClaimToken:   issue.ClaimToken,
		ClaimedAt:    issue.ClaimedAt,
		LastActivity: last,
		TTLMinutes:   issue.ClaimTTL,
	}
}

// promotionGraphCache lazily builds and memoizes an enriched promotion graph
// for one store+index pair. Fail-closed: any ReadIssue error aborts the build.
type promotionGraphCache struct {
	store *snapshot.Store
	index materialize.Index

	once  sync.Once
	graph promotion.Graph
	err   error
}

func newPromotionGraphCache(store *snapshot.Store, index materialize.Index) *promotionGraphCache {
	return &promotionGraphCache{store: store, index: index}
}

func (c *promotionGraphCache) Graph() (promotion.Graph, error) {
	if c == nil {
		return nil, fmt.Errorf("promotion graph: nil cache")
	}
	c.once.Do(func() {
		c.graph, c.err = buildPromotionGraphFromStore(c.store, c.index)
	})
	return c.graph, c.err
}

// promotionGraphFromStore enriches the index graph with Base/Tip + lease clocks
// via ReadIssue only — no rematerialize / checkpoint write (safe for arm merged).
// Fail-closed on ReadIssue error. Prefer newPromotionGraphCache when the same
// graph may be requested more than once in a command.
func promotionGraphFromStore(store *snapshot.Store, index materialize.Index) (promotion.Graph, error) {
	return buildPromotionGraphFromStore(store, index)
}

func buildPromotionGraphFromStore(store *snapshot.Store, index materialize.Index) (promotion.Graph, error) {
	g := promotionGraphFromIndex(index)
	if store == nil {
		return g, nil
	}
	for id, node := range g {
		// Lazy: only ReadIssue when delivery clocks / lease facts matter.
		if !needsPromotionEnrichment(node) {
			continue
		}
		issue, err := store.ReadIssue(id)
		if err != nil {
			return nil, fmt.Errorf("promotion graph: read issue %s: %w", id, err)
		}
		if issue == nil {
			continue
		}
		enriched := nodeFromMaterializeIssue(issue)
		if enriched.Parent == "" {
			enriched.Parent = node.Parent
		}
		if len(enriched.Children) == 0 {
			enriched.Children = node.Children
		}
		g[id] = enriched
	}
	return g, nil
}

func needsPromotionEnrichment(node promotion.Node) bool {
	switch node.Status {
	case ops.StatusDone, ops.StatusClaimed, ops.StatusInProgress, ops.StatusBlocked:
		return true
	default:
		return strings.TrimSpace(node.ClaimedBy) != ""
	}
}
