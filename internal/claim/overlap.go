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
// directory scope) matching the other, or two glob patterns whose
// languages intersect (e.g. "src/auth/*.go" and "src/auth/login.*" both
// match "src/auth/login.go", and "src/**/foo.go" and "src/auth/*.go" both
// match "src/auth/foo.go"). It deliberately does NOT fall back to "shares a
// containing/ancestor directory": two literal entries whose paths differ
// (e.g. "docs/agents/quality-gates.md" and "docs/use-cases.md") are never
// reported as overlapping by this function. That is materially narrower
// than the removed ancestor-prefix fallback, which matched on directory
// containment alone.
//
// The glob-to-glob case is a deliberate over-approximation: when a precise
// intersection answer isn't cheap to compute in general, this errs toward
// reporting overlap rather than missing one, because a false positive here
// costs a reviewed --force while a false negative lets two workers land
// concurrent, conflicting claims on the same file.
func globOverlaps(a, b string) bool {
	a = normalizeScopeEntry(a)
	b = normalizeScopeEntry(b)
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

// normalizeScopeEntry strips worker annotations such as " (new)" and
// collapses "./" prefixes via scopematch.CleanScope. Directory scopes keep
// their trailing slash so Allows still treats them as covering children.
func normalizeScopeEntry(raw string) string {
	cleaned, isDir := scopematch.CleanScope(raw)
	if isDir && cleaned != "." {
		return cleaned + "/"
	}
	return cleaned
}

// globToGlobIntersects reports whether two path patterns can both match a
// common path. It walks slash-separated segments (with "**" spanning zero
// or more segments) and intersects each pair of filename-level globs.
// Character classes are over-approximated as "?" (any one character): a
// false positive costs a reviewed --force, a false negative lets two
// workers share a file. Two literal files that merely share a directory
// still do not overlap: a literal segment only intersects another literal
// when they are equal.
func globToGlobIntersects(a, b string) bool {
	segsA := pathPatternSegments(a)
	segsB := pathPatternSegments(b)
	memo := make(map[[2]int]bool, len(segsA)*len(segsB))
	return pathSegsIntersect(segsA, segsB, 0, 0, memo)
}

// pathPatternSegments splits a scope pattern on "/". A trailing slash is
// treated as a directory scope and rewritten to a final "**" segment so
// "src/" intersects every path under src/.
func pathPatternSegments(pattern string) []string {
	if pattern == "" {
		return nil
	}
	trailing := strings.HasSuffix(pattern, "/")
	trimmed := strings.TrimSuffix(pattern, "/")
	if trimmed == "" {
		return []string{"**"}
	}
	segs := strings.Split(trimmed, "/")
	if trailing {
		segs = append(segs, "**")
	}
	return segs
}

// pathSegsIntersect reports whether suffixes a[i:] and b[j:] can both
// match a common sequence of path segments.
func pathSegsIntersect(a, b []string, i, j int, memo map[[2]int]bool) bool {
	key := [2]int{i, j}
	if v, ok := memo[key]; ok {
		return v
	}
	// Deliberately if/else rather than switch: gremlins' mutation coverage
	// does not instrument switch-case guard expressions.
	var result bool
	aDone, bDone := i == len(a), j == len(b)
	if aDone && bDone { //nolint:gocritic // see comment above
		result = true
	} else if !aDone && a[i] == "**" {
		result = pathSegsIntersect(a, b, i+1, j, memo) || (!bDone && pathSegsIntersect(a, b, i, j+1, memo))
	} else if !bDone && b[j] == "**" {
		result = pathSegsIntersect(a, b, i, j+1, memo) || (!aDone && pathSegsIntersect(a, b, i+1, j, memo))
	} else if aDone || bDone {
		result = false
	} else {
		result = globPatternsIntersect(a[i], b[j]) && pathSegsIntersect(a, b, i+1, j+1, memo)
	}
	memo[key] = result
	return result
}

// globPatternsIntersect reports whether two single-segment glob patterns
// can both match at least one common string. "*" is any run of characters,
// "?" is any one character, and a closed "[...]" class is treated as "?"
// (over-approximate; see globToGlobIntersects). When neither pattern
// contains a wildcard, this reduces to exact string equality.
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
	} else {
		nextA, anyA := nextAtom(a, i)
		nextB, anyB := nextAtom(b, j)
		result = (anyA || anyB || a[i] == b[j]) && patternsIntersectFrom(a, b, nextA, nextB, memo)
	}
	memo[key] = result
	return result
}

// nextAtom advances past one matching unit. A closed "[...]" class is
// consumed as a single "?"-like atom; an unclosed "[" stays a literal.
func nextAtom(s string, i int) (next int, any bool) {
	if s[i] == '?' {
		return i + 1, true
	}
	if s[i] == '[' {
		if closeAt := strings.IndexByte(s[i+1:], ']'); closeAt >= 0 {
			return i + 1 + closeAt + 1, true
		}
	}
	return i + 1, false
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
