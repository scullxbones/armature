package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Client wraps git operations (boundary adapter).
type Client struct {
	repoPath string
}

// New creates a git client for a repository path.
func New(repoPath string) *Client {
	return &Client{repoPath: repoPath}
}

// CurrentBranch returns the current git branch name.
func (c *Client) CurrentBranch() (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return string(output[:len(output)-1]), nil // Strip newline
}

// CommitMessage returns the commit message for a given SHA.
func (c *Client) CommitMessage(sha string) (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "log", "-1", "--pretty=%B", sha)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit message for %s: %w", sha, err)
	}
	return string(output), nil
}

// IsCommitOnBranch checks if a commit is reachable on a branch.
func (c *Client) IsCommitOnBranch(sha, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "merge-base", "--is-ancestor", sha, branch)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// Exit code 1 means not an ancestor; other errors are real failures
	if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	}
	return false, fmt.Errorf("failed to check if %s is on %s: %w", sha, branch, err)
}

// CreateOrphanBranch creates an orphan branch (no parent commits) with a single empty commit.
// If the branch already exists, this is a no-op. Always returns to the original branch.
func (c *Client) CreateOrphanBranch(branch string) error {
	// Check if branch already exists — idempotent fast-path
	check := exec.Command("git", "-C", c.repoPath, "rev-parse", "--verify", branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Capture current branch name so we can return to it explicitly
	headCmd := exec.Command("git", "-C", c.repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	priorBranch := strings.TrimSpace(string(headOut))

	// Create orphan branch and make an empty initial commit
	orphanCmd := exec.Command("git", "-C", c.repoPath, "checkout", "--orphan", branch)
	if out, err := orphanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --orphan %s: %w\n%s", branch, err, out)
	}
	// Clear the index; ignore exit code 1 (nothing to remove on an empty repo)
	rmCmd := exec.Command("git", "-C", c.repoPath, "rm", "-rf", "--quiet", ".")
	rmCmd.Run() //nolint:errcheck
	commitCmd := exec.Command("git", "-C", c.repoPath, "commit", "--allow-empty", "-m", "chore: init trellis issues branch")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit on orphan branch: %w\n%s", err, out)
	}

	// Return to the original branch by name (not `checkout -` which may fail on fresh repos)
	restore := exec.Command("git", "-C", c.repoPath, "checkout", priorBranch)
	if out, err := restore.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", priorBranch, err, out)
	}
	return nil
}

// AddWorktree adds a linked worktree for an existing branch at the given path.
// If the worktree already exists at that path (has a .git file), this is a no-op.
func (c *Client) AddWorktree(branch, path string) error {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return nil // already a worktree
	}
	cmd := exec.Command("git", "-C", c.repoPath, "worktree", "add", path, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

// SetGitConfig sets a local git config key to value.
func (c *Client) SetGitConfig(key, value string) error {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "--local", key, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config set %s: %w\n%s", key, err, out)
	}
	return nil
}

// ReadGitConfig reads a local git config key. Returns error if unset.
func (c *Client) ReadGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "-C", c.repoPath, "config", "--local", key)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitWorktreeOp stages and commits a single file change within a worktree.
// The receiver's repoPath must be the worktree root (not the main repo root).
// relPath is relative to the worktree root. If there is nothing to commit, this is a no-op.
func (c *Client) CommitWorktreeOp(relPath, message string) error {
	// Stage the specific file
	add := exec.Command("git", "-C", c.repoPath, "add", relPath)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w\n%s", relPath, err, out)
	}

	// Check if there is actually something staged
	diff := exec.Command("git", "-C", c.repoPath, "diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil // nothing staged, no-op
	}

	// Commit
	commit := exec.Command("git", "-C", c.repoPath, "commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}
