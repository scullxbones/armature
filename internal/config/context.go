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
	RepoPath       string
	IssuesDir      string
	WorktreePath   string
	StateDir       string
	Config         Config
	WorkerIDFlag   string
	InvocationPath string
}

type invokingWorktreePath string
type mainGitDir string

func (p invokingWorktreePath) String() string { return string(p) }
func (d mainGitDir) String() string           { return string(d) }

func (c *Context) invokingWorktree() invokingWorktreePath {
	if c == nil {
		return ""
	}
	if c.InvocationPath != "" {
		return invokingWorktreePath(c.InvocationPath)
	}
	return invokingWorktreePath(c.RepoPath)
}

func (c *Context) gitDir() mainGitDir {
	if c == nil {
		return ""
	}
	return mainGitDir(c.RepoPath)
}

type repoProbeResult struct {
	gitDir      mainGitDir
	opsWorktree string
	invoking    invokingWorktreePath
}

func nestedStateDir(worktreePath string) (string, bool) {
	dir := filepath.Join(worktreePath, StateDirName)
	info, err := adapters.Stat(dir)
	return dir, err == nil && info != nil && info.IsDir()
}

func hasCollapsedRootConfig(worktreePath string) bool {
	info, err := adapters.Stat(filepath.Join(worktreePath, "config.json"))
	return err == nil && info != nil
}

func issuesDirFor(worktreePath string) string {
	if filepath.Base(worktreePath) == StateDirName {
		return worktreePath
	}
	return filepath.Join(worktreePath, StateDirName)
}

func resolveIssuesDir(worktreePath string) string {
	if dir, ok := nestedStateDir(worktreePath); ok {
		return dir
	}
	if hasCollapsedRootConfig(worktreePath) {
		return worktreePath
	}
	return issuesDirFor(worktreePath)
}

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
	invoking := invokingWorktreePath(repoPath)
	probeResult, err := defaultRepoProbe{}.probe(invoking)
	if err != nil {
		return nil, err
	}
	if probeResult.opsWorktree == "" {
		return nil, fmt.Errorf("armature.ops-worktree-path must be set")
	}
	return &Context{
		RepoPath:       probeResult.gitDir.String(),
		IssuesDir:      resolveIssuesDir(probeResult.opsWorktree),
		WorktreePath:   probeResult.opsWorktree,
		InvocationPath: probeResult.invoking.String(),
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

func lookupMainGitDir(invoking invokingWorktreePath) (mainGitDir, error) {
	path := invoking.String()
	isWorktree, err := isGitWorktree(path)
	if err != nil {
		return "", fmt.Errorf("check git worktree: %w", err)
	}
	if !isWorktree {
		return mainGitDir(path), nil
	}
	parent, err := resolveParentRepoFromWorktree(path)
	if err != nil {
		return "", fmt.Errorf("resolve parent repo from worktree: %w", err)
	}
	return mainGitDir(parent), nil
}

func (defaultRepoProbe) probe(invoking invokingWorktreePath) (repoProbeResult, error) {
	gitDir, err := lookupMainGitDir(invoking)
	if err != nil {
		return repoProbeResult{}, err
	}

	worktreePath, err := adapters.GitConfig(gitDir.String(), "armature.ops-worktree-path")
	if err != nil {
		path := gitDir.String()
		if info, statErr := os.Stat(path); statErr != nil || info == nil || !info.IsDir() {
			return repoProbeResult{}, err
		}
		return repoProbeResult{}, fmt.Errorf("armature.ops-worktree-path must be set: %w", err)
	}

	return repoProbeResult{
		gitDir:      gitDir,
		opsWorktree: worktreePath,
		invoking:    invoking,
	}, nil
}
