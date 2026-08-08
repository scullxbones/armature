// Package worktree provides managed worktree reconciliation and lifecycle.
package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectType represents the type of project detected in a repository.
type ProjectType string

const (
	ProjectTypeGo      ProjectType = "go"
	ProjectTypeUnknown ProjectType = "unknown"
)

// Mitigation represents a specific worktree mitigation to apply.
type Mitigation string

const (
	MitigationGoWorkIsolation Mitigation = "go-work-isolation"
)

// DetectProjectType detects the project type based on files present in the repo root.
// Returns ProjectTypeGo if either go.mod or go.work is present, ProjectTypeUnknown otherwise.
// This is a pure function that does not shell out.
func DetectProjectType(repoRoot string) ProjectType {
	// Check for go.mod
	goModPath := filepath.Join(repoRoot, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		return ProjectTypeGo
	}

	// Check for go.work
	goWorkPath := filepath.Join(repoRoot, "go.work")
	if _, err := os.Stat(goWorkPath); err == nil {
		return ProjectTypeGo
	}

	return ProjectTypeUnknown
}

// GetMitigationsForProjectType returns the list of mitigations applicable to a project type.
// For Go projects, returns MitigationGoWorkIsolation to isolate the worktree's Go workspace
// from the main tree's workspace, preventing gopls from getting confused about module boundaries.
// For unknown project types, returns an empty list.
func GetMitigationsForProjectType(projType ProjectType) []Mitigation {
	switch projType {
	case ProjectTypeGo:
		return []Mitigation{MitigationGoWorkIsolation}
	default:
		return []Mitigation{}
	}
}

// ApplyMitigations applies all applicable mitigations for the detected project type.
// For Go projects, it creates a go.work file in the worktree to isolate it from the main
// tree's workspace. This prevents gopls in the IDE from treating the worktree as part of
// a larger workspace and getting confused about module boundaries.
// The mitigation is idempotent: if a go.work already exists in the worktree, it is
// preserved and not overwritten.
func ApplyMitigations(repoRoot, worktreeRoot string) error {
	// Detect the project type
	projType := DetectProjectType(repoRoot)

	// Get applicable mitigations
	mitigations := GetMitigationsForProjectType(projType)

	// Apply each mitigation
	for _, mitigation := range mitigations {
		if mitigation == MitigationGoWorkIsolation {
			if err := applyGoWorkIsolation(repoRoot, worktreeRoot); err != nil {
				return fmt.Errorf("apply go-work-isolation: %w", err)
			}
		}
	}

	return nil
}

// applyGoWorkIsolation ensures the worktree has a go.work file that isolates it
// from the main tree's workspace. Creates a minimal go.work in the worktree if it
// doesn't already exist, preserving any existing go.work (idempotent).
func applyGoWorkIsolation(repoRoot, worktreeRoot string) error {
	// Ensure the worktree directory exists
	// #nosec G301 - this is an internal worktree directory, not user-controlled
	if err := os.MkdirAll(worktreeRoot, 0o700); err != nil {
		return fmt.Errorf("create worktree directory: %w", err)
	}

	worktreeGoWork := filepath.Join(worktreeRoot, "go.work")

	// Check if go.work already exists in the worktree
	if _, err := os.Stat(worktreeGoWork); err == nil {
		// go.work already exists, preserve it (idempotent)
		return nil
	}

	// Determine the Go version to use in the go.work file
	// Read it from the repo root's go.mod or go.work if available
	goVersion := determineGoVersion(repoRoot)

	// Create a minimal go.work file in the worktree
	// This declares the worktree as a standalone workspace, independent of the main tree
	goWorkContent := fmt.Sprintf("go %s\n", goVersion)

	// #nosec G306 - this is an internal worktree file, not user-controlled
	if err := os.WriteFile(worktreeGoWork, []byte(goWorkContent), 0o600); err != nil {
		return fmt.Errorf("write go.work: %w", err)
	}

	return nil
}

// determineGoVersion extracts the Go version from the repo root's go.mod or go.work.
// Returns a sensible default if neither file is found or the version cannot be determined.
func determineGoVersion(repoRoot string) string {
	for _, name := range []string{"go.mod", "go.work"} {
		// #nosec G304 - repoRoot is internal, paths are not user-controlled
		content, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			continue
		}
		if version := parseGoVersion(string(content)); version != "" {
			return version
		}
	}
	// Default to a conservative version
	return "1.20"
}

// parseGoVersion extracts the version from the first "go <version>" directive in
// go.mod/go.work content, stripping any trailing line comment. Returns "" if absent.
func parseGoVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "go ")
		if !ok {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = rest[:i]
		}
		if version := strings.TrimSpace(rest); version != "" {
			return version
		}
	}
	return ""
}
