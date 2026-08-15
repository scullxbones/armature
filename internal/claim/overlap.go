package claim

import (
	"path/filepath"
	"slices"
	"strings"

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

// globOverlaps reports whether two scope entries overlap: exact path
// equality, one glob (including a "**" doublestar or trailing-slash
// directory scope) matching the other, or — when neither side is a literal
// path contained by the other — two glob patterns that share the same
// directory portion and could both match a common filename in that
// directory (e.g. "src/auth/*.go" and "src/auth/login.*" both match
// "src/auth/login.go"). It deliberately does NOT fall back to "shares a
// containing/ancestor directory": two entries whose directory portions
// differ (e.g. "docs/agents/quality-gates.md" and "docs/use-cases.md", with
// directories "docs/agents/" and "docs/") are never reported as overlapping
// by this function, no matter what their filename portions look like. That
// is materially narrower than the removed ancestor-prefix fallback, which
// matched on directory containment alone.
//
// The glob-to-glob case is a deliberate over-approximation: when a precise
// intersection answer isn't cheap to compute in general, this errs toward
// reporting overlap rather than missing one, because a false positive here
// costs a reviewed --force while a false negative lets two workers land
// concurrent, conflicting claims on the same file.
func globOverlaps(a, b string) bool {
	if matched, _ := filepath.Match(a, b); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	if matched, _ := filepath.Match(b, a); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	if scopematch.Allows([]string{a}, b) || scopematch.Allows([]string{b}, a) {
		return true
	}
	return globToGlobIntersects(a, b)
}

// globToGlobIntersects reports whether a and b, split into directory and
// filename portions, could both match a common filename: the directory
// portions must be identical (as literal strings — this is what keeps the
// check from degenerating into the old ancestor-directory fallback), and
// the filename portions must be able to match a common string under glob
// semantics (see globPatternsIntersect). Two literal filenames that merely
// share a directory are not treated as overlapping: globPatternsIntersect
// requires exact equality when neither filename pattern contains a wildcard.
func globToGlobIntersects(a, b string) bool {
	dirA, baseA := splitDirBase(a)
	dirB, baseB := splitDirBase(b)
	if dirA != dirB {
		return false
	}
	return globPatternsIntersect(baseA, baseB)
}

// splitDirBase splits a scope pattern into its directory portion (including
// the trailing slash, or "" if the pattern has no "/") and its final path
// segment.
func splitDirBase(pattern string) (dir, base string) {
	if i := strings.LastIndexByte(pattern, '/'); i >= 0 {
		return pattern[:i+1], pattern[i+1:]
	}
	return "", pattern
}

// globPatternsIntersect reports whether two single-segment glob patterns
// (each treated as a sequence of literal characters, "*" meaning zero or
// more characters, and "?" meaning exactly one character) can both match at
// least one common string. This is a language-intersection check between
// two patterns, not a match of one pattern against a known string: e.g.
// "*.go" and "login.*" intersect because "login.go" satisfies both, even
// though neither pattern is a substring or superset of the other. When
// neither pattern contains a wildcard, this reduces to exact string
// equality.
func globPatternsIntersect(a, b string) bool {
	memo := make(map[[2]int]bool, len(a)*len(b))
	return patternsIntersectFrom(a, b, 0, 0, memo)
}

// patternsIntersectFrom is the memoized recursion behind globPatternsIntersect:
// it reports whether the suffixes a[i:] and b[j:] can both match a common
// (possibly empty) string.
func patternsIntersectFrom(a, b string, i, j int, memo map[[2]int]bool) bool {
	key := [2]int{i, j}
	if v, ok := memo[key]; ok {
		return v
	}
	// Deliberately if/else rather than switch: gremlins' mutation coverage
	// does not instrument switch-case guard expressions, which left these
	// conditions permanently "not covered" despite exercising tests.
	var result bool
	if i == len(a) && j == len(b) { //nolint:gocritic // see comment above
		result = true
	} else if i == len(a) {
		result = b[j] == '*' && patternsIntersectFrom(a, b, i, j+1, memo)
	} else if j == len(b) {
		result = a[i] == '*' && patternsIntersectFrom(a, b, i+1, j, memo)
	} else if a[i] == '*' {
		result = patternsIntersectFrom(a, b, i+1, j, memo) || patternsIntersectFrom(a, b, i, j+1, memo)
	} else if b[j] == '*' {
		result = patternsIntersectFrom(a, b, i, j+1, memo) || patternsIntersectFrom(a, b, i+1, j, memo)
	} else if a[i] == '?' || b[j] == '?' || a[i] == b[j] {
		result = patternsIntersectFrom(a, b, i+1, j+1, memo)
	} else {
		result = false
	}
	memo[key] = result
	return result
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
