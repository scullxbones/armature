package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

// Context holds resolved paths and config for the current armature session.
type Context struct {
	RepoPath     string // resolved repo root
	IssuesDir    string // path to issues directory
	WorktreePath string // path to .arm/ worktree; empty in single-branch mode
	StateDir     string // path to runtime state directory
	Mode         string // "single-branch" or "dual-branch"
	Config       Config // loaded from IssuesDir/config.json
}

// isGitWorktree checks if the given path is a git worktree by verifying if .git is a file (not a directory).
// In git worktrees, .git is a file containing "gitdir: <path>".
func isGitWorktree(path string) (bool, error) {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	// If .git is not a directory, it's a worktree
	return !info.IsDir(), nil
}

// resolveParentRepoFromWorktree reads the .git file in a worktree and extracts the parent repo path.
// The .git file contains "gitdir: <gitdir-path>". We resolve parent repo by going up from gitdir to find the .git directory.
func resolveParentRepoFromWorktree(worktreePath string) (string, error) {
	gitFile := filepath.Join(worktreePath, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format, expected 'gitdir: ...'")
	}

	gitdirPath := strings.TrimPrefix(line, "gitdir: ")
	gitdirPath = strings.TrimSpace(gitdirPath)

	// gitdirPath typically points to .git/worktrees/<name>
	// We need to find the parent repo root, which is the directory containing the actual .git directory
	// Go up directories until we find a directory that contains a .git directory (the parent repo's .git)
	current := gitdirPath
	for {
		parent := filepath.Dir(current)
		if parent == current {
			// reached filesystem root without finding parent repo
			return "", fmt.Errorf("could not find parent repo root from gitdir: %s", gitdirPath)
		}

		// Check if parent/.git exists (the actual .git directory of the parent repo)
		potentialGitDir := filepath.Join(parent, ".git")
		if _, err := os.Stat(potentialGitDir); err == nil {
			// Found the parent repo's .git directory, so parent is the repo root
			return parent, nil
		}

		current = parent
	}
}

// ResolveContext reads git config for mode and resolves the issues directory path.
// If invoked from a git worktree, resolves IssuesDir relative to the parent repo root.
func ResolveContext(repoPath string) (*Context, error) {
	// Detect if we're in a git worktree and resolve to parent repo if so
	isWorktree, err := isGitWorktree(repoPath)
	if err != nil {
		return nil, fmt.Errorf("check git worktree: %w", err)
	}

	actualRepoPath := repoPath
	if isWorktree {
		actualRepoPath, err = resolveParentRepoFromWorktree(repoPath)
		if err != nil {
			return nil, fmt.Errorf("resolve parent repo from worktree: %w", err)
		}
	}

	mode, err := adapters.GitConfigMode(actualRepoPath)
	if err != nil {
		return nil, fmt.Errorf("read armature mode: %w", err)
	}

	var issuesDir string
	var worktreePath string
	switch mode {
	case "single-branch":
		issuesDir = filepath.Join(actualRepoPath, ".armature")
	case "dual-branch":
		worktreePath, err = adapters.GitConfig(actualRepoPath, "armature.ops-worktree-path")
		if err != nil {
			return nil, fmt.Errorf("dual-branch mode requires armature.ops-worktree-path to be set: %w", err)
		}
		issuesDir = filepath.Join(worktreePath, ".armature")
	default:
		return nil, fmt.Errorf("unknown armature mode: %q", mode)
	}

	cfg, err := LoadConfig(filepath.Join(issuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &Context{
		RepoPath:     actualRepoPath,
		IssuesDir:    issuesDir,
		WorktreePath: worktreePath,
		Mode:         mode,
		Config:       cfg,
	}, nil
}
