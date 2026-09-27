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

// ScopesOverlap checks if two scope glob lists have any overlap.
func ScopesOverlap(scopeA, scopeB []string) bool {
	for _, a := range scopeA {
		for _, b := range scopeB {
			overlap, err := scopematch.Overlaps(a, b)
			if claimTreatsMalformedScopeAsNoOverlap(overlap, err) {
				return true
			}
		}
	}
	return false
}

func claimTreatsMalformedScopeAsNoOverlap(overlap bool, err error) bool {
	return err == nil && overlap
}

// ScopesOverlapIgnoringAncestry reports glob overlap except when issueA and
// issueB sit on the same parent-child chain (see IsAncestorOrDescendant).
func ScopesOverlapIgnoringAncestry(scopeA, scopeB []string, graph HierarchyGraph, issueA, issueB string) bool {
	if IsAncestorOrDescendant(graph, issueA, issueB) {
		return false
	}
	return ScopesOverlap(scopeA, scopeB)
}

// IsAncestorOrDescendant reports whether one issue sits on the other's
// parent-child chain. A nil graph is treated as no relationship.
func IsAncestorOrDescendant(graph HierarchyGraph, issueA, issueB string) bool {
	if graph == nil {
		return false
	}
	return slices.Contains(graph.Descendants(issueA), issueB) ||
		slices.Contains(graph.Descendants(issueB), issueA)
}

// IsWithinScope checks if all files in the provided list are within the
// declared scope globs. It returns (true, "") if all files are in scope,
// or (false, "filename") if a file is outside scope.
// An empty files list is considered within any scope.
func IsWithinScope(files, scope []string) (bool, string) {
	if len(files) == 0 {
		return true, ""
	}

	for _, file := range files {
		if !scopematch.Allows(scope, file) {
			return false, file
		}
	}

	return true, ""
}
