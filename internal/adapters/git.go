package adapters

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/scullxbones/armature/internal/gitoid"
)

type Client struct {
	repoPath       string
	isolateEnv     bool
	environ        []string
	networkTimeout time.Duration
}

const (
	gitContentionMaxAttempts = 3
	gitContentionBackoff     = 100 * time.Millisecond

	// defaultGitNetworkTimeout bounds short remote probes (ops-backup,
	// FetchBranchToRef, CreateOrphanBranch's single-ref fetch) so a stalled
	// origin fail-closes instead of hanging the CLI.
	defaultGitNetworkTimeout = 15 * time.Second

	// defaultGitPublishNetworkTimeout bounds Push and FetchAndRebase's
	// `git fetch origin`. Those transfer whatever history origin has; the 15s
	// probe budget would fail a healthy slow link. Thirty minutes still
	// fail-closes a blackholed origin and stays above a normal push or fetch
	// that succeeds today. WithNetworkTimeout overrides it.
	defaultGitPublishNetworkTimeout = 30 * time.Minute

	// gitNetworkWaitDelay bounds how long Wait may block on stuck pipes after
	// Cancel kills the network git process group (git-remote-http children).
	gitNetworkWaitDelay = 1 * time.Second

	// armatureTmpNS holds per-call restore-plan staging refs inside the user's
	// clone. Network ops run here so git resolves clone config natively; the
	// namespace is never refs/remotes/origin/* or a user branch.
	armatureTmpNS = "refs/armature-tmp"
)

func New(repoPath string) *Client {
	return &Client{repoPath: repoPath}
}

// NewIsolated is like New but the child git process binds to the checkout
// at repoPath: env overrides (GIT_DIR / GIT_WORK_TREE / …) are stripped and
// --git-dir/--work-tree are pinned from the filesystem .git (not
// core.worktree or rev-parse --show-toplevel).
func NewIsolated(repoPath string) *Client {
	return &Client{repoPath: repoPath, isolateEnv: true}
}

// WithEnviron overlays the git child environment (including PATH for LookPath).
// A nil/empty slice leaves the process environment in place.
func (c *Client) WithEnviron(environ []string) *Client {
	if c == nil {
		return nil
	}
	cp := *c
	cp.environ = append([]string(nil), environ...)
	return &cp
}

// WithNetworkTimeout sets the deadline for network git calls (ops-backup remote
// probes, FetchBranchToRef, Push, FetchAndRebase). Zero restores each call's
// default (probe budget or the longer publish budget).
func (c *Client) WithNetworkTimeout(d time.Duration) *Client {
	if c == nil {
		return nil
	}
	cp := *c
	cp.networkTimeout = d
	return &cp
}

func (c *Client) effectiveNetworkTimeout() time.Duration {
	if c != nil && c.networkTimeout > 0 {
		return c.networkTimeout
	}
	return defaultGitNetworkTimeout
}

func (c *Client) cmd(args ...string) *exec.Cmd {
	return c.cmdContext(context.Background(), args...)
}

// CommandGit builds a git command. A nil/empty environ, or an environ whose
// PATH matches the process PATH, uses exec.Command("git") (ErrDot protected).
// Custom PATH resolution runs only when PATH is an explicit overlay.
func CommandGit(ctx context.Context, environ []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: git plus caller-built args
	if len(environ) == 0 {
		return cmd
	}
	cmd.Env = append([]string(nil), environ...)
	path := envValFold(environ, "PATH")
	if path == "" || path == envValFold(os.Environ(), "PATH") {
		return cmd
	}
	applyOverlayLookPath(cmd, environ, path)
	return cmd
}

func applyOverlayLookPath(cmd *exec.Cmd, environ []string, pathList string) {
	lp, err := lookPathOnConfig(pathList, "git", lookPathConfig{
		windows: runtime.GOOS == "windows",
		pathext: envValFold(environ, "PATHEXT"),
	})
	if lp != "" {
		cmd.Path = lp
	}
	if err != nil {
		cmd.Err = err
	}
}

// GIT_TERMINAL_PROMPT=0 prevents git from blocking on credential prompts.
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
	cmd := CommandGit(ctx, c.environ, prefix...)
	env := cmd.Env
	if len(env) == 0 {
		env = os.Environ()
	} else {
		env = append([]string(nil), env...)
	}
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

func gitOverrideEnvPrefixes() []string {
	return []string{
		"GIT_DIR=",
		"GIT_WORK_TREE=",
		"GIT_INDEX_FILE=",
		"GIT_OBJECT_DIRECTORY=",
		"GIT_COMMON_DIR=",
	}
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
	data, err := readStateFile(p)
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
		for _, p := range gitOverrideEnvPrefixes() {
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

func (c *Client) CurrentBranch() (string, error) {
	cmd := c.cmd("rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) MergeBase(rev1, rev2 string) (string, error) {
	cmd := c.cmd("merge-base", rev1, rev2)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find merge-base of %s and %s: %w", rev1, rev2, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsWorkingTreeDirty checks if the working tree has uncommitted changes to tracked files.
// Returns true if there are modified tracked files or staged changes, false if clean.
// Untracked files are ignored (only tracked file changes count as "dirty").
func (c *Client) IsWorkingTreeDirty() (bool, error) {
	entries, err := c.DirtyEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.Untracked && !entry.Ignored {
			return true, nil
		}
	}
	return false, nil
}

type DirtyEntry struct {
	Path      string
	OldPath   string
	Untracked bool
	Ignored   bool
}

func (c *Client) DirtyEntries() ([]DirtyEntry, error) {
	return c.dirtyEntries("status", "--porcelain", "--ignored")
}

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
	if err := c.eachGitlink(prefix, func(sub *Client, childPrefix, subDir string) error {
		if !populatedSubmodule(subDir) {
			if nonEmptyDir(subDir) {
				entries = append(entries, DirtyEntry{Path: childPrefix})
			}
			return nil
		}
		inner, err := sub.dirtyEntriesRecursive(childPrefix)
		if err != nil {
			return err
		}
		entries = append(entries, inner...)
		return nil
	}); err != nil {
		return nil, err
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
	if err := c.eachGitlink(prefix, func(sub *Client, childPrefix, subDir string) error {
		if !populatedSubmodule(subDir) {
			return nil
		}
		inner, err := sub.indexConcealmentRecursive(childPrefix)
		if err != nil {
			return err
		}
		paths = append(paths, inner...)
		return nil
	}); err != nil {
		return nil, err
	}
	return paths, nil
}

func (c *Client) eachGitlink(prefix string, visit func(sub *Client, childPrefix, subDir string) error) error {
	gitlinks, err := c.gitlinkPaths()
	if err != nil {
		return err
	}
	for _, gl := range gitlinks {
		childPrefix := gl
		if prefix != "" {
			childPrefix = filepath.Join(prefix, gl)
		}
		subDir := filepath.Join(c.repoPath, gl)
		if err := visit(c.child(subDir), childPrefix, subDir); err != nil {
			return err
		}
	}
	return nil
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
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func nonEmptyDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := openState(dir)
	if err != nil {
		return false
	}
	names, err := f.Readdirnames(1)
	closeErr := f.Close()
	return err == nil && closeErr == nil && len(names) > 0
}

func (c *Client) child(repoPath string) *Client {
	return &Client{repoPath: repoPath, isolateEnv: c.isolateEnv, environ: c.environ}
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

func isBenignEmptyRepoRmError(output []byte) bool {
	return strings.Contains(string(output), "did not match any files")
}

// CreateOrphanBranch creates an orphan branch (no parent commits) with a single empty commit.
// If the branch already exists locally, this is a no-op.
// If the branch exists on origin but not locally, creates a local tracking branch from origin.
// Otherwise, creates a new orphan branch with an empty commit.
func (c *Client) CreateOrphanBranch(branch string) error {
	check := c.cmd("rev-parse", "--verify", branch)
	if err := check.Run(); err == nil {
		return nil
	}

	// Fetch after git clone --single-branch so origin/<branch> exists locally.
	// Failure is non-fatal: the orphan path below still runs.
	fetchCmd, _, cancelFetch := c.networkCmd("fetch", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch)
	defer cancelFetch()
	if fetchErr := fetchCmd.Run(); fetchErr != nil {
		// Origin can be missing on a fresh repo; the orphan path below still runs.
		cancelFetch()
	}

	remoteBranch := "origin/" + branch
	remoteCheck := c.cmd("rev-parse", "--verify", remoteBranch)
	if err := remoteCheck.Run(); err == nil {
		createCmd := c.cmd("branch", branch, remoteBranch)
		if out, err := createCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git branch %s from %s: %w\n%s", branch, remoteBranch, err, out)
		}
		return nil
	}

	dirty, err := c.IsWorkingTreeDirty()
	if err != nil {
		return fmt.Errorf("check working tree: %w", err)
	}
	if dirty {
		return fmt.Errorf("working tree is dirty (contains uncommitted changes): please commit or stash your changes before running bootstrap")
	}

	// On detached HEAD, --abbrev-ref returns the literal string "HEAD".
	headCmd := c.cmd("rev-parse", "--abbrev-ref", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}
	priorBranch := strings.TrimSpace(string(headOut))

	if priorBranch == "HEAD" {
		shaCmd := c.cmd("rev-parse", "HEAD")
		shaOut, err := shaCmd.Output()
		if err != nil {
			return fmt.Errorf("get current commit SHA: %w", err)
		}
		priorBranch = strings.TrimSpace(string(shaOut))
	}

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
		return c.failAfterOrphanOp(priorBranch, rmErr, rmOut, "git rm -rf . on orphan branch")
	}
	commitCmd := c.cmd("commit", "--no-verify", "--allow-empty", "-m", "chore: init armature issues branch")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return c.failAfterOrphanOp(priorBranch, err, out, "git commit on orphan branch")
	}

	// Return to the original branch by name (not `checkout -` which may fail on fresh repos)
	restore := c.cmd("checkout", priorBranch)
	if out, err := restore.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", priorBranch, err, out)
	}
	return nil
}

func (c *Client) failAfterOrphanOp(priorBranch string, opErr error, out []byte, op string) error {
	if restoreErr := c.cmd("checkout", priorBranch).Run(); restoreErr != nil {
		return fmt.Errorf("%s failed: %w; then failed to restore to %s: %w\n%s", op, opErr, priorBranch, restoreErr, out)
	}
	return fmt.Errorf("%s: %w\n%s", op, opErr, out)
}

// AddWorktree adds a linked worktree for an existing branch at the given path.
// A live worktree at that path is a no-op. A stale .git pointer left behind by a
// failed rollback is removed so Git can register a usable replacement.
func (c *Client) AddWorktree(branch, path string) error {
	gitFile := filepath.Join(path, ".git")
	if _, err := os.Stat(gitFile); err == nil {
		check := c.cmd("-C", path, "rev-parse", "--is-inside-work-tree")
		if err := check.Run(); err == nil {
			return nil
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

func (c *Client) SetGitConfig(key, value string) error {
	return c.setGitConfig("--local", key, value, "git config set")
}

// SetGitConfigWorktree writes a worktree-scoped git config key (--worktree).
func (c *Client) SetGitConfigWorktree(key, value string) error {
	return c.setGitConfig("--worktree", key, value, "git config --worktree set")
}

func (c *Client) setGitConfig(scope, key, value, errLabel string) error {
	if out, err := c.cmd("config", scope, key, value).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", errLabel, key, err, out)
	}
	return nil
}

// ReadGitConfigWorktree reads a worktree-scoped git config key.
func (c *Client) ReadGitConfigWorktree(key string) (string, error) {
	return c.getGitConfig([]string{"--worktree", "--get", key}, "git config --worktree get", key)
}

// CommonGitDir returns git rev-parse --git-common-dir (absolute).
func (c *Client) CommonGitDir() (string, error) {
	cmd := c.cmd("rev-parse", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		return "", fmt.Errorf("git common dir empty")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(c.repoPath, dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if resolved, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

// ReadGitConfig reads a local git config key. Returns error if unset.
func (c *Client) ReadGitConfig(key string) (string, error) {
	return c.getGitConfig([]string{"--local", key}, "git config get", key)
}

func (c *Client) getGitConfig(args []string, errLabel, key string) (string, error) {
	out, err := c.cmd(append([]string{"config"}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", errLabel, key, err)
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
	if out, err := c.runMutatingWithRetry("git add "+relPath, "add", relPath); err != nil {
		return fmt.Errorf("%s", enhanceGitLockfileError(
			fmt.Sprintf("git add %s: %v\n%s", relPath, err, out),
			string(out),
		))
	}

	diff := c.cmd("diff", "--cached", "--quiet")
	if err := diff.Run(); err == nil {
		return nil
	}

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
	if strings.Contains(out, "index.lock") && strings.Contains(strings.ToLower(out), "read-only file system") {
		return base + "\nHint: sandbox blocked git lockfile writes (.git/worktrees/*/index.lock). Re-run this arm command with elevated permissions/approval."
	}
	return base
}

func (c *Client) Push(branch string) error {
	cmd, ctx, cancel := c.withPublishDeadline().networkCmd("push", "origin", branch)
	defer cancel()
	out, err := cmd.CombinedOutput()
	out, err = networkResult(ctx, out, err)
	if err != nil {
		return fmt.Errorf("git push origin %s: %w\n%s", branch, err, out)
	}
	return nil
}

func (c *Client) FetchAndRebase(branch string) error {
	fetch, ctx, cancel := c.withPublishDeadline().networkCmd("fetch", "origin")
	defer cancel()
	out, err := fetch.CombinedOutput()
	out, err = networkResult(ctx, out, err)
	if err != nil {
		return fmt.Errorf("git fetch origin: %w\n%s", err, out)
	}
	rebase := c.cmd("rebase", "origin/"+branch)
	out, err = rebase.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git rebase origin/%s: %w\n%s", branch, err, out)
	}
	return nil
}

// ErrRemoteRefMissing reports that origin was reachable but the requested
// branch/ref is not advertised. Distinct from transport/unreachable failures.
var ErrRemoteRefMissing = errors.New("couldn't find remote ref")

// IsRemoteRefMissing reports whether err (or its chain/text) is an absent
// advertised remote ref rather than an unreachable origin / other fetch failure.
func IsRemoteRefMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRemoteRefMissing) {
		return true
	}
	return remoteRefMissingText(err.Error())
}

func remoteRefMissingText(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "couldn't find remote ref") ||
		strings.Contains(lower, "does not exist in remote") ||
		strings.Contains(lower, "no such ref") ||
		(strings.Contains(lower, "remote ref") && strings.Contains(lower, "not found"))
}

// FetchTrackingRefWithoutMovingHEAD updates refs/remotes/origin/<branch> from origin without
// moving the local branch.
func (c *Client) FetchTrackingRefWithoutMovingHEAD(branch string) error {
	return c.FetchBranchToRef(branch, "refs/remotes/origin/"+branch)
}

// FetchBranchToRef force-fetches origin refs/heads/<branch> into dstRef without
// moving HEAD. dstRef is the full destination ref (caller-chosen) so concurrent
// ops-backup runs can avoid sharing refs/remotes/origin/_armature.
//
// Empty --refmap= disables the remote's configured refspec: without it,
// `git fetch origin +refs/heads/_armature:<per-run>` also updates
// refs/remotes/origin/_armature and races under concurrency ("cannot lock ref").
// The explicit refspec still writes dstRef (including when dstRef is the
// conventional tracking tip via FetchTrackingRefWithoutMovingHEAD).
func (c *Client) FetchBranchToRef(branch, dstRef string) error {
	branch = strings.TrimSpace(branch)
	dstRef = strings.TrimSpace(dstRef)
	if branch == "" || dstRef == "" {
		return fmt.Errorf("fetch branch to ref: branch and destination ref are required")
	}
	// Fail-fast if origin has no URL; fetch addresses the named remote so
	// remote.origin.* (proxy, tagOpt, …) applies and multi-valued URLs stay
	// first-wins (git fetch by name). Do not -c remote.origin.url= — that
	// appends a value and does not override.
	remoteURL, err := c.firstRemoteURL("origin")
	if err != nil {
		return fmt.Errorf("fetch branch to ref: %w", err)
	}
	spec := "+refs/heads/" + branch + ":" + dstRef
	cmd, ctx, cancel := c.networkCmd("fetch", "--no-tags", "--no-write-fetch-head", "--refmap=", "origin", spec)
	defer cancel()
	out, err := cmd.CombinedOutput()
	out, err = networkResult(ctx, out, err)
	if err != nil {
		wrapped := fmt.Errorf("git fetch origin %s (first-url %s): %w\n%s", branch, remoteURL, err, out)
		if remoteRefMissingText(string(out)) || remoteRefMissingText(err.Error()) {
			return fmt.Errorf("%w: %v", ErrRemoteRefMissing, wrapped)
		}
		return wrapped
	}
	return nil
}

// withPublishDeadline returns a client whose networkCmd budget is the publish
// cap, unless the caller already set WithNetworkTimeout.
func (c *Client) withPublishDeadline() *Client {
	if c == nil || c.networkTimeout > 0 {
		return c
	}
	return c.WithNetworkTimeout(defaultGitPublishNetworkTimeout)
}

// networkCmd is the only owner of remote-network git (fetch, push, ls-remote,
// pull, clone, and probes of a named remote). It applies the deadline,
// process-group cancel, and WaitDelay. Pass the remote by name so
// remote.<name>.* applies inside the clone; do not -c remote.<name>.url=
// (git appends a value and does not override). Caller must defer cancel().
func (c *Client) networkCmd(args ...string) (*exec.Cmd, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), c.effectiveNetworkTimeout())
	cmd := c.cmdContext(ctx, args...)
	configureNetworkCmd(cmd)
	return cmd, ctx, cancel
}

// networkResult keeps a deadline visible when process-group kill turns it into
// "signal: killed". Successful runs (nil err) are unchanged.
func networkResult(ctx context.Context, out []byte, err error) ([]byte, error) {
	if err != nil && ctx != nil && ctx.Err() != nil {
		return out, fmt.Errorf("%w: %v", ctx.Err(), err)
	}
	return out, err
}

func (c *Client) RevListCount(rangeSpec string) (int, error) {
	cmd := c.cmd("rev-list", "--count", rangeSpec)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count %s: %w", rangeSpec, err)
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if convErr != nil {
		return 0, fmt.Errorf("git rev-list --count %s: parse %q: %w", rangeSpec, strings.TrimSpace(string(out)), convErr)
	}
	return n, nil
}

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

const gitLogPretty = "%H%x00%s%x00%ae%x00%ai%x00%P"

// LogRange returns log entries reachable from head but not from base
// (`git log base..head`), most recent first.
func (c *Client) LogRange(base, head string) ([]LogEntry, error) {
	return c.gitLogEntries("git log "+base+".."+head, "log", base+".."+head, "--format="+gitLogPretty, "--")
}

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

func (c *Client) ShowFileAtCommit(sha, path string) ([]byte, error) {
	cmd := c.cmd("show", sha+":"+path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return out, nil
}

func (c *Client) BlobExists(rev, path string) bool {
	cmd := c.cmd("cat-file", "-e", rev+":"+path)
	return cmd.Run() == nil
}

// LogBranch returns up to n log entries from the tip of branch, most recent first.
// A non-positive n returns all entries.
func (c *Client) LogBranch(branch string, n int) ([]LogEntry, error) {
	if strings.HasPrefix(branch, "-") {
		return nil, fmt.Errorf("invalid branch %q", branch)
	}
	args := []string{"log", branch, "--format=" + gitLogPretty}
	if n > 0 {
		args = append(args, fmt.Sprintf("-n%d", n))
	}
	// git log BRANCH -- disambiguates a branch name that collides with a path
	// ("ambiguous argument").
	args = append(args, "--")
	return c.gitLogEntries("git log "+branch, args...)
}

func (c *Client) gitLogEntries(errPrefix string, args ...string) ([]LogEntry, error) {
	out, err := c.cmd(args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errPrefix, err)
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

func (c *Client) HeadSHA() (string, error) {
	cmd := c.cmd("rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

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

// RemoveWorktree removes a linked worktree at the given path without forcing
// deletion. Git therefore rejects dirty tracked or untracked content, which
// preserves work until an operator deliberately resolves it.
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
	return c.DiffNameStatusRange(baseSHA, "HEAD")
}

func (c *Client) DiffNameStatusRange(baseSHA, head string) ([]DiffStatusEntry, error) {
	// -z switches git to NUL-delimited, unquoted output: without it, git
	// quotes and octal-escapes any path containing non-ASCII or special
	// characters (e.g. "caf\303\251.go" instead of the literal "café.go"),
	// which breaks scope-containment comparisons against the literal path.
	cmd := c.cmd("diff", "--name-status", "-M", "-z", baseSHA, head)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status -M %s %s: %w", baseSHA, head, err)
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

func (c *Client) ResetHard(ref string) error {
	cmd := c.cmd("reset", "--hard", ref)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s: %w\n%s", ref, err, out)
	}
	return nil
}

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
// the working tree files. An untracked path is a no-op (--ignore-unmatch), but
// any other failure — a locked or unwritable index, say — is returned, so a
// caller cannot mistake a still-tracked path for a successful untrack.
func (c *Client) RemoveFromIndex(path string) error {
	cmd := c.cmd("rm", "-r", "--cached", "--quiet", "--ignore-unmatch", "--", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git rm -r --cached %s: %w\n%s", path, err, out)
	}
	return nil
}

// RestoreIndexFromHEAD puts the given repository-relative paths back into the
// index as they are at HEAD, without touching the working tree. Used to undo
// a `git rm --cached` when the follow-up commit fails, so a retry still sees
// the paths as tracked instead of as dangling staged deletions.
func (c *Client) RestoreIndexFromHEAD(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"reset", "-q", "HEAD", "--"}, paths...)
	out, err := c.cmd(args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git reset HEAD -- <paths>: %w\n%s", err, out)
	}
	return nil
}

func (c *Client) StagedPaths() ([]string, error) {
	out, err := c.cmd("diff", "--cached", "--name-only", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached --name-only: %w", err)
	}
	return splitNUL(out), nil
}

func (c *Client) IsTracked(path string) bool {
	cmd := c.cmd("ls-files", path)
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

func (c *Client) CommitPathsNoVerify(message string, paths ...string) error {
	args := append([]string{"commit", "--no-verify", "-m", message, "--"}, paths...)
	cmd := c.cmd(args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if strings.Contains(string(out), "nothing to commit") ||
		strings.Contains(string(out), "nothing added to commit") {
		return nil
	}
	return fmt.Errorf("git commit: %w\n%s", err, out)
}

// CommitWithMessage creates a commit with the given message. Returns an error
// if there is nothing staged to commit.
func (c *Client) CommitWithMessage(message string) error {
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

// CommitIndexNoVerify commits whatever is currently staged. It
// takes no pathspec, unlike CommitPathsNoVerify, because a path-scoped commit
// re-reads those paths from the working tree and so cannot record an index-only
// change such as a `git rm --cached` removal of a file that stays on disk.
// No-op (nil) when the index is clean.
func (c *Client) CommitIndexNoVerify(message string) error {
	if err := c.cmd("diff", "--cached", "--quiet").Run(); err == nil {
		return nil
	}
	out, err := c.runMutatingWithRetry("git commit", "commit", "--no-verify", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit --no-verify: %w\n%s", err, out)
	}
	return nil
}

func (c *Client) ResolveRevision(rev string) (string, error) {
	cmd := c.cmd("rev-parse", "--verify", rev)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to resolve revision %s: %w", rev, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) DiffRange(base, head string) (string, error) {
	cmd := c.cmd("diff", base+"..."+head, "--")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to diff range %s...%s: %w", base, head, err)
	}
	return string(output), nil
}

func (c *Client) DiffNameOnlyRange(base, head string) ([]string, error) {
	cmd := c.cmd("diff", "--name-only", "-z", base+"..."+head, "--")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff --name-only %s...%s: %w", base, head, err)
	}
	return splitNUL(output), nil
}

func splitNUL(out []byte) []string {
	raw := strings.TrimRight(string(out), "\x00")
	if raw == "" {
		return []string{}
	}
	return strings.Split(raw, "\x00")
}

// RevListReverse returns commit SHAs reachable from rev, oldest first.
func (c *Client) RevListReverse(rev string) ([]string, error) {
	cmd := c.cmd("rev-list", "--reverse", rev)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-list --reverse %s: %w", rev, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\n"), nil
}

func (c *Client) CommitterUnix(sha string) (int64, error) {
	cmd := c.cmd("log", "-1", "--format=%ct", sha)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git log committer %s: %w", sha, err)
	}
	n, convErr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if convErr != nil {
		return 0, fmt.Errorf("parse committer unix %s: %w", sha, convErr)
	}
	return n, nil
}

// CommitterUnixRange returns committer unix timestamps for every commit in
// rangeSpec (e.g. "origin/_armature..HEAD") in one git process. Empty ranges
// yield an empty slice, not an error. Order matches `git log` (newest first);
// callers that need a minimum should scan the slice.
func (c *Client) CommitterUnixRange(rangeSpec string) ([]int64, error) {
	rangeSpec = strings.TrimSpace(rangeSpec)
	if rangeSpec == "" {
		return nil, fmt.Errorf("git log committer range: range is required")
	}
	cmd := c.cmd("log", "--format=%ct", rangeSpec)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log committer range %s: %w", rangeSpec, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []int64{}, nil
	}
	lines := strings.Split(raw, "\n")
	outUnix := make([]int64, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, convErr := strconv.ParseInt(line, 10, 64)
		if convErr != nil {
			return nil, fmt.Errorf("parse committer unix in range %s: %q: %w", rangeSpec, line, convErr)
		}
		outUnix = append(outUnix, n)
	}
	return outUnix, nil
}

// CommitWalkEntry is one commit from CommitWalkReverse: identity, committer
// time, first parent (empty for roots), and paths changed vs that parent
// (full tree for roots). Matches DiffNameOnlyRange(firstParent, sha) /
// ListFilesAtCommit for roots.
type CommitWalkEntry struct {
	SHA           string
	CommitterUnix int64
	FirstParent   string
	ChangedFiles  []string
}

// BlobRef names a tree path at a revision for ShowFilesBatch (`rev:path`).
type BlobRef struct {
	Rev  string
	Path string
}

// BlobResult is one ShowFilesBatch response. OK is false when the path is
// missing at Rev (cat-file "missing"); that is not an error.
type BlobResult struct {
	Content []byte
	OK      bool
}

// commitWalkFormat is TAB-separated sha/ct/parents so -z can NUL-delimit
// headers from paths. Avoids embedding %x00 in --format (ambiguous with -z).
const commitWalkFormat = "%H%x09%ct%x09%P"

// commitWalkConfigPins force walk output independent of user git config.
// log.diffMerges=separate matches `--diff-merges=separate` (per-parent
// blocks). The CLI flag alone already overrides hostile log.diffMerges
// (combined/dense-combined/remerge/off/…); the -c pin stays as belt-and-
// suspenders. diff.renames=true matches DiffNameOnlyRange rename detection.
func commitWalkConfigPins() []string {
	return []string{
		"-c", "log.diffMerges=separate",
		"-c", "log.showRoot=true",
		"-c", "core.quotePath=false",
		"-c", "color.ui=never",
		"-c", "color.diff=never",
		"-c", "log.decorate=false",
		"-c", "diff.renames=true",
		"-c", "diff.noprefix=false",
		"-c", "diff.relative=false",
	}
}

// CommitWalkReverse returns every commit reachable from rev, oldest first, in
// one git process. FirstParent and ChangedFiles use first-parent merge diffs
// so callers can replace per-SHA CommitterUnix, FirstParent, DiffNameOnlyRange,
// and ListFilesAtCommit loops.
//
// Walk target is Git 2.34+: `--diff-merges=separate` (Git 2.31+) for
// per-parent merge diffs, `--root` so log.showRoot=false still emits the
// root commit, `-z` so core.quotePath cannot C-quote non-ASCII paths.
// `--diff-merges=separate` repeats merge SHAs once per parent;
// parseCommitWalkOutput keeps the first block (first-parent diff).
// Walk-affecting config is pinned via commitWalkConfigPins so user
// log.diffMerges / showRoot / quotePath / color / decorate / renames /
// noprefix / relative cannot change the machine-readable output (hash
// algorithm is still repo-native: SHA-1 40-hex or SHA-256 64-hex OIDs).
// Documented install min in architecture.md is unchanged by this helper.
func (c *Client) CommitWalkReverse(rev string) ([]CommitWalkEntry, error) {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return nil, fmt.Errorf("git log commit walk: rev is required")
	}
	args := make([]string, 0, len(commitWalkConfigPins())+8)
	args = append(args, commitWalkConfigPins()...)
	args = append(args,
		"log",
		"-z",
		"--reverse",
		"--diff-merges=separate",
		"--root",
		"--name-only",
		"--format="+commitWalkFormat,
		rev,
	)
	cmd := c.cmd(args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log commit walk %s: %w", rev, err)
	}
	entries, parseErr := parseCommitWalkOutput(out)
	if parseErr != nil {
		return nil, fmt.Errorf("parse commit walk %s: %w", rev, parseErr)
	}
	return entries, nil
}

// FilesBatchSession is one `git cat-file --batch` process. Read one BlobRef at
// a time so callers can discard each snapshot before requesting the next
// (LocateOps must stay O(history) memory, not O(history×log-size)).
type FilesBatchSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	closed bool
}

// BeginFilesBatch starts a Git 2.25+ `cat-file --batch` session.
func (c *Client) BeginFilesBatch() (*FilesBatchSession, error) {
	cmd := c.cmd("cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close() //nolint:errcheck // best-effort abort before start
		return nil, fmt.Errorf("git cat-file --batch stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close() //nolint:errcheck // best-effort abort before start
		return nil, fmt.Errorf("git cat-file --batch start: %w", err)
	}
	return &FilesBatchSession{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	}, nil
}

// Read requests one rev:path and returns its blob. The returned Content is not
// retained by the session; callers should drop it before the next Read.
func (s *FilesBatchSession) Read(ref BlobRef) (BlobResult, error) {
	if s == nil || s.closed {
		return BlobResult{}, fmt.Errorf("git cat-file --batch: session closed")
	}
	line := ref.Rev + ":" + ref.Path + "\n"
	if _, err := io.WriteString(s.stdin, line); err != nil {
		return BlobResult{}, fmt.Errorf("git cat-file --batch write %s:%s: %w", ref.Rev, ref.Path, err)
	}
	res, err := readCatFileBatchItem(s.reader)
	if err != nil {
		return BlobResult{}, fmt.Errorf("git cat-file --batch read %s:%s: %w", ref.Rev, ref.Path, err)
	}
	return res, nil
}

// Close ends the batch session. Safe to call multiple times.
func (s *FilesBatchSession) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	_ = s.stdin.Close() //nolint:errcheck // closing writer signals EOF to cat-file
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("git cat-file --batch: %w", err)
	}
	return nil
}

// ShowFilesBatch returns blob contents for each rev:path in one
// `git cat-file --batch` process (Git 2.25+). Missing paths yield OK=false.
// Prefer BeginFilesBatch/Read when snapshots must not all be retained.
func (c *Client) ShowFilesBatch(refs []BlobRef) ([]BlobResult, error) {
	out := make([]BlobResult, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	sess, err := c.BeginFilesBatch()
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.Close() }() //nolint:errcheck // results already read
	for i, ref := range refs {
		res, rerr := sess.Read(ref)
		if rerr != nil {
			_ = sess.Close() //nolint:errcheck
			return nil, rerr
		}
		out[i] = res
	}
	if err := sess.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func readCatFileBatchItem(r *bufio.Reader) (BlobResult, error) {
	header, err := r.ReadBytes('\n')
	if err != nil {
		return BlobResult{}, err
	}
	line := strings.TrimSuffix(string(header), "\n")
	if strings.HasSuffix(line, " missing") {
		return BlobResult{OK: false}, nil
	}
	// <oid> SP <type> SP <size> — oid is 40-hex (SHA-1) or 64-hex (SHA-256).
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return BlobResult{}, fmt.Errorf("bad cat-file header %q", line)
	}
	if !isGitObjectID(fields[0]) {
		return BlobResult{}, fmt.Errorf("bad cat-file oid in %q: want 40- or 64-hex object id", line)
	}
	size, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil || size < 0 {
		return BlobResult{}, fmt.Errorf("bad cat-file size in %q: %w", line, err)
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(r, content); err != nil {
		return BlobResult{}, err
	}
	// cat-file --batch appends a trailing LF after the payload.
	if _, err := r.ReadByte(); err != nil {
		return BlobResult{}, err
	}
	return BlobResult{Content: content, OK: true}, nil
}

func parseCommitWalkOutput(out []byte) ([]CommitWalkEntry, error) {
	if len(out) == 0 {
		return []CommitWalkEntry{}, nil
	}
	parts := bytes.Split(out, []byte{0})
	var entries []CommitWalkEntry
	seen := map[string]struct{}{}
	i := 0
	for i < len(parts) {
		raw := parts[i]
		i++
		if len(raw) == 0 {
			continue
		}
		if !isCommitWalkHeader(raw) {
			return nil, fmt.Errorf("expected commit header, got %q", truncateForErr(raw))
		}
		sha, ct, firstParent, herr := parseCommitWalkHeader(raw)
		if herr != nil {
			return nil, herr
		}
		var files []string
		for i < len(parts) {
			p := parts[i]
			if len(p) == 0 {
				i++
				break
			}
			if isCommitWalkHeader(p) {
				break
			}
			path := strings.TrimPrefix(string(p), "\n")
			if path != "" {
				files = append(files, path)
			}
			i++
		}
		if _, dup := seen[sha]; dup {
			continue
		}
		seen[sha] = struct{}{}
		if files == nil {
			files = []string{}
		}
		entries = append(entries, CommitWalkEntry{
			SHA:           sha,
			CommitterUnix: ct,
			FirstParent:   firstParent,
			ChangedFiles:  files,
		})
	}
	return entries, nil
}

func isGitObjectID(s string) bool {
	return gitoid.Valid(s)
}

func isCommitWalkHeader(raw []byte) bool {
	// OID (40 hex SHA-1 or 64 hex SHA-256) TAB ...
	switch {
	case len(raw) > gitoid.LenSHA1 && raw[gitoid.LenSHA1] == '\t':
		return isGitObjectID(string(raw[:gitoid.LenSHA1]))
	case len(raw) > gitoid.LenSHA256 && raw[gitoid.LenSHA256] == '\t':
		return isGitObjectID(string(raw[:gitoid.LenSHA256]))
	default:
		return false
	}
}

func parseCommitWalkHeader(raw []byte) (sha string, ct int64, firstParent string, err error) {
	fields := bytes.SplitN(raw, []byte{'\t'}, 3)
	if len(fields) != 3 {
		return "", 0, "", fmt.Errorf("commit header %q: want sha, ct, parents", truncateForErr(raw))
	}
	sha = string(fields[0])
	if !isGitObjectID(sha) {
		return "", 0, "", fmt.Errorf("commit header oid %q: want 40- or 64-hex object id", sha)
	}
	ct, convErr := strconv.ParseInt(string(fields[1]), 10, 64)
	if convErr != nil {
		return "", 0, "", fmt.Errorf("committer unix for %s: %w", sha, convErr)
	}
	if parents := strings.Fields(string(fields[2])); len(parents) > 0 {
		if !isGitObjectID(parents[0]) {
			return "", 0, "", fmt.Errorf("commit header parent oid %q: want 40- or 64-hex object id", parents[0])
		}
		firstParent = parents[0]
	}
	return sha, ct, firstParent, nil
}

func truncateForErr(b []byte) string {
	const max = 96 // fits a 64-hex SHA-256 oid plus a short suffix
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

// IsAncestor reports whether anc is an ancestor of desc (inclusive).
func (c *Client) IsAncestor(anc, desc string) (bool, error) {
	cmd := c.cmd("merge-base", "--is-ancestor", anc, desc)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %w", anc, desc, err)
}

// FirstParent returns the first parent of sha. ok is false for a root commit.
func (c *Client) FirstParent(sha string) (parent string, ok bool, err error) {
	cmd := c.cmd("rev-parse", "--verify", sha+"^")
	out, runErr := cmd.Output()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("git rev-parse %s^: %w", sha, runErr)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", false, nil
	}
	return p, true, nil
}

// UpdateRef points ref at sha (fully resolved by the caller).
func (c *Client) UpdateRef(ref, sha string) error {
	out, err := c.cmd("update-ref", ref, sha).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref %s %s: %w\n%s", ref, sha, err, out)
	}
	return nil
}

// DeleteRef removes ref. A missing ref is success.
func (c *Client) DeleteRef(ref string) error {
	if _, err := c.ResolveRevision(ref); err != nil {
		return nil
	}
	out, err := c.cmd("update-ref", "-d", ref).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git update-ref -d %s: %w\n%s", ref, err, out)
	}
	return nil
}

// FirstParentAfter returns first-parent commits of head after base, oldest first.
func (c *Client) FirstParentAfter(base, head string) ([]string, error) {
	cmd := c.cmd("rev-list", "--first-parent", "--reverse", base+".."+head)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-list --first-parent %s..%s: %w", base, head, err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return []string{}, nil
	}
	return strings.Split(raw, "\n"), nil
}

// DiffTwoDot is path-literal `git diff --no-renames base tip` (two-dot combined range).
func (c *Client) DiffTwoDot(base, tip string) (string, error) {
	cmd := c.cmd("diff", "--no-ext-diff", "--no-color", "--no-renames", base, tip, "--")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to diff %s %s: %w", base, tip, err)
	}
	return string(output), nil
}

// DiffNameOnlyTwoDot is path-literal `git diff --name-only base tip` (no rename detection).
func (c *Client) DiffNameOnlyTwoDot(base, tip string) ([]string, error) {
	cmd := c.cmd("diff", "--name-only", "--no-renames", "-z", base, tip, "--")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff --name-only %s %s: %w", base, tip, err)
	}
	return splitNUL(output), nil
}

// StablePatchID is `git patch-id --stable` of a raw diff. Empty diff is "".
func (c *Client) StablePatchID(diff string) (string, error) {
	if strings.TrimSpace(diff) == "" {
		return "", nil
	}
	cmd := c.cmd("patch-id", "--stable")
	cmd.Stdin = strings.NewReader(diff)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git patch-id --stable: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

// CommitDiff is the first-parent patch of sha (`git diff sha^ sha`).
func (c *Client) CommitDiff(sha string) (string, error) {
	parent, ok, err := c.FirstParent(sha)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("commit %s has no first parent", sha)
	}
	return c.DiffTwoDot(parent, sha)
}

// TreeEntry returns mode and object ID at rev:path. Missing paths are empty strings.
func (c *Client) TreeEntry(rev, path string) (mode, oid string, err error) {
	cmd := c.cmd("ls-tree", "-z", rev, "--", path)
	out, runErr := cmd.Output()
	if runErr != nil {
		return "", "", fmt.Errorf("git ls-tree %s -- %s: %w", rev, path, runErr)
	}
	raw := strings.TrimRight(string(out), "\x00")
	if raw == "" {
		return "", "", nil
	}
	line, _, _ := strings.Cut(raw, "\x00")
	modeType, _, ok := strings.Cut(line, "\t")
	if !ok {
		return "", "", fmt.Errorf("git ls-tree %s -- %s: malformed output", rev, path)
	}
	fields := strings.Fields(modeType)
	if len(fields) < 3 {
		return "", "", fmt.Errorf("git ls-tree %s -- %s: malformed output", rev, path)
	}
	return fields[0], fields[2], nil
}

// BundleCreateAdvertised writes a git bundle at outputPath that advertises
// advertiseRef at tipSHA. Objects are pushed by tip SHA into a throwaway bare
// repo so the working repository never grows a shared staging ref (concurrent
// snapshots cannot cross-wire tips or delete mid-push).
func (c *Client) BundleCreateAdvertised(outputPath, tipSHA, advertiseRef string) error {
	outputPath = strings.TrimSpace(outputPath)
	tipSHA = strings.TrimSpace(tipSHA)
	advertiseRef = strings.TrimSpace(advertiseRef)
	if outputPath == "" || tipSHA == "" || advertiseRef == "" {
		return fmt.Errorf("bundle create: output, tip, and advertise ref are required")
	}
	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("bundle create: abs output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o750); err != nil {
		return fmt.Errorf("bundle create: mkdir: %w", err)
	}

	workDir, releaseWork, err := c.beginArmatureWorkDir()
	if err != nil {
		return fmt.Errorf("bundle create: temp bare: %w", err)
	}
	defer releaseWork()
	bare := filepath.Join(workDir, "bare.git")
	if out, initErr := c.cmd("init", "--bare", bare).CombinedOutput(); initErr != nil {
		return fmt.Errorf("git init --bare: %w\n%s", initErr, out)
	}

	// Push tip SHA directly — no lock-free fixed refs/bundle-backup/_armature.
	spec := tipSHA + ":" + advertiseRef
	// gitnetallow: bundle-create-local-bare-push local bare path under armature-tmp-work, not a named remote
	if out, pushErr := c.cmd("push", bare, spec).CombinedOutput(); pushErr != nil {
		return fmt.Errorf("git push bare %s: %w\n%s", spec, pushErr, out)
	}

	bareClient := New(bare).WithEnviron(c.environ)
	if out, bundleErr := bareClient.cmd("bundle", "create", absOut, advertiseRef).CombinedOutput(); bundleErr != nil {
		return fmt.Errorf("git bundle create: %w\n%s", bundleErr, out)
	}
	return nil
}

// BundleListHeads returns ref names advertised by a git bundle (first column of
// git bundle list-heads).
func (c *Client) BundleListHeads(bundlePath string) ([]string, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return nil, fmt.Errorf("bundle list-heads: path required")
	}
	abs, err := filepath.Abs(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("bundle list-heads: abs: %w", err)
	}
	out, err := c.cmd("bundle", "list-heads", abs).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git bundle list-heads: %w\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	refs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		refs = append(refs, fields[1])
	}
	return refs, nil
}

// BundleVerifyRestorable confirms a bundle advertises wantRef and that its
// object pack is fetchable. list-heads / `git bundle verify` only read the
// header — a truncated pack can still look green there when the working repo
// already has the objects — so this fetches into a throwaway bare repo.
func (c *Client) BundleVerifyRestorable(bundlePath, wantRef string) error {
	bundlePath = strings.TrimSpace(bundlePath)
	wantRef = strings.TrimSpace(wantRef)
	if bundlePath == "" || wantRef == "" {
		return fmt.Errorf("bundle verify: path and advertise ref are required")
	}
	refs, err := c.BundleListHeads(bundlePath)
	if err != nil {
		return err
	}
	found := false
	for _, ref := range refs {
		if ref == wantRef {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("bundle verify: bundle does not advertise %s (got %v)", wantRef, refs)
	}

	abs, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("bundle verify: abs: %w", err)
	}
	workDir, releaseWork, err := c.beginArmatureWorkDir()
	if err != nil {
		return fmt.Errorf("bundle verify: temp bare: %w", err)
	}
	defer releaseWork()
	bare := filepath.Join(workDir, "bare.git")
	if out, initErr := c.cmd("init", "--bare", bare).CombinedOutput(); initErr != nil {
		return fmt.Errorf("git init --bare: %w\n%s", initErr, out)
	}
	bareClient := New(bare).WithEnviron(c.environ)
	spec := "+" + wantRef + ":" + wantRef
	// gitnetallow: bundle-verify-local-fetch absolute bundle file into a throwaway local bare, not a remote URL
	if out, fetchErr := bareClient.cmd("fetch", "--no-tags", abs, spec).CombinedOutput(); fetchErr != nil {
		return fmt.Errorf("git fetch bundle into verify bare %s: %w\n%s", spec, fetchErr, out)
	}
	return nil
}

// RestoreEvidence compares the published ops tip to a bundle tip inside the
// user's clone. Network probes (ls-remote + origin fetch) run with -C <clone>
// so git resolves scopes, worktree config, includes, insteadOf, and helpers
// natively — no regex-copied localTransportConfig. Isolation is per-call refs
// under refs/armature-tmp/<run-id>/ (never refs/remotes/origin/* or user
// branches). Staging uses a live-pid lease; crash leftovers are swept at the
// next run start without touching concurrent live nonces.
//
// Origin absence is structural: ls-remote reports no exact refname match →
// missing (git patterns can suffix-match peers like
// refs/heads/archive/refs/heads/_armature — those must not count as exists).
// Unreachable origin / other failures return err (fail-closed). Network probes
// address the named origin remote so remote.origin.* applies; multi-valued
// remote URLs stay first-wins (do not -c remote.origin.url= — that appends).
func (c *Client) RestoreEvidence(
	bundlePath, advertiseRef, opsBranch string,
) (originMissing bool, originTip, recovered string, originAncOfRecovered, recoveredAncOfOrigin bool, err error) {
	bundlePath = strings.TrimSpace(bundlePath)
	advertiseRef = strings.TrimSpace(advertiseRef)
	opsBranch = strings.TrimSpace(opsBranch)
	if bundlePath == "" || advertiseRef == "" || opsBranch == "" {
		return false, "", "", false, false, fmt.Errorf("restore evidence: bundle, advertise ref, and branch are required")
	}
	remoteRef := "refs/heads/" + opsBranch

	tmpNS, releaseTmp, tmpErr := c.beginArmatureTmpNS()
	if tmpErr != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: temp ref ns: %w", tmpErr)
	}
	defer releaseTmp()
	bundleTmp := tmpNS + "/bundle"
	originTmp := tmpNS + "/origin"

	remoteURL, urlErr := c.firstRemoteURL("origin")
	if urlErr != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: origin URL: %w", urlErr)
	}

	_, exists, lsErr := c.lsRemoteRef("origin", remoteRef)
	if lsErr != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: ls-remote origin %s (first-url %s) failed (fail-closed): %w", remoteRef, remoteURL, lsErr)
	}

	absBundle, absErr := filepath.Abs(bundlePath)
	if absErr != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: abs bundle: %w", absErr)
	}
	bundleSpec := "+" + advertiseRef + ":" + bundleTmp
	// gitnetallow: restore-local-bundle-fetch absolute bundle file into refs/armature-tmp; origin fetch stays on networkCmd
	if out, fetchErr := c.cmd("fetch", "--no-tags", "--no-write-fetch-head", "--refmap=", absBundle, bundleSpec).CombinedOutput(); fetchErr != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: load bundle: %w\n%s", fetchErr, out)
	}
	recovered, err = c.ResolveRevision(bundleTmp)
	if err != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: resolve recovered tip: %w", err)
	}
	if recovered == "" {
		return false, "", "", false, false, fmt.Errorf("restore evidence: resolve recovered tip: empty SHA")
	}
	if !exists {
		return true, "", recovered, false, false, nil
	}

	originSpec := "+" + remoteRef + ":" + originTmp
	cmd, ctx, cancel := c.networkCmd("fetch", "--no-tags", "--no-write-fetch-head", "--refmap=", "origin", originSpec)
	defer cancel()
	out, fetchErr := cmd.CombinedOutput()
	out, fetchErr = networkResult(ctx, out, fetchErr)
	if fetchErr != nil {
		return false, "", "", false, false, fmt.Errorf(
			"restore evidence: fetch origin %s (first-url %s) failed (fail-closed): %w\n%s",
			remoteRef, remoteURL, fetchErr, out)
	}
	originTip, err = c.ResolveRevision(originTmp)
	if err != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: resolve origin tip: %w", err)
	}
	if originTip == "" {
		return false, "", "", false, false, fmt.Errorf("restore evidence: resolve origin tip: empty SHA")
	}

	originAncOfRecovered, err = c.IsAncestor(originTip, recovered)
	if err != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: ancestry: %w", err)
	}
	recoveredAncOfOrigin, err = c.IsAncestor(recovered, originTip)
	if err != nil {
		return false, "", "", false, false, fmt.Errorf("restore evidence: ancestry: %w", err)
	}
	return false, originTip, recovered, originAncOfRecovered, recoveredAncOfOrigin, nil
}

// lsRemoteRef reports whether the named remote advertises an exact refname.
// Exit 0 + no exact match (empty stdout, or only pattern/suffix peers) means
// reachable remote, ref absent. Non-zero exit (unreachable, auth, …) is an
// error. Parsing goes through exactRefFromPatternOutput — the shared class
// helper for every ls-remote / for-each-ref pattern existence probe. The
// remote operand is always the name so remote.<name>.* applies; multi-valued
// URLs stay first-wins.
func (c *Client) lsRemoteRef(name, ref string) (sha string, exists bool, err error) {
	cmd, ctx, cancel := c.networkCmd("ls-remote", "--refs", name, ref)
	defer cancel()
	out, runErr := cmd.CombinedOutput()
	out, runErr = networkResult(ctx, out, runErr)
	if runErr != nil {
		return "", false, fmt.Errorf("git ls-remote %s %s: %w\n%s", name, ref, runErr, out)
	}
	sha, exists, err = exactRefFromPatternOutput(string(out), ref)
	if err != nil {
		return "", false, fmt.Errorf("git ls-remote %s %s: %w", name, ref, err)
	}
	return sha, exists, nil
}

// firstRemoteURL returns the first configured remote.<name>.url. Git fetch /
// ls-remote use the first URL when a remote has multiple; `git config --get`
// returns the last — callers that need one shared URL must use this.
func (c *Client) firstRemoteURL(name string) (string, error) {
	out, err := c.cmd("config", "--get-all", "remote."+name+".url").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git config remote.%s.url: %w\n%s", name, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		url := strings.TrimSpace(line)
		if url != "" {
			return url, nil
		}
	}
	return "", fmt.Errorf("git config remote.%s.url: empty URL", name)
}
