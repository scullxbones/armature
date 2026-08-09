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
// Identity is read from each worktree's binding; branch and path are
// observations only. A failed git listing or unreadable binding fails closed.
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
		result = append(result, Meta{Path: block.path, Branch: block.branch, Binding: issueID})
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

// ReadBinding reads the current issue binding, falling back to the legacy task
// binding. Missing bindings mean an unbound worktree; unreadable bindings fail
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
// repository-local .worktrees root. It deliberately retains binding identity
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

// Resolution is the outcome of resolving which worktree an issue owns.
//
// Three outcomes, not two. A bool cannot distinguish "there is nothing to act
// on" from "there is more than one candidate and no way to choose between
// them", and a caller that collapses the two skips a fail-closed gate instead
// of refusing — the exact I5/I6 hole this type exists to make unrepresentable.
type Resolution int

const (
	// NotFound means no inventory entry carries this issue's binding. There is
	// genuinely nothing to act on; a caller MAY fall through to a read-only
	// widening lookup, but MUST NOT infer a target from a branch or a basename.
	NotFound Resolution = iota
	// Bound means exactly one entry was resolved and is safe to act on.
	Bound
	// Ambiguous means two or more entries carry this issue's binding and the
	// recorded worktree path does not uniquely pick one — e.g. a legacy
	// explicit-path worktree alongside the canonical .worktrees/<id> one.
	// Callers MUST fail closed. Treating this as NotFound would let a
	// destructive operation guess, or let a delivery gate be skipped entirely.
	Ambiguous
)

// SelectByIssue resolves the single inventory entry bound to id.
//
// This is a SELECTION question: "which one worktree do I act on?" It is the
// ONLY function in this package whose result may be passed to a destructive git
// operation (worktree removal, which runs with --force and can discard
// uncommitted work). It fails closed by construction.
//
// The issue binding is the sole identity authority. Branch name and directory
// basename are NOT identity and are never consulted here: a worktree detached
// mid-rebase, or parked on a scratch branch, is still bound to its issue, and a
// stranger's worktree that happens to hold the expected branch is still not.
//
// Precedence: exactly one bound entry resolves as-is; with several, only the
// entry whose normalized path equals the recorded worktreePath resolves.
// Anything else is Ambiguous — including an empty worktreePath, which cannot
// disambiguate anything.
//
// For the opposite question — "is ANY worktree bound to this issue?", where
// over-inclusion is the safe direction — use AnyBound.
func SelectByIssue(items []Meta, id, worktreePath string) (Meta, Resolution) {
	bound := boundEntries(items, id)
	switch len(bound) {
	case 0:
		return Meta{}, NotFound
	case 1:
		return bound[0], Bound
	}
	if worktreePath == "" {
		return Meta{}, Ambiguous
	}
	want := NormalizePath(worktreePath)
	var matches []Meta
	for _, item := range bound {
		if NormalizePath(item.Path) == want {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0], Bound
	}
	return Meta{}, Ambiguous
}

// AnyBound reports whether any inventory entry is bound to id.
//
// This is an EXISTENCE question: "does this issue still own a live worktree
// here?" It is deliberately over-inclusive, and NEVER usable to choose a
// target — it returns no entry precisely so its answer cannot reach a
// destructive operation. Use SelectByIssue for that.
//
// Over-inclusion is the safe direction here because a positive answer PREVENTS
// an action: doctor uses this to decide whether a claimed issue's worktree has
// vanished, and answering "none" for an issue that owns two bound worktrees
// would release a live worker's claim. Where SelectByIssue reports Ambiguous,
// this reports true.
//
// When worktreePath is recorded it must match a bound entry: an absolute
// recorded path identifies the clone that owns the claim, so a bound worktree
// at some other path is not evidence that THIS clone's recorded worktree is
// alive. With no recorded path the binding alone is sufficient, which is what
// keeps a legacy explicit-path worktree from being mistaken for a dead claim.
func AnyBound(items []Meta, id, worktreePath string) bool {
	for _, item := range boundEntries(items, id) {
		if worktreePath == "" {
			return true
		}
		if NormalizePathAllowingMissing(item.Path) == NormalizePathAllowingMissing(worktreePath) {
			return true
		}
	}
	return false
}

// boundEntries returns the entries whose binding names id.
func boundEntries(items []Meta, id string) []Meta {
	var bound []Meta
	for _, item := range items {
		if item.Binding == id {
			bound = append(bound, item)
		}
	}
	return bound
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
