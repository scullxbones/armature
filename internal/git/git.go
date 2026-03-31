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

// cmd builds a non-interactive git command rooted at the client's repo path.
// GIT_TERMINAL_PROMPT=0 prevents git from blocking on credential prompts.
func (c *Client) cmd(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-C", c.repoPath}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_ASKPASS=true")
	return cmd
}

// CurrentBranch returns the current git branch name.
func (c *Client) CurrentBranch() (string, error) {
	cmd := c.cmd("rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return string(output[:len(output)-1]), nil // Strip newline
}

// CommitMessage returns the commit message for a given SHA.
func (c *Client) CommitMessage(sha string) (string, error) {
	cmd := c.cmd("log", "-1", "--pretty=%B", sha)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit message for %s: %w", sha, err)
	}
	return string(output), nil
}

// IsCommitOnBranch checks if a commit is reachable on a branch.
func (c *Client) IsCommitOnBranch(sha, branch string) (bool, error) {
	cmd := c.cmd("merge-base", "--is-ancestor", sha, branch)
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
	check := c.cmd("rev-parse", "--verify", branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Capture current branch name so we can return to it explicitly
	headCmd := c.cmd("rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	priorBranch := strings.TrimSpace(string(headOut))

	// Create orphan branch and make an empty initial commit
	orphanCmd := c.cmd("checkout", "--orphan", branch)
	if out, err := orphanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --orphan %s: %w\n%s", branch, err, out)
	}
	// Clear the index; ignore exit code 1 (nothing to remove on an empty repo)
	rmCmd := c.cmd("rm", "-rf", "--quiet", ".")
	rmCmd.Run() //nolint:errcheck
	commitCmd := c.cmd("commit", "--allow-empty", "-m", "chore: init trellis issues branch")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit on orphan branch: %w\n%s", err, out)
	}

	// Return to the original branch by name (not `checkout -` which may fail on fresh repos)
	restore := c.cmd("checkout", priorBranch)
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
	cmd := c.cmd("worktree", "add", path, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

// SetGitConfig sets a local git config key to value.
func (c *Client) SetGitConfig(key, value string) error {
	cmd := c.cmd("config", "--local", key, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config set %s: %w\n%s", key, err, out)
	}
	return nil
}

// ReadGitConfig reads a local git config key. Returns error if unset.
func (c *Client) ReadGitConfig(key string) (string, error) {
	cmd := c.cmd("config", "--local", key)
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
	add := c.cmd("add", relPath)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w\n%s", relPath, err, out)
	}

	// Check if there is actually something staged
	diff := c.cmd("diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil // nothing staged, no-op
	}

	// Commit
	commit := c.cmd("commit", "-m", message)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}

// Push pushes the current branch to origin. Returns an error if the push is
// rejected (e.g. non-fast-forward).
func (c *Client) Push(branch string) error {
	cmd := c.cmd("push", "origin", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push origin %s: %w\n%s", branch, err, out)
	}
	return nil
}

// FetchAndRebase fetches from origin and rebases the local branch onto the
// remote tracking branch. This is used to resolve push rejections.
func (c *Client) FetchAndRebase(branch string) error {
	fetch := c.cmd("fetch", "origin")
	if out, err := fetch.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch origin: %w\n%s", err, out)
	}
	rebase := c.cmd("rebase", "origin/"+branch)
	if out, err := rebase.CombinedOutput(); err != nil {
		return fmt.Errorf("git rebase origin/%s: %w\n%s", branch, err, out)
	}
	return nil
}

// LogEntry represents a single git log entry.
type LogEntry struct {
	SHA     string
	Subject string
	Author  string
	Date    string
}

// ListFilesAtCommit returns the list of file paths tracked at the given commit SHA.
func (c *Client) ListFilesAtCommit(sha string) ([]string, error) {
	cmd := c.cmd("ls-tree", "-r", "--name-only", sha)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s: %w", sha, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\n"), nil
}

// ShowFileAtCommit returns the contents of the file at path as it existed at the given commit SHA.
func (c *Client) ShowFileAtCommit(sha, path string) ([]byte, error) {
	cmd := c.cmd("show", sha+":"+path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return out, nil
}

// LogBranch returns up to n log entries from the tip of branch, most recent first.
func (c *Client) LogBranch(branch string, n int) ([]LogEntry, error) {
	format := "%H%x00%s%x00%ae%x00%ai"
	cmd := c.cmd("log", branch, fmt.Sprintf("-n%d", n), "--format="+format)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", branch, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []LogEntry{}, nil
	}
	lines := strings.Split(raw, "\n")
	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x00")
		if len(parts) != 4 {
			continue
		}
		entries = append(entries, LogEntry{
			SHA:     parts[0],
			Subject: parts[1],
			Author:  parts[2],
			Date:    parts[3],
		})
	}
	return entries, nil
}

// BranchMergedInto checks if branch has been fully merged into target.
// Returns (false, nil) if the branch does not exist, rather than an error.
func (c *Client) BranchMergedInto(branch, target string) (bool, error) {
	// Check that branch exists
	check := c.cmd("rev-parse", "--verify", branch)
	if err := check.Run(); err != nil {
		return false, nil // branch doesn't exist
	}

	// Get the tip commit of branch
	tip := c.cmd("rev-parse", branch)
	tipOut, err := tip.Output()
	if err != nil {
		return false, fmt.Errorf("rev-parse %s: %w", branch, err)
	}
	sha := strings.TrimSpace(string(tipOut))

	return c.IsCommitOnBranch(sha, target)
}
