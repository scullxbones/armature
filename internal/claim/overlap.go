package claim

import (
	"slices"

	"github.com/scullxbones/armature/internal/scopematch"
)

// HierarchyGraph defines the minimal interface needed for ancestor/descendant checking.
type HierarchyGraph interface {
	// Descendants returns all downstream descendants of a node (all nodes that
	// depend on this node being completed, following child links).
	Descendants(id string) []string
}

func scopesOverlapIgnoringAncestry(scopeA, scopeB []string, graph HierarchyGraph, issueA, issueB string) bool {
	if isAncestorOrDescendant(graph, issueA, issueB) {
		return false
	}
	return scopematch.ScopesOverlap(scopeA, scopeB)
}

func isAncestorOrDescendant(graph HierarchyGraph, issueA, issueB string) bool {
	if graph == nil {
		return false
	}
	return slices.Contains(graph.Descendants(issueA), issueB) ||
		slices.Contains(graph.Descendants(issueB), issueA)
}
