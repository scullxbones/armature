package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// List returns the complete, non-prunable worktree inventory for repoPath.
// Identity is read from each worktree's binding marker; branch and path are
// observations only. A failed git listing or unreadable marker fails closed.
func List(repoPath string) ([]Meta, error) {
	// #nosec G204 - git and its arguments are controlled by Armature.
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain: %w", err)
	}
	blocks := parsePorcelainBlocks(string(out))
	result := make([]Meta, 0, len(blocks))
	for _, block := range blocks {
		if block.prunable || block.path == "" {
			continue
		}
		issueID := ""
		gitDir, resolveErr := ResolveGitDir(block.path)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve worktree git dir for %s: %w", block.path, resolveErr)
		}
		issueID, err = ReadBinding(gitDir)
		if err != nil {
			return nil, fmt.Errorf("read worktree binding for %s: %w", block.path, err)
		}
		result = append(result, Meta{Path: block.path, Branch: block.branch, IssueID: issueID})
	}
	return result, nil
}

// ResolveGitDir resolves the actual git directory for a linked or main
// worktree. A linked worktree stores a relative or absolute gitdir pointer in
// its .git file; the main worktree uses a .git directory directly.
func ResolveGitDir(worktreePath string) (string, error) {
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat .git: %w", err)
	}
	if info.IsDir() {
		return gitPath, nil
	}
	data, err := os.ReadFile(gitPath) //nolint:gosec // path is the .git entry of a git worktree
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("unexpected .git file format: %s", line)
	}
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreePath, gitDir)
	}
	return gitDir, nil
}

// ReadBinding reads the current issue marker, falling back to the legacy task
// marker. Missing markers mean an unbound worktree; unreadable markers fail
// closed so inventory consumers never silently downgrade a corrupted binding.
func ReadBinding(gitDir string) (string, error) {
	for _, name := range []string{"armature-issue-id", "armature-task-id"} {
		path := filepath.Join(gitDir, name)
		data, err := os.ReadFile(path) //nolint:gosec // path is derived from the resolved git directory
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
	}
	return "", nil
}

// ListManaged returns the inventory entries directly under the canonical
// repository-local .worktrees root. It deliberately retains marker identity
// and detached state from List so all lifecycle consumers use one policy.
func ListManaged(repoPath string) ([]Meta, error) {
	all, err := List(repoPath)
	if err != nil {
		return nil, err
	}
	root := CanonicalRoot(repoPath)
	managed := make([]Meta, 0, len(all))
	for _, item := range all {
		path := NormalizePath(item.Path)
		if path != root && IsUnderRoot(path, root) {
			managed = append(managed, item)
		}
	}
	return managed, nil
}

// CanonicalRoot returns the normalized managed-worktree root for repoPath.
func CanonicalRoot(repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	return NormalizePath(filepath.Join(abs, ".worktrees"))
}

// CanonicalPath returns the canonical path for an issue ID. Callers must
// validate the issue ID before using this path for mutation.
func CanonicalPath(repoPath, issueID string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	return filepath.Join(abs, ".worktrees", issueID)
}

// IsUnderRoot reports whether path is root itself or a descendant of root.
func IsUnderRoot(path, root string) bool {
	path = NormalizePath(path)
	root = NormalizePath(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// CountByIssue reports how many inventory entries are marker-bound to id.
// Callers use it to distinguish "no bound worktree" (a legacy branch fallback
// may still apply) from "one or more bound worktrees" (where SelectByIssue's
// WorktreePath-first precedence must govern selection).
func CountByIssue(items []Meta, id string) int {
	n := 0
	for _, item := range items {
		if item.IssueID == id {
			n++
		}
	}
	return n
}

// SelectByIssue resolves the single inventory entry bound to id, applying the
// WorktreePath-first precedence used across the worktree lifecycle (mirroring
// selectGCRemoval in reconcile.go): a single bound entry is returned as-is;
// with multiple bound entries, only the one whose normalized path equals the
// recorded worktreePath is returned. It fails closed (ok=false) whenever more
// than one entry is bound and none uniquely matches the recorded path — including
// when worktreePath is empty — so a legacy explicit-path worktree and the
// canonical .worktrees/<id> worktree that share one marker can never be resolved
// to the wrong destructive target.
func SelectByIssue(items []Meta, id, worktreePath string) (Meta, bool) {
	var bound []Meta
	for _, item := range items {
		if item.IssueID == id {
			bound = append(bound, item)
		}
	}
	switch len(bound) {
	case 0:
		return Meta{}, false
	case 1:
		return bound[0], true
	}
	if worktreePath == "" {
		return Meta{}, false
	}
	want := NormalizePath(worktreePath)
	var matches []Meta
	for _, item := range bound {
		if NormalizePath(item.Path) == want {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return Meta{}, false
}

// HasPrunableRegistration reports whether repoPath has a worktree registration
// for the exact path that git considers prunable — its working directory was
// deleted but the administrative registration under .git/worktrees survives.
// worktree.List deliberately skips prunable blocks, so callers that need to
// detect this stale-registration case (which makes `git worktree add <path>`
// fail with "missing but already registered worktree") use this instead. It is
// scoped to the exact path so callers can clear only this registration with an
// exact-path re-add, never a broad `git worktree prune` that could drop
// unrelated registrations.
func HasPrunableRegistration(repoPath, path string) (bool, error) {
	// #nosec G204 - git and its arguments are controlled by Armature.
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git worktree list --porcelain: %w", err)
	}
	want := NormalizePathAllowingMissing(path)
	for _, block := range parsePorcelainBlocks(string(out)) {
		if block.prunable && block.path != "" && NormalizePathAllowingMissing(block.path) == want {
			return true, nil
		}
	}
	return false, nil
}

type porcelainBlock struct {
	path     string
	branch   string
	prunable bool
}

func parsePorcelainBlocks(output string) []porcelainBlock {
	var result []porcelainBlock
	var current porcelainBlock
	haveBlock := false
	flush := func() {
		if haveBlock {
			result = append(result, current)
		}
		current = porcelainBlock{}
		haveBlock = false
	}
	for line := range strings.SplitSeq(output, "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			if haveBlock {
				flush()
			}
			current.path = strings.TrimPrefix(line, "worktree ")
			haveBlock = true
		case strings.HasPrefix(line, "branch "):
			current.branch = strings.TrimPrefix(line, "branch ")
		case strings.HasPrefix(line, "detached"):
			current.branch = "detached"
		case strings.HasPrefix(line, "prunable "):
			current.prunable = true
		}
	}
	flush()
	return result
}
