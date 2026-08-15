// Package scopematch implements the canonical scope-glob matching logic
// shared by internal/harnesspolicy (worker-facing scope checks) and
// internal/claim (delivery-gate scope overlap checks). It is a leaf package
// with no dependency on either caller, deliberately, to avoid the import
// cycle that would otherwise result: internal/harnesspolicy imports
// internal/materialize, which imports internal/claim, so internal/claim
// cannot import internal/harnesspolicy (or vice versa). Both callers
// previously carried their own hand-ported copy of this logic; keeping a
// single implementation here means a fix (e.g. normalizing a "./" prefix in
// a scope entry) only needs to happen once.
package scopematch

import (
	"path/filepath"
	"strings"
)

// Overlaps reports whether two scope entries overlap: exact path equality,
// one glob (including a "**" doublestar or trailing-slash directory scope)
// matching the other, or two glob patterns that could both match some common
// concrete path (e.g. "src/auth/*.go" and "src/auth/login.*" both match
// "src/auth/login.go", even though neither pattern matches the other's
// literal string). It deliberately does NOT fall back to "shares a
// containing/ancestor directory" — two distinct files or globs that merely
// live under the same directory are not an overlap; a differing literal path
// segment (e.g. "auth" vs "billing") rules out any intersection at that
// segment regardless of wildcards elsewhere in the pattern.
//
// When both sides contribute a wildcard to the same path segment (e.g. the
// "*.go" vs "login.*" example above), computing the precise set of strings
// each pattern segment can produce and intersecting them is not cheap, so
// this deliberately over-approximates and reports an overlap. Under-blocking
// — silently letting two claims with a genuine glob-vs-glob conflict proceed
// concurrently — is the dangerous direction; an occasional false-positive
// warning costs a reviewed `--force`, not silent concurrent writes.
//
// Directory scopes are rewritten to cleaned/** before matching (see
// canonicalSegments) so descendant inclusion is a property of the pattern,
// not a parallel literal-prefix path that breaks once the directory itself
// contains a wildcard (e.g. "src/*/"). Intersection is memoized on suffix
// indexes so pairs of patterns with many "**" segments stay polynomial.
//
// This is the single canonical implementation shared by internal/claim
// (delivery-gate overlap checks) and internal/validate (scope-overlap
// warnings) so the two layers cannot diverge again as they once did.
func Overlaps(a, b string) bool {
	if matched, _ := filepath.Match(a, b); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	if matched, _ := filepath.Match(b, a); matched { //nolint:errcheck // ErrBadPattern unreachable for valid armature scope paths
		return true
	}
	if Allows([]string{a}, b) || Allows([]string{b}, a) {
		return true
	}
	return globPatternsMayIntersect(a, b)
}

// canonicalSegments is the single scope-entry normalization used by both
// Allows (pattern vs concrete path) and glob-vs-glob intersection. A
// trailing-slash directory scope becomes cleaned/** so descendants are
// matched by the same "**" expansion as an explicit directory glob. The
// repo-root entry "." is "**".
func canonicalSegments(raw string) []string {
	cleaned, isDir := CleanScope(raw)
	if cleaned == "." {
		return []string{"**"}
	}
	segs := strings.Split(cleaned, "/")
	if isDir {
		segs = append(segs, "**")
	}
	return segs
}

// globPatternsMayIntersect reports whether two scope-glob patterns could
// both match some common concrete path, comparing path segment by segment
// (with "**" expanding to zero or more segments, via the same backtracking
// approach as matchSegments). Two segments are considered
// compatible if they are identical, if one is a literal string the other's
// wildcard segment matches (via filepath.Match), or if both segments contain
// wildcard characters — in the last case the exact intersection of the two
// character classes/glob shapes is not computed; they are conservatively
// assumed compatible. A literal-vs-literal mismatch at any segment (e.g.
// "auth" vs "billing") is decisive and rules out intersection regardless of
// wildcards elsewhere in the pattern, which is what keeps this bounded:
// "src/auth/*.go" and "src/billing/*.go" still report no overlap.
func globPatternsMayIntersect(a, b string) bool {
	segA := canonicalSegments(a)
	segB := canonicalSegments(b)
	if (len(segA) == 1 && segA[0] == "**") || (len(segB) == 1 && segB[0] == "**") {
		return true
	}
	return matchPatternSegments(segA, segB)
}

// matchPatternSegments reports whether pattern segment lists a and b could
// both match some common list of concrete path segments. "**" in either list
// expands to zero or more segments via backtracking. Results are memoized
// on (i, j) suffix indexes so a pair of patterns with many "**" segments is
// polynomial in the segment counts instead of combinatorial in the
// backtracking tree.
func matchPatternSegments(a, b []string) bool {
	return matchPatternSegmentsAt(a, b, 0, 0, make(map[segmentKey]bool))
}

type segmentKey struct{ i, j int }

func matchPatternSegmentsAt(a, b []string, i, j int, memo map[segmentKey]bool) bool {
	k := segmentKey{i, j}
	if v, ok := memo[k]; ok {
		return v
	}
	result := computePatternSegments(a, b, i, j, memo)
	memo[k] = result
	return result
}

func computePatternSegments(a, b []string, i, j int, memo map[segmentKey]bool) bool {
	if i < len(a) && a[i] == "**" {
		if i+1 == len(a) {
			return true
		}
		for t := j; t <= len(b); t++ {
			if matchPatternSegmentsAt(a, b, i+1, t, memo) {
				return true
			}
		}
		return false
	}
	if j < len(b) && b[j] == "**" {
		if j+1 == len(b) {
			return true
		}
		for t := i; t <= len(a); t++ {
			if matchPatternSegmentsAt(a, b, t, j+1, memo) {
				return true
			}
		}
		return false
	}
	if i == len(a) || j == len(b) {
		return i == len(a) && j == len(b)
	}
	if !segmentsCompatible(a[i], b[j]) {
		return false
	}
	return matchPatternSegmentsAt(a, b, i+1, j+1, memo)
}

// segmentsCompatible reports whether two single-path-segment glob patterns
// could both match some common concrete segment string.
func segmentsCompatible(s1, s2 string) bool {
	if s1 == s2 {
		return true
	}
	w1 := isWildcardSegment(s1)
	w2 := isWildcardSegment(s2)
	switch {
	case !w1 && !w2:
		// Both literal and already known unequal (s1 == s2 handled above).
		return false
	case w1 && !w2:
		matched, err := filepath.Match(s1, s2)
		return err == nil && matched
	case !w1 && w2:
		matched, err := filepath.Match(s2, s1)
		return err == nil && matched
	default:
		// Both segments carry a wildcard; computing the precise
		// intersection of what each can match is not cheap, so
		// conservatively assume they can overlap. See Overlaps' doc
		// comment for why over-approximation here is the safe direction.
		return true
	}
}

// isWildcardSegment reports whether a single path segment contains glob
// wildcard characters.
func isWildcardSegment(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// Allows reports whether path is covered by any entry in scope. Scope
// entries may be:
//   - "." meaning the entire repository is in scope.
//   - An exact repo-relative path.
//   - A directory scope, either with a trailing slash ("dir/") or without,
//     covering everything at or under that directory.
//   - A glob pattern, including "**" doublestar segments spanning zero or
//     more path components.
//
// Both path and scope entries are normalized (via CleanRepoPath/CleanScope)
// before matching, so a "./"-prefixed scope entry like "./internal/**"
// matches the same set of files as "internal/**".
func Allows(scope []string, path string) bool {
	cleanedPath := CleanRepoPath(path)
	pathSegs := strings.Split(cleanedPath, "/")
	for _, rawScope := range scope {
		segs := canonicalSegments(rawScope)
		if len(segs) == 1 && segs[0] == "**" {
			return true
		}
		if matchSegments(segs, pathSegs) {
			return true
		}
	}
	return false
}

// CleanScope normalizes a raw scope entry and reports whether it denotes a
// directory scope (i.e. had a trailing slash before normalization), which
// covers everything at or under that directory regardless of whether the
// entry also contains "**". Before normalization, a trailing human-readable
// annotation like " (new)" is stripped -- workers commonly append this to a
// scope entry when declaring a file that doesn't exist yet (e.g.
// "internal/foo/bar.go (new)"), and scope entries are stored verbatim
// including the annotation, so any exact-path matcher must strip it before
// comparing against real file paths.
func CleanScope(raw string) (string, bool) {
	stripped := stripAnnotation(raw)
	return CleanRepoPath(stripped), strings.HasSuffix(filepath.ToSlash(stripped), "/")
}

// stripAnnotation removes a trailing " (...)" annotation from a scope entry,
// e.g. "internal/foo.go (new)" becomes "internal/foo.go". A parenthesized
// suffix is only treated as an annotation when preceded by a space, so a
// literal filename containing parens (e.g. "internal/foo/bar(baz).go") is
// left untouched.
func stripAnnotation(glob string) string {
	if i := strings.LastIndex(glob, " ("); i >= 0 && strings.HasSuffix(glob, ")") {
		return strings.TrimSpace(glob[:i])
	}
	return glob
}

// CleanRepoPath normalizes a path for scope comparison: converts to forward
// slashes, strips a leading "./", cleans the result (collapsing "..", "//",
// etc.), and strips any leading "/". The special value "." (repo root) is
// preserved as-is.
func CleanRepoPath(path string) string {
	slashed := filepath.ToSlash(path)
	trimmed := strings.TrimPrefix(slashed, "./")
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." {
		return "."
	}
	return strings.TrimPrefix(cleaned, "/")
}

// matchSegments matches pattern segments against path segments, expanding a
// "**" segment to zero or more path segments via backtracking, and matching
// all other segments with filepath.Match (which itself supports
// single-segment glob syntax like "*", "?", "[...]").
func matchSegments(pattern, segments []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(segments); i++ {
				if matchSegments(pattern[1:], segments[i:]) {
					return true
				}
			}
			return false
		}
		if len(segments) == 0 {
			return false
		}
		matched, err := filepath.Match(pattern[0], segments[0])
		if err != nil || !matched {
			return false
		}
		pattern = pattern[1:]
		segments = segments[1:]
	}
	return len(segments) == 0
}
