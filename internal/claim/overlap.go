package claim

import (
	"path/filepath"
	"slices"
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

// IsWithinScope checks if all files in the provided list are within the
// declared scope globs. It returns (true, "") if all files are in scope,
// or (false, "filename") if a file is outside scope.
// An empty files list is considered within any scope.
func IsWithinScope(files, scope []string) (bool, string) {
	if len(files) == 0 {
		return true, ""
	}

	for _, file := range files {
		inScope := false
		for _, rawGlob := range scope {
			glob := stripScopeAnnotation(rawGlob)
			// Try matching the file against this scope glob
			if matched, _ := filepath.Match(glob, file); matched { //nolint:errcheck // ErrBadPattern unreachable for valid scope paths
				inScope = true
				break
			}

			// Also check if the glob matches the file's directory structure
			// This handles cases like "internal/claim/**" matching "internal/claim/sub/file.go"
			if globCoversFile(glob, file) {
				inScope = true
				break
			}
		}

		if !inScope {
			return false, file
		}
	}

	return true, ""
}

// stripScopeAnnotation removes a trailing human-readable annotation like
// " (new)" that workers commonly append to scope entries when declaring a
// file that doesn't exist yet (e.g. "internal/foo/bar.go (new)"). Scope
// entries are stored verbatim including this annotation, so any exact-path
// matcher must strip it before comparing against real file paths.
func stripScopeAnnotation(glob string) string {
	if i := strings.LastIndex(glob, " ("); i >= 0 && strings.HasSuffix(glob, ")") {
		return strings.TrimSpace(glob[:i])
	}
	return glob
}

// globCoversFile checks if a glob pattern covers a file path by examining
// directory hierarchy. This handles patterns like "dir/**" that should match
// files in subdirectories, and a trailing-slash directory scope like "dir/"
// (no "/**" suffix), which by convention also means "recursively, everything
// under dir/" — consistent with internal/harnesspolicy/scope.go's cleanScope,
// which treats any scope entry ending in "/" as a recursive directory scope.
func globCoversFile(glob, file string) bool {
	// A trailing-slash directory scope (e.g. "internal/") covers everything
	// under that directory, at any depth.
	if dirPart, ok := strings.CutSuffix(glob, "/"); ok {
		return file == dirPart || strings.HasPrefix(file, dirPart+"/")
	}

	// A "**" segment anywhere in the pattern (not just as a trailing "/**"
	// suffix) matches zero or more path segments, per standard doublestar
	// semantics. This handles both a trailing "dir/**" (matching everything
	// under dir) and a mid-pattern "**" like "internal/**/api.go" (matching
	// internal/api.go, internal/foo/api.go, internal/foo/bar/api.go, etc.) —
	// consistent with internal/harnesspolicy/scope.go's doublestarMatch.
	if strings.Contains(glob, "**") {
		return doublestarMatch(glob, file)
	}

	return false
}

// doublestarMatch reports whether file matches a glob pattern containing
// "**" segments, matching path-segment-by-segment so that "**" spans zero or
// more path segments (unlike filepath.Match, which treats "**" as a literal
// "*" within a single segment and so cannot match across a "/"). Ported from
// internal/harnesspolicy/scope.go's doublestarMatch/matchSegments: the two
// packages must not import each other (harnesspolicy sits below claim in the
// dependency graph), so the segment-aware matching logic is duplicated here
// rather than shared via an import.
func doublestarMatch(glob, file string) bool {
	return matchGlobSegments(strings.Split(glob, "/"), strings.Split(file, "/"))
}

// matchGlobSegments matches glob path segments against file path segments,
// expanding a "**" segment to zero or more path segments via backtracking,
// and matching all other segments with filepath.Match.
func matchGlobSegments(pattern, segments []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			if len(pattern) == 1 {
				return true
			}
			for i := 0; i <= len(segments); i++ {
				if matchGlobSegments(pattern[1:], segments[i:]) {
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
