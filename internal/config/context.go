package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

// StateDirName is the name of the state directory (issues directory) within the ops worktree.
const StateDirName = ".armature"

// Context holds resolved paths and config for the current armature session.
type Context struct {
	RepoPath     string
	IssuesDir    string
	WorktreePath string
	StateDir     string
	Config       Config
}

type repoProbeResult struct {
	RepoPath     string
	WorktreePath string
}

func issuesDirFor(worktreePath string) string {
	if filepath.Base(worktreePath) == StateDirName {
		return worktreePath
	}
	return filepath.Join(worktreePath, StateDirName)
}

func resolveIssuesDir(worktreePath string) string {
	nestedStateDir := filepath.Join(worktreePath, StateDirName)
	if info, err := adapters.Stat(nestedStateDir); err == nil && info != nil && info.IsDir() {
		return nestedStateDir
	}

	rootConfig := filepath.Join(worktreePath, "config.json")
	if info, err := adapters.Stat(rootConfig); err == nil && info != nil {
		return worktreePath
	}
	return issuesDirFor(worktreePath)
}

// isGitWorktree checks if the given path is a git worktree by verifying if .git is a file (not a directory).
// In git worktrees, .git is a file containing "gitdir: <path>".
func isGitWorktree(path string) (bool, error) {
	gitPath := filepath.Join(path, ".git")
	info, err := adapters.Stat(gitPath)
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}
	return !info.IsDir(), nil
}

// resolveParentRepoFromWorktree reads the .git file in a worktree and extracts the parent repo path.
// The .git file contains "gitdir: <gitdir-path>". We resolve parent repo by going up from gitdir to find the .git directory.
func resolveParentRepoFromWorktree(worktreePath string) (string, error) {
	gitFile := filepath.Join(worktreePath, ".git")
	content, err := adapters.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format, expected 'gitdir: ...'")
	}

	gitdirPath := strings.TrimPrefix(line, "gitdir: ")
	gitdirPath = strings.TrimSpace(gitdirPath)

	current := gitdirPath
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("could not find parent repo root from gitdir: %s", gitdirPath)
		}

		potentialGitDir := filepath.Join(parent, ".git")
		info, err := adapters.Stat(potentialGitDir)
		if err == nil && info != nil {
			return parent, nil
		}

		current = parent
	}
}

// ResolveLayout locates the ops worktree and issues directory without loading
// config.json. Use this when config decode failed but the modern worktree is
// still the right place to diagnose or repair.
func ResolveLayout(repoPath string) (*Context, error) {
	probeResult, err := defaultRepoProbe{}.probe(repoPath)
	if err != nil {
		return nil, err
	}
	if probeResult.WorktreePath == "" {
		return nil, fmt.Errorf("armature.ops-worktree-path must be set")
	}
	return &Context{
		RepoPath:     probeResult.RepoPath,
		IssuesDir:    resolveIssuesDir(probeResult.WorktreePath),
		WorktreePath: probeResult.WorktreePath,
	}, nil
}

// ResolveContext resolves the issues directory path from the ops worktree.
// It requires armature.ops-worktree-path to be set and returns a clear error if unset.
func ResolveContext(repoPath string) (*Context, error) {
	ctx, err := ResolveLayout(repoPath)
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfig(filepath.Join(ctx.IssuesDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	ctx.Config = cfg
	return ctx, nil
}

// DetectUnmigratedLayout checks if the issues directory is nested below the ops
// worktree, which identifies the old dual-branch layout regardless of a custom
// ops worktree path.
func DetectUnmigratedLayout(worktreePath, issuesDir string) bool {
	return filepath.Clean(worktreePath) != filepath.Clean(issuesDir)
}

type defaultRepoProbe struct{}

func (defaultRepoProbe) probe(repoPath string) (repoProbeResult, error) {
	isWorktree, err := isGitWorktree(repoPath)
	if err != nil {
		return repoProbeResult{}, fmt.Errorf("check git worktree: %w", err)
	}

	actualRepoPath := repoPath
	if isWorktree {
		actualRepoPath, err = resolveParentRepoFromWorktree(repoPath)
		if err != nil {
			return repoProbeResult{}, fmt.Errorf("resolve parent repo from worktree: %w", err)
		}
	}

	worktreePath, err := adapters.GitConfig(actualRepoPath, "armature.ops-worktree-path")
	if err != nil {
		if info, statErr := os.Stat(actualRepoPath); statErr != nil || info == nil || !info.IsDir() {
			return repoProbeResult{}, err
		}
		return repoProbeResult{}, fmt.Errorf("armature.ops-worktree-path must be set: %w", err)
	}

	return repoProbeResult{
		RepoPath:     actualRepoPath,
		WorktreePath: worktreePath,
	}, nil
}
