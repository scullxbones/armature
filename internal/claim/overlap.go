package claim

import (
	"slices"

	"github.com/scullxbones/armature/internal/scopematch"
)

// HierarchyGraph defines the minimal interface needed for ancestor/descendant checking.
// This interface allows checking if one issue is a descendant of another.
type HierarchyGraph interface {
	// Descendants returns all downstream descendants of a node (all nodes that
	// depend on this node being completed, following child links).
	Descendants(id string) []string
}

// ScopesOverlap checks if two scope glob lists have any overlap.
func ScopesOverlap(scopeA, scopeB []string) bool {
	for _, a := range scopeA {
		for _, b := range scopeB {
			if globOverlaps(a, b) {
				return true
			}
		}
	}
	return false
}

// ScopesOverlapEx checks if two scope glob lists have any overlap,
// excluding ancestor/descendant issue pairs from overlap detection.
// A parent story's scope is by design the union of its children's scopes,
// so parent/child scope overlap is not a real conflict and should not be reported.
func ScopesOverlapEx(scopeA, scopeB []string, graph HierarchyGraph, issueA, issueB string) bool {
	// If no graph is provided, fall back to basic scope overlap check
	if graph == nil {
		return ScopesOverlap(scopeA, scopeB)
	}

	// Check if issueA is an ancestor of issueB (issueB is a descendant of issueA)
	if slices.Contains(graph.Descendants(issueA), issueB) {
		// issueA is ancestor of issueB — exclude from overlap detection
		return false
	}

	// Check if issueB is an ancestor of issueA (issueA is a descendant of issueB)
	if slices.Contains(graph.Descendants(issueB), issueA) {
		// issueB is ancestor of issueA — exclude from overlap detection
		return false
	}

	// Neither is an ancestor of the other — check scope overlap normally
	return ScopesOverlap(scopeA, scopeB)
}

// globOverlaps delegates to scopematch.Overlaps, the single canonical
// overlap-matching implementation shared with internal/validate, so the two
// layers cannot diverge again as they once did. Glob-to-glob intersection
// (e.g. "src/auth/*.go" vs "src/auth/login.*") lives in that shared
// implementation; it is not re-derived here.
func globOverlaps(a, b string) bool {
	return scopematch.Overlaps(a, b)
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
