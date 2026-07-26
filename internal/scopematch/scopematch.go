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
	for _, rawScope := range scope {
		cleaned, isDir := CleanScope(rawScope)
		if cleaned == "." {
			return true
		}
		if cleanedPath == cleaned {
			return true
		}
		if isDir && strings.HasPrefix(cleanedPath, cleaned+"/") {
			return true
		}
		if strings.Contains(cleaned, "**") {
			if doublestarMatch(cleaned, cleanedPath) {
				return true
			}
			continue
		}
		if strings.ContainsAny(cleaned, "*?[") {
			matched, err := filepath.Match(cleaned, cleanedPath)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

// CleanScope normalizes a raw scope entry and reports whether it denotes a
// directory scope (i.e. had a trailing slash before normalization), which
// covers everything at or under that directory regardless of whether the
// entry also contains "**".
func CleanScope(raw string) (string, bool) {
	return CleanRepoPath(raw), strings.HasSuffix(filepath.ToSlash(raw), "/")
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

// doublestarMatch reports whether path matches a scope glob pattern
// containing "**" segments, matching path-segment-by-segment (unlike a
// plain prefix cut, which ignores everything after the "**" and so both
// over-allows, e.g. "**/*.go" allowing non-Go files, and under-allows
// nothing after a suffix, e.g. "internal/**/api.go" matching
// "internal/foo/bar.go"). "**" spans zero or more path segments, per the
// conventional doublestar glob semantics.
func doublestarMatch(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
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
