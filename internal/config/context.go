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

// ResolveContext reads git config for mode and resolves the issues directory path.
func ResolveContext(repoPath string) (*Context, error) {
	mode, err := readGitConfigMode(repoPath)
	if err != nil {
		return nil, fmt.Errorf("read trellis mode: %w", err)
	}

	var issuesDir string
	var worktreePath string
	switch mode {
	case "single-branch":
		issuesDir = filepath.Join(repoPath, ".issues")
	case "dual-branch":
		worktreePath, err = readGitConfig(repoPath, "trellis.ops-worktree-path")
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
		RepoPath:     repoPath,
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
