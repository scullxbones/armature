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
	if i := strings.LastIndexByte(glob, '('); i > 0 && strings.HasSuffix(glob, ")") {
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
	if strings.HasSuffix(glob, "/") {
		dirPart := strings.TrimSuffix(glob, "/")
		return file == dirPart || strings.HasPrefix(file, dirPart+"/")
	}

	// If the glob contains /**, it's a directory pattern that covers all subdirectories
	if !strings.Contains(glob, "/**") {
		return false
	}

	// Extract the directory part (before /**)
	dirPart := strings.TrimSuffix(glob, "/**")

	// Check if the file is in this directory or a subdirectory
	return file == dirPart || strings.HasPrefix(file, dirPart+"/")
}
