package harnesspolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ScopePolicy struct {
	scope []string
	root  string
}

type ScopeCheckResult struct {
	Allowed    bool
	EmptyScope bool
	Violations []ScopeViolation
	Scope      []string
}

type ScopeViolation struct {
	Path string
}

func NewScopePolicy(scope []string) ScopePolicy {
	root, _ := os.Getwd() //nolint:errcheck // Getwd failure falls back to empty prefix for path normalization
	return NewScopePolicyWithRoot(scope, root)
}

// NewScopePolicyWithRoot creates a ScopePolicy with an explicit root directory.
// The root is used to normalize absolute paths for scope checking. When root is empty,
// paths are normalized using their relative form or os.Getwd() defaults.
func NewScopePolicyWithRoot(scope []string, root string) ScopePolicy {
	return ScopePolicy{scope: append([]string(nil), scope...), root: root}
}

func (p ScopePolicy) CheckPaths(paths []string) ScopeCheckResult {
	if len(p.scope) == 0 {
		return ScopeCheckResult{
			Allowed:    false,
			EmptyScope: true,
			Scope:      append([]string(nil), p.scope...),
			Violations: violationsForPaths(paths),
		}
	}

	violations := make([]ScopeViolation, 0)
	for _, rawPath := range paths {
		path := p.cleanPath(rawPath)
		if p.allows(path) {
			continue
		}
		violations = append(violations, ScopeViolation{Path: path})
	}

	return ScopeCheckResult{
		Allowed:    len(violations) == 0,
		Scope:      append([]string(nil), p.scope...),
		Violations: violations,
	}
}

func (r ScopeCheckResult) Message() string {
	if r.Allowed {
		return "all paths are within task scope"
	}
	if r.EmptyScope {
		return "task has no declared scope; declare scope before allowing file edits"
	}
	if len(r.Violations) == 0 {
		return "task has no declared scope; declare scope before allowing file edits"
	}

	paths := make([]string, 0, len(r.Violations))
	for _, violation := range r.Violations {
		paths = append(paths, violation.Path)
	}

	scope := strings.Join(r.Scope, ", ")
	if scope == "" {
		scope = "(none)"
	}
	return fmt.Sprintf(
		"path(s) outside task scope: %s; allowed scope: %s",
		strings.Join(paths, ", "),
		scope,
	)
}

func (p ScopePolicy) allows(path string) bool {
	for _, rawScope := range p.scope {
		scope, isDir := cleanScope(rawScope)
		if scope == "." {
			return true
		}
		if path == scope {
			return true
		}
		if isDir && strings.HasPrefix(path, scope+"/") {
			return true
		}
		if strings.Contains(scope, "**") {
			if doublestarMatch(scope, path) {
				return true
			}
			continue
		}
		if strings.ContainsAny(scope, "*?[") {
			matched, err := filepath.Match(scope, path)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

// doublestarMatch reports whether path matches a scope glob pattern containing
// "**" segments, matching path-segment-by-segment (unlike a plain prefix cut,
// which ignores everything after the "**" and so both over-allows, e.g.
// "**/*.go" allowing non-Go files, and under-allows nothing after a suffix,
// e.g. "internal/**/api.go" matching "internal/foo/bar.go"). "**" spans zero
// or more path segments, per the conventional doublestar glob semantics.
func doublestarMatch(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

// matchSegments matches pattern segments against path segments, expanding a
// "**" segment to zero or more path segments via backtracking, and matching
// all other segments with filepath.Match (which itself supports single-segment
// glob syntax like "*", "?", "[...]").
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

func (p ScopePolicy) cleanPath(path string) string {
	if filepath.IsAbs(path) && p.root != "" {
		if rel, err := filepath.Rel(p.root, path); err == nil {
			return cleanRepoPath(rel)
		}
	}
	return cleanRepoPath(path)
}

func violationsForPaths(paths []string) []ScopeViolation {
	violations := make([]ScopeViolation, 0, len(paths))
	for _, path := range paths {
		violations = append(violations, ScopeViolation{Path: cleanRepoPath(path)})
	}
	return violations
}

func cleanScope(raw string) (string, bool) {
	return cleanRepoPath(raw), strings.HasSuffix(filepath.ToSlash(raw), "/")
}

func cleanRepoPath(path string) string {
	slashed := filepath.ToSlash(path)
	trimmed := strings.TrimPrefix(slashed, "./")
	cleaned := filepath.ToSlash(filepath.Clean(trimmed))
	if cleaned == "." {
		return "."
	}
	return strings.TrimPrefix(cleaned, "/")
}
