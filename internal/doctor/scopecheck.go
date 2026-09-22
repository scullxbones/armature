package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/materialize"
)

// CheckD8ScopeViolations checks for out-of-scope artifacts on disk that correlate with
// active or recently-completed tasks. It uses ScopePolicy.CheckPaths to detect any untracked
// or modified paths outside a task's declared scope glob.
//
// The check is designed to catch stray binaries and other artifacts that escaped scope
// enforcement at commit time. It does NOT flag general main-worktree hygiene unrelated
// to a task's scope.
//
// Scope:
//   - Only checks against active (claimed/in-progress) or recently-completed (done/merged)
//     tasks, within a grace period (e.g., 30 minutes after completion).
//   - For each such task, walks the filesystem and identifies paths that would violate
//     the task's scope globs.
//   - Does NOT check the entire filesystem against all tasks (that would be O(n*m) and
//     flag unrelated hygiene issues); instead checks only paths that match the task's
//     scope pattern, looking for both in-scope and out-of-scope variants.
func CheckD8ScopeViolations(index materialize.Index, allIssues map[string]*materialize.Issue, repoPath string, now time.Time) Finding {
	f := Finding{
		Check:    "D8",
		Severity: SeverityOK,
		Message:  "No out-of-scope artifacts detected",
	}

	if _, err := os.Stat(repoPath); err != nil {
		return f
	}

	var tasksToCheck []*materialize.Issue
	gracePeriod := 30 * time.Minute

	for id, issue := range allIssues {
		if issue == nil {
			continue
		}

		if _, inIndex := index[id]; !inIndex {
			continue
		}

		if issue.Status == "claimed" || issue.Status == "in-progress" {
			tasksToCheck = append(tasksToCheck, issue)
			continue
		}

		if issue.Status == "done" || issue.Status == "merged" {
			if issue.Updated > 0 {
				completedTime := time.Unix(issue.Updated, 0)
				if now.Sub(completedTime) <= gracePeriod {
					tasksToCheck = append(tasksToCheck, issue)
					continue
				}
			}
		}
	}

	if len(tasksToCheck) == 0 {
		return f
	}

	rawViolations := make(map[string][]string)
	for _, issue := range tasksToCheck {
		if len(issue.Scope) == 0 {
			continue
		}

		violations := findOutOfScopeArtifacts(repoPath, issue.Scope)
		if len(violations) > 0 {
			rawViolations[issue.ID] = violations
		}
	}

	explainedBy := make(map[string][]*materialize.Issue)
	for _, issue := range tasksToCheck {
		if len(issue.Scope) == 0 {
			continue
		}
		policy := harnesspolicy.NewScopePolicyWithRoot(issue.Scope, repoPath)
		for taskID := range rawViolations {
			for _, path := range rawViolations[taskID] {
				if result := policy.CheckPaths([]string{path}); result.Allowed {
					explainedBy[path] = append(explainedBy[path], issue)
				}
			}
		}
	}

	allViolations := make(map[string][]string)
	for taskID, violations := range rawViolations {
		for _, path := range violations {
			if len(explainedBy[path]) > 0 {
				continue
			}
			allViolations[taskID] = append(allViolations[taskID], path)
		}
	}

	if len(allViolations) > 0 {
		f.Severity = SeverityError
		f.Message = "Out-of-scope artifacts detected for active or recently-completed tasks"

		for taskID, violations := range allViolations {
			for _, v := range violations {
				f.Items = append(f.Items, taskID+": "+v)
			}
		}
		sort.Strings(f.Items)
	}

	return f
}

func findOutOfScopeArtifacts(repoPath string, scope []string) []string {
	if len(scope) == 0 {
		return nil
	}

	candidates := gitDirtyPaths(repoPath)
	if len(candidates) == 0 {
		return nil
	}

	policy := harnesspolicy.NewScopePolicyWithRoot(scope, repoPath)

	var outOfScope []string
	for _, rel := range candidates {
		rel = filepath.ToSlash(rel)

		if isNonCodeDir(topLevelDir(rel)) {
			continue
		}

		if !strings.Contains(rel, "/") && isConfigFile(filepath.Base(rel)) {
			continue
		}

		result := policy.CheckPaths([]string{rel})
		if !result.Allowed && len(result.Violations) > 0 {
			outOfScope = append(outOfScope, rel)
		}
	}

	if len(outOfScope) > 0 {
		sort.Strings(outOfScope)
	}

	return outOfScope
}

func topLevelDir(rel string) string {
	if idx := strings.Index(rel, "/"); idx >= 0 {
		return rel[:idx]
	}
	return ""
}

func gitDirtyPaths(repoPath string) []string {
	// #nosec G204 - repoPath is a caller-supplied trusted repo/worktree path
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "status", "--porcelain", "--untracked-files=all")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		// Porcelain format: "XY <path>" or "XY <path> -> <newpath>" for renames.
		entry := line[3:]
		if idx := strings.Index(entry, " -> "); idx >= 0 {
			entry = entry[idx+4:]
		}
		entry = strings.Trim(entry, "\"")
		if entry != "" {
			paths = append(paths, entry)
		}
	}
	return paths
}

func isNonCodeDir(name string) bool {
	nonCodeDirs := map[string]bool{
		"docs":          true,
		"doc":           true,
		".github":       true,
		".gitlab":       true,
		".gitignore":    true,
		"node_modules":  true,
		"venv":          true,
		".venv":         true,
		"__pycache__":   true,
		".pytest_cache": true,
		".coverage":     true,
		"coverage":      true,
		"vendor":        true,
		"dist":          true,
		"build":         true,
		".build":        true,
		"target":        true,
		".gradle":       true,
	}
	return nonCodeDirs[name]
}

func isConfigFile(name string) bool {
	configFiles := map[string]bool{
		"README.md":          true,
		"README.rst":         true,
		"README.txt":         true,
		"LICENSE":            true,
		"COPYING":            true,
		".gitignore":         true,
		".gitattributes":     true,
		".editorconfig":      true,
		"Makefile":           true,
		"Dockerfile":         true,
		"docker-compose.yml": true,
		".travis.yml":        true,
		".github":            true,
		"setup.py":           true,
		"setup.cfg":          true,
		"pyproject.toml":     true,
		"package.json":       true,
		"yarn.lock":          true,
		"pnpm-lock.yaml":     true,
		"go.mod":             true,
		"go.sum":             true,
		"Cargo.toml":         true,
		"Cargo.lock":         true,
		".vscode":            true,
		".idea":              true,
		".env":               true,
		".env.local":         true,
	}
	return configFiles[name]
}
