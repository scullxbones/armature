package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Client wraps git operations (boundary adapter).
type Client struct {
	repoPath   string
	isolateEnv bool
}

const (
	gitContentionMaxAttempts = 3
	gitContentionBackoff     = 100 * time.Millisecond
)

// New creates a git client for a repository path.
func New(repoPath string) *Client {
	return &Client{repoPath: repoPath}
}

// NewIsolated is like New but the child git process binds to the checkout
// at repoPath: env overrides (GIT_DIR / GIT_WORK_TREE / …) are stripped and
// --git-dir/--work-tree are pinned from the filesystem .git (not
// core.worktree or rev-parse --show-toplevel). Used by arm gate run.
func NewIsolated(repoPath string) *Client {
	return &Client{repoPath: repoPath, isolateEnv: true}
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
	prefix := []string{"-C", c.repoPath, "-c", "maintenance.auto=false", "-c", "gc.auto=0"}
	if c.isolateEnv {
		if gitDir, workTree, err := resolveIsolatedGitDirs(c.repoPath); err == nil {
			prefix = []string{
				"--git-dir=" + gitDir,
				"--work-tree=" + workTree,
				"-C", c.repoPath,
				"-c", "maintenance.auto=false",
				"-c", "gc.auto=0",
			}
		}
	}
	prefix = append(prefix, args...)
	cmd := exec.CommandContext(ctx, "git", prefix...) //nolint:gosec // G204: internal args, not user input
	env := os.Environ()
	if c.isolateEnv {
		env = stripGitOverrideEnv(env)
	}
	env = append(env,
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_ASKPASS=true",
	)
	cmd.Env = env
	return cmd
}

var gitOverrideEnvPrefixes = []string{
	"GIT_DIR=",
	"GIT_WORK_TREE=",
	"GIT_INDEX_FILE=",
	"GIT_OBJECT_DIRECTORY=",
	"GIT_COMMON_DIR=",
}

func resolveIsolatedGitDirs(start string) (gitDir, workTree string, err error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	cur := abs
	for {
		gd, wt, ok, walkErr := gitDirAt(cur)
		if walkErr != nil {
			return "", "", walkErr
		}
		if ok {
			return gd, wt, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", fmt.Errorf("no .git above %s", abs)
		}
		cur = parent
	}
}

func gitDirAt(dir string) (gitDir, workTree string, ok bool, err error) {
	p := filepath.Join(dir, ".git")
	info, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	if info.IsDir() {
		return p, dir, true, nil
	}
	data, err := os.ReadFile(p) //nolint:gosec // path is dir/.git
	if err != nil {
		return "", "", false, err
	}
	line := strings.TrimSpace(string(data))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false, fmt.Errorf("invalid .git file at %s", p)
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if raw == "" {
		return "", "", false, fmt.Errorf("empty gitdir in %s", p)
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(dir, raw)
	}
	return filepath.Clean(raw), dir, true, nil
}

func stripGitOverrideEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		drop := false
		for _, p := range gitOverrideEnvPrefixes {
			if strings.HasPrefix(e, p) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, e)
		}
	}
	return out
}

// Toplevel returns the top-level working directory of the git repository (or
// worktree) containing the client's repoPath, via `git rev-parse
// --show-toplevel`. Unlike a manual .git-file stat, this resolves correctly
// even when repoPath is a subdirectory of the worktree: git itself walks up
// parent directories to find the enclosing repository, so this works from
// any nesting depth.
func (c *Client) Toplevel() (string, error) {
	cmd := c.cmd("rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve worktree top level: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
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

// MergeBase returns the SHA of the merge-base between two revisions.
func (c *Client) MergeBase(rev1, rev2 string) (string, error) {
	cmd := c.cmd("merge-base", rev1, rev2)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find merge-base of %s and %s: %w", rev1, rev2, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// RevListCount returns the number of commits reachable from `to` but not
// from `from` (i.e. `git rev-list --count from..to`). Used to prove
// non-divergence: a count of 0 means `to` has not moved past `from`.
func (c *Client) RevListCount(from, to string) (int, error) {
	cmd := c.cmd("rev-list", "--count", from+".."+to)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to count commits %s..%s: %w", from, to, err)
	}
	count, convErr := strconv.Atoi(strings.TrimSpace(string(output)))
	if convErr != nil {
		return 0, fmt.Errorf("failed to parse rev-list count output %q: %w", output, convErr)
	}
	return count, nil
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
// opposed to a tracked file with staged or unstaged changes. For a staged
// rename, OldPath holds the source path (Path is always the destination);
// OldPath is empty for all other statuses.
type DirtyEntry struct {
	Path      string
	OldPath   string
	Untracked bool
	Ignored   bool
}

// DirtyEntries returns every working-tree change, tracked or untracked
// (unlike IsWorkingTreeDirty, which treats untracked files as never dirty).
// Callers that need to classify dirty paths against an allow-list — e.g.
// reconciling known-safe debris before refusing a dirty worktree outright —
// need the Untracked flag too, since tolerating incidental untracked
// scaffolding while still refusing on tracked changes is exactly
// IsWorkingTreeDirty's existing contract; this exposes the same information
// per-path instead of collapsing it to a single bool. Renamed paths report
// the destination path in Path and the source path in OldPath, so a caller
// checking a rename against a boundary (e.g. a scope or state directory)
// can inspect both sides rather than only seeing the destination. Returns an
// empty (nil) slice for a clean working tree.
func (c *Client) DirtyEntries() ([]DirtyEntry, error) {
	return c.dirtyEntries("status", "--porcelain", "--ignored")
}

// DirtyEntriesIncludingSubmodules is like DirtyEntries but does not honor
// submodule.<name>.ignore, status.ignoreSubmodules, or
// status.showUntrackedFiles. `-c diff.ignoreSubmodules=none` is not enough:
// `git status` still hides dirty submodule worktrees under those settings.
// `--ignore-submodules=none` and `--untracked-files=all` are the status
// flags that surface them.
func (c *Client) DirtyEntriesIncludingSubmodules() ([]DirtyEntry, error) {
	return c.dirtyEntriesRecursive("")
}

func (c *Client) dirtyEntriesRecursive(prefix string) ([]DirtyEntry, error) {
	entries, err := c.dirtyEntries("-c", "diff.ignoreSubmodules=none", "status", "--porcelain", "--ignored", "--ignore-submodules=none", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	if prefix != "" {
		for i := range entries {
			entries[i].Path = filepath.Join(prefix, entries[i].Path)
			if entries[i].OldPath != "" {
				entries[i].OldPath = filepath.Join(prefix, entries[i].OldPath)
			}
		}
	}
	gitlinks, err := c.gitlinkPaths()
	if err != nil {
		return nil, err
	}
	for _, gl := range gitlinks {
		subDir := filepath.Join(c.repoPath, gl)
		childPrefix := gl
		if prefix != "" {
			childPrefix = filepath.Join(prefix, gl)
		}
		if !populatedSubmodule(subDir) {
			if nonEmptyDir(subDir) {
				entries = append(entries, DirtyEntry{Path: childPrefix})
			}
			continue
		}
		inner, err := c.child(subDir).dirtyEntriesRecursive(childPrefix)
		if err != nil {
			return nil, err
		}
		entries = append(entries, inner...)
	}
	return entries, nil
}

func (c *Client) dirtyEntries(args ...string) ([]DirtyEntry, error) {
	cmd := c.cmd(args...)
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
		// Porcelain v1 format: "XY PATH" or "XY ORIG_PATH -> PATH" for renames
		// (rename status "R" appears at either index 0 or 1 depending on
		// whether the rename is staged, unstaged, or both).
		rest := line[3:]
		path := rest
		oldPath := ""
		if idx := strings.Index(rest, " -> "); idx >= 0 {
			oldPath = rest[:idx]
			path = rest[idx+len(" -> "):]
		}
		entries = append(entries, DirtyEntry{
			Path:      path,
			OldPath:   oldPath,
			Untracked: strings.HasPrefix(line, "??"),
			Ignored:   strings.HasPrefix(line, "!!"),
		})
	}
	return entries, nil
}

// IndexConcealmentEntries returns tracked paths whose index flags hide
// worktree mutations from `git status`: skip-worktree (tag S) or
// assume-unchanged (lowercase tag under `ls-files -v`). Populated
// submodules are walked so an inner skip-worktree file is not invisible
// behind a superproject gitlink.
func (c *Client) IndexConcealmentEntries() ([]string, error) {
	return c.indexConcealmentRecursive("")
}

func (c *Client) indexConcealmentRecursive(prefix string) ([]string, error) {
	cmd := c.cmd("ls-files", "-v")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files -v: %w", err)
	}
	var paths []string
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 3 || line[1] != ' ' {
			continue
		}
		tag := line[0]
		if tag != 'S' && (tag < 'a' || tag > 'z') {
			continue
		}
		rel := line[2:]
		if prefix != "" {
			rel = filepath.Join(prefix, rel)
		}
		paths = append(paths, rel)
	}
	gitlinks, err := c.gitlinkPaths()
	if err != nil {
		return nil, err
	}
	for _, gl := range gitlinks {
		subDir := filepath.Join(c.repoPath, gl)
		if !populatedSubmodule(subDir) {
			continue
		}
		sub := c.child(subDir)
		childPrefix := gl
		if prefix != "" {
			childPrefix = filepath.Join(prefix, gl)
		}
		inner, err := sub.indexConcealmentRecursive(childPrefix)
		if err != nil {
			return nil, err
		}
		paths = append(paths, inner...)
	}
	return paths, nil
}

func (c *Client) gitlinkPaths() ([]string, error) {
	cmd := c.cmd("ls-files", "-s")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files -s: %w", err)
	}
	var paths []string
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if !strings.HasPrefix(line, "160000 ") {
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab < 0 || tab+1 >= len(line) {
			continue
		}
		paths = append(paths, line[tab+1:])
	}
	return paths, nil
}

func populatedSubmodule(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info != nil
}

func nonEmptyDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.Open(dir) //nolint:gosec // path is a gitlink checkout
	if err != nil {
		return false
	}
	names, err := f.Readdirnames(1)
	closeErr := f.Close()
	return err == nil && closeErr == nil && len(names) > 0
}

func (c *Client) child(repoPath string) *Client {
	return &Client{repoPath: repoPath, isolateEnv: c.isolateEnv}
}

// CheckIgnoreSource reports whether path is ignored and, if so, the
// check-ignore -v source (e.g. ".gitignore" or an absolute exclude file).
func (c *Client) CheckIgnoreSource(path string) (string, bool, error) {
	cmd := c.cmd("check-ignore", "-v", "--no-index", "--", path)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git check-ignore: %w", err)
	}
	return parseCheckIgnoreSource(string(out)), true, nil
}

func parseCheckIgnoreSource(raw string) string {
	line := strings.TrimSpace(raw)
	if i := strings.IndexByte(line, '\t'); i >= 0 {
		line = line[:i]
	}
	for i := 0; i < len(line)-2; i++ {
		if line[i] != ':' || line[i+1] < '0' || line[i+1] > '9' {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] >= '0' && line[j] <= '9' {
			j++
		}
		if j < len(line) && line[j] == ':' {
			return line[:i]
		}
	}
	return line
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
	// Parents holds the SHAs of this commit's parent commits, from git log's
	// %P. Empty for the repository's root commit, one entry for an ordinary
	// commit, two or more for a genuine merge commit.
	Parents []string
}

// ParentCount returns the number of parent commits this entry has. A
// genuine merge commit has 2 or more; an ordinary commit has exactly 1; a
// root commit has 0. Used to distinguish a real merge commit from an
// ordinary commit whose subject merely happens to look like one (e.g. a
// hand-written "merge: ID description" subject on a single-parent commit).
func (e LogEntry) ParentCount() int {
	return len(e.Parents)
}

// LogRange returns log entries reachable from head but not from base
// (`git log base..head`), most recent first.
func (c *Client) LogRange(base, head string) ([]LogEntry, error) {
	format := "%H%x00%s%x00%ae%x00%ai%x00%P"
	cmd := c.cmd("log", base+".."+head, "--format="+format, "--")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log %s..%s: %w", base, head, err)
	}
	return parseLogOutput(out), nil
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
	format := "%H%x00%s%x00%ae%x00%ai%x00%P"
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
	return parseLogOutput(out), nil
}

// parseLogOutput parses NUL-delimited
// `git log --format=%H%x00%s%x00%ae%x00%ai%x00%P` output. The trailing %P
// field is space-separated parent SHAs and may be empty (root commit).
func parseLogOutput(out []byte) []LogEntry {
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []LogEntry{}
	}
	lines := strings.Split(raw, "\n")
	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x00")
		if len(parts) != 5 {
			continue
		}
		var parents []string
		if parts[4] != "" {
			parents = strings.Fields(parts[4])
		}
		entries = append(entries, LogEntry{
			SHA:     parts[0],
			Subject: parts[1],
			Author:  parts[2],
			Date:    parts[3],
			Parents: parents,
		})
	}
	return entries
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

// RemoveWorktree removes a linked worktree at the given path without forcing
// deletion. Git therefore rejects dirty tracked or untracked content, which
// preserves work until an operator deliberately resolves it.
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
	cmd := c.cmd("worktree", "remove", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove %s: %w\n%s", path, err, out)
	}
	return nil
}

// RemovePartiallyProvisionedWorktree forcibly removes the exact worktree path
// that claim created while rolling back a failed provision. Lifecycle teardown
// must use RemoveWorktree so it cannot discard worker changes.
func (c *Client) RemovePartiallyProvisionedWorktree(path string) error {
	cmd := c.cmd("worktree", "remove", "--force", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("force-remove partial worktree %s: %w\n%s", path, err, out)
	}
	return nil
}

// DiffStatusEntry represents one line of `git diff --name-status` output.
// Status is the raw git status code (e.g. "A", "M", "D", or "R100" for a
// rename with a 100% similarity score). For renames, OldPath is the source
// path and Path is the destination path; for all other statuses OldPath is
// empty.
type DiffStatusEntry struct {
	Status  string
	Path    string
	OldPath string
}

// DiffNameStatus returns the list of file changes between the given base
// commit and HEAD, with rename detection enabled (-M) so that renamed files
// report BOTH their source and destination paths. This is important for scope
// checks: `git diff --name-only` collapses a rename to only its destination
// path, which can hide that the file's original location was out of scope.
func (c *Client) DiffNameStatus(baseSHA string) ([]DiffStatusEntry, error) {
	// -z switches git to NUL-delimited, unquoted output: without it, git
	// quotes and octal-escapes any path containing non-ASCII or special
	// characters (e.g. "caf\303\251.go" instead of the literal "café.go"),
	// which breaks scope-containment comparisons against the literal path.
	// Mirrors DiffNameOnlyRange, which already handles this correctly.
	cmd := c.cmd("diff", "--name-status", "-M", "-z", baseSHA, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status -M %s HEAD: %w", baseSHA, err)
	}
	return parseNameStatusZ(out), nil
}

// parseNameStatusZ parses the NUL-delimited output shared by `git diff
// --name-status -z` and `git diff-tree --name-status -z`: a flat sequence of
// NUL-terminated fields (no per-line \t separator): status, path, status,
// path, ... or for a rename/copy: status, oldpath, newpath, status, path, ...
func parseNameStatusZ(out []byte) []DiffStatusEntry {
	raw := strings.TrimRight(string(out), "\x00")
	if raw == "" {
		return []DiffStatusEntry{}
	}
	fields := strings.Split(raw, "\x00")
	entries := make([]DiffStatusEntry, 0, len(fields))
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if i+1 >= len(fields) {
				break
			}
			entries = append(entries, DiffStatusEntry{Status: status, OldPath: fields[i], Path: fields[i+1]})
			i += 2
			continue
		}
		if i >= len(fields) {
			break
		}
		entries = append(entries, DiffStatusEntry{Status: status, Path: fields[i]})
		i++
	}
	return entries
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

// CommitChangedFiles returns the file paths touched by a single commit,
// diffed against its first parent (`git diff-tree --no-commit-id --name-only
// -M -r <sha>`). Rename detection (-M) is enabled for consistency with
// DiffNameStatus: without it, a pure rename is reported as a delete of the
// old path plus an add of the new path instead of a single renamed entry.
// With --name-only specifically (unlike --name-status), git does not emit a
// separate old-path line or similarity-score prefix for a detected rename —
// it simply reports the new (post-image) path alone, so no output-parsing
// change is needed here to accommodate -M; verified against actual git
// behavior, not assumed. Returns an empty (non-nil) slice for a no-op/empty
// commit (e.g. one created with `git commit --allow-empty`), which callers
// use to distinguish a commit with real content from one that only satisfies
// a message-shape check.
func (c *Client) CommitChangedFiles(sha string) ([]string, error) {
	cmd := c.cmd("diff-tree", "--no-commit-id", "--name-only", "-M", "-r", sha)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree --no-commit-id --name-only -M -r %s: %w", sha, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\n"), nil
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
