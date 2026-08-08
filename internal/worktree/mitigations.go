package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NormalizePath resolves symlinks in path for reliable comparison, falling back
// to an absolute path when the path cannot be resolved (e.g. it does not exist
// yet). Both sides of any worktree-path comparison must go through this so a
// symlinked repo root (common on macOS, where /tmp -> /private/tmp) does not
// make an identical worktree look like two different paths.
func NormalizePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// NormalizePathAllowingMissing normalizes a path that may not exist on disk yet
// (e.g. a ghost worktree whose directory was removed), keeping the result
// symmetric with NormalizePath'd, EvalSymlinks-resolved managed roots. It resolves
// the nearest existing ancestor via EvalSymlinks and re-joins the missing tail, so
// a path reached through a symlinked repo root still shares its resolved prefix
// with the managed root. Plain NormalizePath cannot do this for a missing leaf:
// EvalSymlinks fails and the filepath.Abs fallback leaves the path symlinky,
// defeating a HasPrefix scope test against a resolved root.
func NormalizePathAllowingMissing(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	missing := ""
	cur := abs
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			if missing == "" {
				return resolved
			}
			return filepath.Join(resolved, missing)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs // reached the filesystem root without resolving anything
		}
		missing = filepath.Join(filepath.Base(cur), missing)
		cur = parent
	}
}

// ApplyMitigations applies best-effort project-isolation for a newly provisioned
// worktree. Its sole job is to keep the MAIN tree's tooling from walking the
// worktree: if the main tree uses a go.work file, the worktree is removed from
// its `use` directives so the main tree's gopls does not treat the worktree as
// part of the same workspace.
//
// If the main tree has no go.work (the common case — this repo has none), it is
// a no-op: the worktree is already isolated because .worktrees/ is gitignored.
// It NEVER creates a go.work file, in the worktree or anywhere else — a bare
// go.work with no `use` would break `go build ./...` inside the worktree.
func ApplyMitigations(repoRoot, worktreeRoot string) error {
	goWorkPath := filepath.Join(repoRoot, "go.work")
	info, err := os.Stat(goWorkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no main-tree go.work: worktree is already isolated
		}
		return fmt.Errorf("stat main go.work: %w", err)
	}

	// #nosec G304 - repoRoot is internal, not user-controlled
	content, err := os.ReadFile(goWorkPath)
	if err != nil {
		return fmt.Errorf("read main go.work: %w", err)
	}

	newContent, changed := removeWorktreeFromGoWork(string(content), repoRoot, worktreeRoot)
	if !changed {
		return nil
	}

	if err := os.WriteFile(goWorkPath, []byte(newContent), info.Mode().Perm()); err != nil {
		return fmt.Errorf("rewrite main go.work: %w", err)
	}
	return nil
}

// removeWorktreeFromGoWork returns go.work content with any `use` directive that
// resolves to worktreeRoot removed. It handles both the block form
// (use (\n\t./path\n)) and the single-line form (use ./path). The second return
// value reports whether anything was removed.
func removeWorktreeFromGoWork(content, repoRoot, worktreeRoot string) (string, bool) {
	target := NormalizePath(worktreeRoot)
	changed := false
	inBlock := false
	var out []string

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inBlock && strings.HasPrefix(trimmed, "use ("):
			inBlock = true
			out = append(out, line)
		case inBlock && trimmed == ")":
			inBlock = false
			out = append(out, line)
		case inBlock:
			if useDirectiveMatches(trimmed, repoRoot, target) {
				changed = true
				continue
			}
			out = append(out, line)
		case strings.HasPrefix(trimmed, "use "):
			if useDirectiveMatches(strings.TrimSpace(strings.TrimPrefix(trimmed, "use")), repoRoot, target) {
				changed = true
				continue
			}
			out = append(out, line)
		default:
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n"), changed
}

// useDirectiveMatches reports whether a `use` path (relative to repoRoot or
// absolute) resolves to the target worktree path.
func useDirectiveMatches(p, repoRoot, target string) bool {
	if p == "" {
		return false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	return NormalizePath(p) == target
}
