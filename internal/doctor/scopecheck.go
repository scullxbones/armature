package doctor

import (
	"os"
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
// - Only checks against active (claimed/in-progress) or recently-completed (done/merged)
//   tasks, within a grace period (e.g., 30 minutes after completion).
// - For each such task, walks the filesystem and identifies paths that would violate
//   the task's scope globs.
// - Does NOT check the entire filesystem against all tasks (that would be O(n*m) and
//   flag unrelated hygiene issues); instead checks only paths that match the task's
//   scope pattern, looking for both in-scope and out-of-scope variants.
func CheckD8ScopeViolations(index materialize.Index, allIssues map[string]*materialize.Issue, repoPath string, now time.Time) Finding {
	f := Finding{
		Check:    "D8",
		Severity: SeverityOK,
		Message:  "No out-of-scope artifacts detected",
	}

	if _, err := os.Stat(repoPath); err != nil {
		// If the repo path doesn't exist, can't check filesystem; pass through
		return f
	}

	// Collect active and recently-completed tasks
	var tasksToCheck []*materialize.Issue
	gracePeriod := 30 * time.Minute // Recently-completed tasks within 30 minutes

	for id, issue := range allIssues {
		if issue == nil {
			continue
		}

		// Check if it's in the index (to avoid stale issues)
		if _, inIndex := index[id]; !inIndex {
			continue
		}

		// Include active tasks (claimed/in-progress)
		if issue.Status == "claimed" || issue.Status == "in-progress" {
			tasksToCheck = append(tasksToCheck, issue)
			continue
		}

		// Include recently-completed tasks (done/merged within grace period)
		if issue.Status == "done" || issue.Status == "merged" {
			// Use Updated timestamp as proxy for completion time
			if issue.Updated > 0 {
				completedTime := time.Unix(issue.Updated, 0)
				if now.Sub(completedTime) <= gracePeriod {
					tasksToCheck = append(tasksToCheck, issue)
					continue
				}
			}
		}
	}

	// If no tasks to check, return OK
	if len(tasksToCheck) == 0 {
		return f
	}

	// Collect all violations across all tasks
	allViolations := make(map[string][]string) // maps task ID to list of violations
	for _, issue := range tasksToCheck {
		if len(issue.Scope) == 0 {
			continue
		}

		violations := findOutOfScopeArtifacts(repoPath, issue.Scope)
		if len(violations) > 0 {
			allViolations[issue.ID] = violations
		}
	}

	// If there are any violations, report them
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

// findOutOfScopeArtifacts scans the repo filesystem and returns paths that are
// outside the scope, excluding files in non-code directories (like docs, CI config, etc).
func findOutOfScopeArtifacts(repoPath string, scope []string) []string {
	if len(scope) == 0 {
		return nil
	}

	// Create a ScopePolicy to check paths against
	policy := harnesspolicy.NewScopePolicyWithRoot(scope, repoPath)

	// Collect all files that are outside the scope
	var outOfScope []string
	walkErr := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip on error
		}

		// Skip git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip documentation and CI directories
		if info.IsDir() {
			dirName := info.Name()
			if isNonCodeDir(dirName) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories, only check files
		if info.IsDir() {
			return nil
		}

		// Get relative path from repo root
		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}

		// Normalize to forward slashes for scope checking
		rel = filepath.ToSlash(rel)

		// Skip root-level configuration files
		if !strings.Contains(rel, "/") && isConfigFile(filepath.Base(rel)) {
			return nil
		}

		// Check if this file is within scope
		result := policy.CheckPaths([]string{rel})
		if !result.Allowed && len(result.Violations) > 0 {
			// File is outside scope; add to violations
			outOfScope = append(outOfScope, rel)
		}

		return nil
	})

	if walkErr != nil {
		return nil
	}

	if len(outOfScope) > 0 {
		sort.Strings(outOfScope)
	}

	return outOfScope
}

// isNonCodeDir returns true if the directory is known to contain non-code files
// (like documentation, CI configuration, etc) and should be skipped.
func isNonCodeDir(name string) bool {
	nonCodeDirs := map[string]bool{
		"docs":           true,
		"doc":            true,
		".github":        true,
		".gitlab":        true,
		".gitignore":     true,
		"node_modules":   true,
		"venv":           true,
		".venv":          true,
		"__pycache__":    true,
		".pytest_cache":  true,
		".coverage":      true,
		"coverage":       true,
		"vendor":         true,
		"dist":           true,
		"build":          true,
		".build":         true,
		"target":         true,
		".gradle":        true,
	}
	return nonCodeDirs[name]
}

// isConfigFile returns true if the filename is a root-level configuration file
// that should not be checked for scope violations.
func isConfigFile(name string) bool {
	configFiles := map[string]bool{
		"README.md":         true,
		"README.rst":        true,
		"README.txt":        true,
		"LICENSE":           true,
		"COPYING":           true,
		".gitignore":        true,
		".gitattributes":    true,
		".editorconfig":     true,
		"Makefile":          true,
		"Dockerfile":        true,
		"docker-compose.yml": true,
		".travis.yml":       true,
		".github":           true,
		"setup.py":          true,
		"setup.cfg":         true,
		"pyproject.toml":    true,
		"package.json":      true,
		"yarn.lock":         true,
		"pnpm-lock.yaml":    true,
		"go.mod":            true,
		"go.sum":            true,
		"Cargo.toml":        true,
		"Cargo.lock":        true,
		".vscode":           true,
		".idea":             true,
		".env":              true,
		".env.local":        true,
	}
	return configFiles[name]
}
