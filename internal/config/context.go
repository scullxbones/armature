package config

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context holds resolved paths and config for the current trellis session.
type Context struct {
	RepoPath  string // resolved repo root
	IssuesDir string // path to issues directory
	Mode      string // "single-branch" or "dual-branch"
	Config    Config // loaded from IssuesDir/config.json
}

// ResolveContext reads git config for mode and resolves the issues directory path.
func ResolveContext(repoPath string) (*Context, error) {
	mode, err := readGitConfigMode(repoPath)
	if err != nil {
		return nil, fmt.Errorf("read trellis mode: %w", err)
	}

	var issuesDir string
	switch mode {
	case "single-branch":
		issuesDir = filepath.Join(repoPath, ".issues")
	case "dual-branch":
		return nil, errors.New("dual-branch mode not yet implemented")
	default:
		return nil, fmt.Errorf("unknown trellis mode: %q", mode)
	}

	cfg, err := LoadConfig(filepath.Join(issuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return &Context{
		RepoPath:  repoPath,
		IssuesDir: issuesDir,
		Mode:      mode,
		Config:    cfg,
	}, nil
}

// readGitConfigMode reads trellis.mode from git config. Returns "single-branch" if unset.
func readGitConfigMode(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "config", "trellis.mode")
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
