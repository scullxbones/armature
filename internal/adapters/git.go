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
	return c.cmdContext(context.Background(), args...)
}

// cmdContext is like cmd but binds the command to the given context, so
// callers can bound long-running or network-touching commands (e.g. fetch)
// with a timeout.
func (c *Client) cmdContext(ctx context.Context, args ...string) *exec.Cmd {
	// maintenance.auto=false (and gc.auto=0 for older git) prevent git from
	// forking "git maintenance run --auto --detach" on commit-like commands.
	// That detached process can outlive this git invocation and still be
	// writing under .git when a caller (e.g. a test's t.TempDir() cleanup)
	// tries to remove the repo, causing intermittent "directory not empty"
	// failures.
	fullArgs := append([]string{"-C", c.repoPath, "-c", "maintenance.auto=false", "-c", "gc.auto=0"}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...) //nolint:gosec // G204: internal args, not user input
	cmd.Env = append(os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_ASKPASS=true",
	)
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

// IsWorkingTreeDirty checks if the working tree has uncommitted changes to tracked files.
// Returns true if there are modified tracked files or staged changes, false if clean.
// Untracked files are ignored (only tracked file changes count as "dirty").
func (c *Client) IsWorkingTreeDirty() (bool, error) {
	// Check for modified/staged tracked files using git status.
	// The --porcelain output includes lines starting with the status codes:
	// - First char: index status (M, D, A, etc. or space if no staged change)
	// - Second char: working tree status (M, D, etc. or space if no modification)
	// - Lines starting with ??: untracked (ignored, not dirty)
	// Any line NOT starting with ?? means a tracked file change
	cmd := c.cmd("status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Skip untracked files (start with ??)
		if strings.HasPrefix(line, "??") {
			continue
		}
		// Any other output means tracked files have changes
		return true, nil
	}
	return false, nil
}

// DirtyEntry is a single parsed line of `git status --porcelain` output: the
// repo-relative path and whether it is untracked (status code "??") as
// opposed to a tracked file with staged or unstaged changes.
type DirtyEntry struct {
	Path      string
	Untracked bool
}

// DirtyEntries returns every working-tree change, tracked or untracked
// (unlike IsWorkingTreeDirty, which treats untracked files as never dirty).
// Callers that need to classify dirty paths against an allow-list — e.g.
// reconciling known-safe debris before refusing a dirty worktree outright —
// need the Untracked flag too, since tolerating incidental untracked
// scaffolding while still refusing on tracked changes is exactly
// IsWorkingTreeDirty's existing contract; this exposes the same information
// per-path instead of collapsing it to a single bool. Renamed paths report
// the destination path. Returns an empty (nil) slice for a clean working tree.
func (c *Client) DirtyEntries() ([]DirtyEntry, error) {
	cmd := c.cmd("status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	var entries []DirtyEntry
	// Only trim the trailing newline here, not leading whitespace: the porcelain
	// status code's first column is often a literal space (e.g. " M file.txt"
	// for an unstaged modification), and strings.TrimSpace on the whole blob
	// would eat that space off the very first line, shifting every subsequent
	// index-based slice by one and corrupting the parsed path.
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" || len(line) < 4 {
			continue
		}
		// Porcelain v1 format: "XY PATH" or "XY ORIG_PATH -> PATH" for renames.
		path := line[3:]
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+len(" -> "):]
		}
		entries = append(entries, DirtyEntry{Path: path, Untracked: strings.HasPrefix(line, "??")})
	}
	return entries, nil
}

// isBenignEmptyRepoRmError reports whether output from `git rm -rf --quiet .`
// reflects the expected, harmless failure on an empty repo (no tracked files
// to remove) rather than a real error that could leave stale index entries.
func isBenignEmptyRepoRmError(output []byte) bool {
	return strings.Contains(string(output), "did not match any files")
}

// CreateOrphanBranch creates an orphan branch (no parent commits) with a single empty commit.
// If the branch already exists locally, this is a no-op.
// If the branch exists on origin but not locally, creates a local tracking branch from origin.
// Otherwise, creates a new orphan branch with an empty commit.
// Always returns to the original branch. Fails with an error if the working tree is dirty.
func (c *Client) CreateOrphanBranch(branch string) error {
	// Check if branch already exists locally — idempotent fast-path
	check := c.cmd("rev-parse", "--verify", branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Attempt to fetch the branch from origin in case it exists on the remote
	// but not in the local remote-tracking refs (e.g., after git clone --single-branch).
	// Fetch with refspec to create/update the remote-tracking ref.
	// This is best-effort; ignore failures when the remote is absent, offline, or the branch doesn't exist.
	// "origin" is hardcoded here, matching the existing pattern elsewhere in this
	// client (Push/FetchAndRebase); this assumes a single remote named "origin".
	// Bounded with a short timeout so a black-holed network can't hang bootstrap.
	fetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fetchCmd := c.cmdContext(fetchCtx, "fetch", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch)
	_ = fetchCmd.Run() //nolint:errcheck // Ignore fetch errors; best-effort fetch for remote branch

	// Check if the branch exists on origin and create a local tracking branch if so
	remoteBranch := "origin/" + branch
	remoteCheck := c.cmd("rev-parse", "--verify", remoteBranch)
	if err := remoteCheck.Run(); err == nil {
		// Remote branch exists; create a local tracking branch from it
		createCmd := c.cmd("branch", branch, remoteBranch)
		if out, err := createCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git branch %s from %s: %w\n%s", branch, remoteBranch, err, out)
		}
		return nil
	}

	// Check if working tree is dirty before doing anything destructive
	dirty, err := c.IsWorkingTreeDirty()
	if err != nil {
		return fmt.Errorf("check working tree: %w", err)
	}
	if dirty {
		return fmt.Errorf("working tree is dirty (contains uncommitted changes): please commit or stash your changes before running bootstrap")
	}

	// Capture current branch name so we can return to it explicitly.
	// On detached HEAD, --abbrev-ref returns the literal string "HEAD".
	// In that case, capture the concrete commit SHA instead so we can restore.
	headCmd := c.cmd("rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	priorBranch := strings.TrimSpace(string(headOut))

	// If in detached HEAD, capture the SHA to restore to, not the "HEAD" literal
	if priorBranch == "HEAD" {
		shaCmd := c.cmd("rev-parse", "HEAD")
		shaOut, err := shaCmd.Output()
		if err != nil {
			return fmt.Errorf("get current commit SHA: %w", err)
		}
		priorBranch = strings.TrimSpace(string(shaOut))
	}

	// Create orphan branch and make an empty initial commit
	orphanCmd := c.cmd("checkout", "--orphan", branch)
	if out, err := orphanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --orphan %s: %w\n%s", branch, err, out)
	}
	// Clear the index. On an empty repo there's nothing tracked to remove and
	// git reports "pathspec '.' did not match any files" — that's expected and
	// safe to ignore. Any other failure means the index may still hold stale
	// entries, which would get committed onto the new orphan branch, so treat
	// it as fatal and restore the prior branch.
	rmCmd := c.cmd("rm", "-rf", "--quiet", ".")
	if rmOut, rmErr := rmCmd.CombinedOutput(); rmErr != nil && !isBenignEmptyRepoRmError(rmOut) {
		restore := c.cmd("checkout", priorBranch)
		restoreErr := restore.Run()
		if restoreErr != nil {
			return fmt.Errorf("git rm -rf . on orphan branch failed: %w; then failed to restore to %s: %w\n%s", rmErr, priorBranch, restoreErr, rmOut)
		}
		return fmt.Errorf("git rm -rf . on orphan branch: %w\n%s", rmErr, rmOut)
	}
	commitCmd := c.cmd("commit", "--no-verify", "--allow-empty", "-m", "chore: init armature issues branch")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		// Commit failed; attempt to restore to the prior branch before returning error
		restore := c.cmd("checkout", priorBranch)
		restoreErr := restore.Run()
		if restoreErr != nil {
			// Restore failed; include both errors in the message
			return fmt.Errorf("git commit on orphan branch failed: %w; then failed to restore to %s: %w\n%s", err, priorBranch, restoreErr, out)
		}
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
// A live worktree at that path is a no-op. A stale .git pointer left behind by a
// failed rollback is removed so Git can register a usable replacement.
func (c *Client) AddWorktree(branch, path string) error {
	gitFile := filepath.Join(path, ".git")
	if _, err := os.Stat(gitFile); err == nil {
		check := c.cmd("-C", path, "rev-parse", "--is-inside-work-tree")
		if err := check.Run(); err == nil {
			return nil // already a usable worktree
		}
		if err := os.Remove(gitFile); err != nil {
			return fmt.Errorf("remove stale worktree pointer %s: %w", gitFile, err)
		}
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

// UnsetGitConfig removes a local git config key. It is a no-op (returns nil)
// if the key is not currently set, so callers can use it to opportunistically
// clear stale config without needing to check existence first.
func (c *Client) UnsetGitConfig(key string) error {
	cmd := c.cmd("config", "--local", "--unset", key)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 5 {
		return nil // git config --unset exits 5 when the key was never set
	}
	return fmt.Errorf("git config unset %s: %w\n%s", key, err, out)
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
// A non-positive n returns all entries.
func (c *Client) LogBranch(branch string, n int) ([]LogEntry, error) {
	if strings.HasPrefix(branch, "-") {
		return nil, fmt.Errorf("invalid branch %q", branch)
	}
	format := "%H%x00%s%x00%ae%x00%ai"
	args := []string{"log", branch, "--format=" + format}
	if n > 0 {
		args = append(args, fmt.Sprintf("-n%d", n))
	}
	// TEST_EXCEPTION: disambiguates branch from a path when a branch name
	// collides with a file/directory path in the repo (git would otherwise
	// error "ambiguous argument"). Not covered by a dedicated test: exercising
	// it requires contriving a directory that shadows a branch name, which
	// isn't a proportionate amount of test scaffolding for a one-line fix.
	args = append(args, "--")
	cmd := c.cmd(args...)
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
// MoveWorktree relocates a linked worktree's directory and updates git's worktree
// registration atomically via `git worktree move`. Unlike a manual rename paired
// with RemoveWorktree/AddWorktree, this cannot leave a partially-registered
// worktree behind: if it fails, the worktree remains fully valid at its original
// path, so rollback is simply calling MoveWorktree again with the paths reversed.
func (c *Client) MoveWorktree(oldPath, newPath string) error {
	cmd := c.cmd("worktree", "move", oldPath, newPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree move %s %s: %w\n%s", oldPath, newPath, err, out)
	}
	return nil
}

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

// RemoveTree removes a path from both the git index and the working tree using
// "git rm -r". Unlike RemoveFromIndex (which uses --cached to preserve the
// working-tree copy), this deletes the files on disk too — used to clear a
// stale tracked subtree whose contents have already been copied elsewhere.
// Returns nil (no-op) if the path is not tracked.
func (c *Client) RemoveTree(path string) error {
	cmd := c.cmd("rm", "-r", "--quiet", "--", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "did not match any files") ||
			strings.Contains(string(out), "pathspec") {
			return nil
		}
		return fmt.Errorf("git rm -r %s: %w\n%s", path, err, out)
	}
	return nil
}

// RemoveFromIndex removes a path from the git index using "git rm --cached".
// This is used to mark tracked files/directories for deletion without deleting
// the working tree files. Returns nil if the path is not tracked.
func (c *Client) RemoveFromIndex(path string) error {
	cmd := c.cmd("rm", "-r", "--cached", "--quiet", path)
	_ = cmd.Run() //nolint:errcheck // path might not be tracked in git
	return nil
}

// IsTracked checks if a path is tracked by git (exists in the index).
// Returns true if the path is tracked, false otherwise.
func (c *Client) IsTracked(path string) bool {
	cmd := c.cmd("ls-files", path)
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

// CommitPaths creates a commit scoped to the given pathspecs, so it structurally cannot
// sweep in unrelated staged changes outside those paths. If there is nothing staged for
// the given paths, this is a no-op (returns nil) rather than an error. Any other commit
// failure (hook rejection, missing git identity, etc.) is returned as an error.
func (c *Client) CommitPaths(message string, paths ...string) error {
	return c.commitPaths(message, false, paths...)
}

// CommitPathsNoVerify behaves like CommitPaths but skips hooks with --no-verify.
func (c *Client) CommitPathsNoVerify(message string, paths ...string) error {
	return c.commitPaths(message, true, paths...)
}

func (c *Client) commitPaths(message string, noVerify bool, paths ...string) error {
	args := append([]string{"commit", "-m", message, "--"}, paths...)
	if noVerify {
		args = append([]string{"commit", "--no-verify", "-m", message, "--"}, paths...)
	}
	cmd := c.cmd(args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "nothing to commit") {
		return nil
	}
	return fmt.Errorf("git commit: %w\n%s", err, out)
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
