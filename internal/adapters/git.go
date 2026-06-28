package adapters

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client wraps git operations (boundary adapter).
type Client struct {
	repoPath string
}

const (
	gitContentionMaxAttempts = 3
	gitContentionBackoff     = 100 * time.Millisecond
)

// New creates a git client for a repository path.
func New(repoPath string) *Client {
	return &Client{repoPath: repoPath}
}

// cmd builds a non-interactive git command rooted at the client's repo path.
// GIT_TERMINAL_PROMPT=0 prevents git from blocking on credential prompts.
func (c *Client) cmd(args ...string) *exec.Cmd {
	fullArgs := append([]string{"-C", c.repoPath}, args...)
	cmd := exec.CommandContext(context.Background(), "git", fullArgs...) //nolint:gosec // G204: internal args, not user input
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
	return strings.TrimSpace(string(output)), nil
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
	rmCmd.Run() //nolint:errcheck,gosec // exit code 1 is expected on empty repo
	commitCmd := c.cmd("commit", "--allow-empty", "-m", "chore: init armature issues branch")
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

// CreateBranchFrom creates a branch from a base branch (with full history).
// If the branch already exists, this is a no-op (idempotent).
// Unlike CreateOrphanBranch, this new branch includes all commits and files from baseBranch.
func (c *Client) CreateBranchFrom(branch, baseBranch string) error {
	// Check if branch already exists — idempotent fast-path
	check := c.cmd("rev-parse", "--verify", "refs/heads/"+branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Create branch from baseBranch using git branch <branch> <baseBranch>
	cmd := c.cmd("branch", branch, baseBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch %s %s: %w\n%s", branch, baseBranch, err, out)
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
	if out, err := c.runMutatingWithRetry("git add "+relPath, "add", relPath); err != nil {
		return fmt.Errorf("%s", enhanceGitLockfileError(
			fmt.Sprintf("git add %s: %v\n%s", relPath, err, out),
			string(out),
		))
	}

	// Check if there is actually something staged
	diff := c.cmd("diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil // nothing staged, no-op
	}

	// Commit
	if out, err := c.runMutatingWithRetry("git commit", "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}

func (c *Client) runMutatingWithRetry(label string, args ...string) ([]byte, error) {
	var lastOut []byte
	var lastErr error
	for attempt := 1; attempt <= gitContentionMaxAttempts; attempt++ {
		cmd := c.cmd(args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return out, nil
		}
		lastOut, lastErr = out, err
		if !isGitContentionError(string(out)) || attempt == gitContentionMaxAttempts {
			break
		}
		time.Sleep(gitContentionBackoff)
	}
	if isGitContentionError(string(lastOut)) {
		return lastOut, fmt.Errorf("%s failed after %d contention retries: %w\n"+
			"Action: another process updated git state concurrently; retry the arm command or run lanes with distinct slots",
			label, gitContentionMaxAttempts, lastErr)
	}
	return lastOut, lastErr
}

func isGitContentionError(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "index.lock") ||
		strings.Contains(lower, "cannot lock ref") ||
		strings.Contains(lower, "another git process seems to be running") ||
		(strings.Contains(lower, "is at") && strings.Contains(lower, "expected"))
}

func enhanceGitLockfileError(base, out string) string {
	// In constrained sandboxes, nested git writes to .git/worktrees/*/index.lock
	// can be denied even when direct top-level git works. Add an actionable hint.
	if strings.Contains(out, "index.lock") && strings.Contains(strings.ToLower(out), "read-only file system") {
		return base + "\nHint: sandbox blocked git lockfile writes (.git/worktrees/*/index.lock). Re-run this arm command with elevated permissions/approval."
	}
	return base
}

// EnhanceGitLockfileErrorForTest exposes lockfile hint behavior to package tests.
func EnhanceGitLockfileErrorForTest(base, out string) string {
	return enhanceGitLockfileError(base, out)
}

// IsGitContentionErrorForTest exposes contention detection behavior to package tests.
func IsGitContentionErrorForTest(out string) bool {
	return isGitContentionError(out)
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

// HeadSHA returns the full SHA of the current HEAD commit.
func (c *Client) HeadSHA() (string, error) {
	cmd := c.cmd("rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoveWorktree removes a linked worktree at the given path. It runs
// "git worktree remove --force <path>" so that it works even if the worktree
// has uncommitted changes. Returns an error if git reports a failure (e.g.
// the path is not a registered worktree).
func (c *Client) RemoveWorktree(path string) error {
	cmd := c.cmd("worktree", "remove", "--force", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove %s: %w\n%s", path, err, out)
	}
	return nil
}

// DiffFrom returns the unified diff between the given base commit and HEAD.
func (c *Client) DiffFrom(baseSHA string) (string, error) {
	cmd := c.cmd("diff", baseSHA, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s HEAD: %w", baseSHA, err)
	}
	return string(out), nil
}

// DiffNameOnly returns the list of file names that differ between the given
// base commit and HEAD.
func (c *Client) DiffNameOnly(baseSHA string) ([]string, error) {
	cmd := c.cmd("diff", "--name-only", baseSHA, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s HEAD: %w", baseSHA, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\n"), nil
}

// ResetHard resets the working tree and index to the given ref (e.g. a SHA or
// branch name).
func (c *Client) ResetHard(ref string) error {
	cmd := c.cmd("reset", "--hard", ref)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s: %w\n%s", ref, err, out)
	}
	return nil
}

// ApplyPatch applies a unified-diff patch to the working tree via
// "git apply". Returns an error if the patch cannot be applied.
func (c *Client) ApplyPatch(patch []byte) error {
	cmd := c.cmd("apply")
	cmd.Stdin = strings.NewReader(string(patch))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply: %w\n%s", err, out)
	}
	return nil
}

// AddAll stages all changes in the working tree (equivalent to "git add -A").
func (c *Client) AddAll() error {
	out, err := c.runMutatingWithRetry("git add -A", "add", "-A")
	if err != nil {
		return fmt.Errorf("git add -A: %w\n%s", err, out)
	}
	return nil
}

// AddPaths stages only the given repository-relative paths.
func (c *Client) AddPaths(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	out, err := c.runMutatingWithRetry("git add -- <paths>", args...)
	if err != nil {
		return fmt.Errorf("git add paths: %w\n%s", err, out)
	}
	return nil
}

// CommitWithMessage creates a commit with the given message. Returns an error
// if there is nothing staged to commit.
func (c *Client) CommitWithMessage(message string) error {
	// Fail fast if nothing is staged
	diff := c.cmd("diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return fmt.Errorf("nothing to commit: index is clean")
	}
	out, err := c.runMutatingWithRetry("git commit", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
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

// ResolveRevision resolves a git revision (ref, SHA, tag, etc.) to its full commit SHA.
func (c *Client) ResolveRevision(rev string) (string, error) {
	cmd := c.cmd("rev-parse", "--verify", rev)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve revision %s: %w", rev, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// DiffRange returns the unified diff between base and head commits using three-dot
// (merge-base) notation. The trailing "--" ensures revisions are not mis-parsed as flags.
func (c *Client) DiffRange(base, head string) (string, error) {
	cmd := c.cmd("diff", base+"..."+head, "--")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to diff range %s...%s: %w", base, head, err)
	}
	return string(output), nil
}

// DiffNameOnlyRange returns the list of changed file names between base and head commits.
// Uses -z (NUL-delimited output) to handle filenames that contain newlines or special
// characters. Three-dot notation and a trailing "--" prevent flag mis-parsing.
func (c *Client) DiffNameOnlyRange(base, head string) ([]string, error) {
	cmd := c.cmd("diff", "--name-only", "-z", base+"..."+head, "--")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff --name-only %s...%s: %w", base, head, err)
	}
	// git -z outputs NUL-terminated entries; trim trailing NUL before splitting.
	raw := strings.TrimRight(string(output), "\x00")
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\x00"), nil
}
