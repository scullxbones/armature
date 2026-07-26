package harnesspolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/scopematch"
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
	return scopematch.Allows(p.scope, path)
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

func cleanRepoPath(path string) string {
	return scopematch.CleanRepoPath(path)
}
