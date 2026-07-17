package claim

import (
	"path/filepath"
	"strings"
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
	descendants := graph.Descendants(issueA)
	for _, desc := range descendants {
		if desc == issueB {
			// issueA is ancestor of issueB — exclude from overlap detection
			return false
		}
	}

	// Check if issueB is an ancestor of issueA (issueA is a descendant of issueB)
	descendants = graph.Descendants(issueB)
	for _, desc := range descendants {
		if desc == issueA {
			// issueB is ancestor of issueA — exclude from overlap detection
			return false
		}
	}

	// Neither is an ancestor of the other — check scope overlap normally
	return ScopesOverlap(scopeA, scopeB)
}

func globOverlaps(a, b string) bool {
	if matched, _ := filepath.Match(a, b); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	if matched, _ := filepath.Match(b, a); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	dirA := extractDir(a)
	dirB := extractDir(b)
	if dirA == "" || dirB == "" {
		return false
	}
	return dirA == dirB || hasPrefix(dirA, dirB+"/") || hasPrefix(dirB, dirA+"/")
}

func extractDir(pattern string) string {
	i := strings.LastIndexByte(pattern, '/')
	if i < 0 {
		return ""
	}
	return pattern[:i]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
