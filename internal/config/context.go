package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context holds resolved paths and config for the current trellis session.
type Context struct {
	RepoPath     string // resolved repo root
	IssuesDir    string // path to issues directory
	WorktreePath string // path to .trellis/ worktree; empty in single-branch mode
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

	mode, err := readGitConfigMode(actualRepoPath)
	if err != nil {
		return nil, fmt.Errorf("read trellis mode: %w", err)
	}

	var issuesDir string
	var worktreePath string
	switch mode {
	case "single-branch":
		issuesDir = filepath.Join(actualRepoPath, ".issues")
	case "dual-branch":
		worktreePath, err = readGitConfig(actualRepoPath, "trellis.ops-worktree-path")
		if err != nil {
			return nil, fmt.Errorf("dual-branch mode requires trellis.ops-worktree-path to be set: %w", err)
		}
		issuesDir = filepath.Join(worktreePath, ".issues")
	default:
		return nil, fmt.Errorf("unknown trellis mode: %q", mode)
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

// nonInteractiveGitCmd builds a git command with GIT_TERMINAL_PROMPT=0 to prevent
// blocking on credential prompts. Note: intentionally does not use git.Client to avoid circular imports.
func nonInteractiveGitCmd(repoPath string, args ...string) *exec.Cmd {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_ASKPASS=true")
	return cmd
}

// readGitConfig reads a single local git config key. Returns error if unset.
// Note: intentionally does not use git.Client to avoid circular imports.
func readGitConfig(repoPath, key string) (string, error) {
	cmd := nonInteractiveGitCmd(repoPath, "config", "--local", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// readGitConfigMode reads trellis.mode from git config. Returns "single-branch" if unset.
func readGitConfigMode(repoPath string) (string, error) {
	cmd := nonInteractiveGitCmd(repoPath, "config", "trellis.mode")
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 means key not set — default to single-branch
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "single-branch", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
