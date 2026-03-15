package git

import (
	"fmt"
	"os/exec"
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
