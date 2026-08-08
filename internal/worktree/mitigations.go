package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	for _, rawLine := range strings.SplitAfter(content, "\n") {
		line := strings.TrimSuffix(rawLine, "\n")
		line = strings.TrimSuffix(line, "\r")
		code := strings.TrimSpace(stripGoWorkComment(line))
		if !inBlock && isUseBlockStart(code) {
			inBlock = true
			out = append(out, rawLine)
			continue
		}
		if inBlock && code == ")" {
			inBlock = false
			out = append(out, rawLine)
			continue
		}
		if path, ok := parseGoWorkUsePath(code, inBlock); ok && useDirectiveMatches(path, repoRoot, target) {
			changed = true
			continue
		}
		out = append(out, rawLine)
	}

	return strings.Join(out, ""), changed
}

func stripGoWorkComment(line string) string {
	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '`' {
			quote = c
			continue
		}
		if c == '/' && i+1 < len(line) && line[i+1] == '/' {
			return line[:i]
		}
	}
	return line
}

func isUseBlockStart(code string) bool {
	return strings.HasPrefix(code, "use") && strings.TrimSpace(code[len("use"):]) == "("
}

func parseGoWorkUsePath(code string, inBlock bool) (string, bool) {
	if code == "" || code == ")" {
		return "", false
	}
	if !inBlock {
		if !strings.HasPrefix(code, "use") {
			return "", false
		}
		if len(code) > len("use") && code[len("use")] != ' ' && code[len("use")] != '\t' {
			return "", false
		}
		code = strings.TrimSpace(code[len("use"):])
		if code == "" || strings.HasPrefix(code, "(") {
			return "", false
		}
	}
	path, _, ok := parseGoWorkToken(code)
	return path, ok
}

func parseGoWorkToken(code string) (string, string, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", "", false
	}
	if code[0] == '"' || code[0] == '`' {
		quote := code[0]
		escaped := false
		for i := 1; i < len(code); i++ {
			if escaped {
				escaped = false
				continue
			}
			if quote == '"' && code[i] == '\\' {
				escaped = true
				continue
			}
			if code[i] == quote {
				raw := code[:i+1]
				value, err := strconv.Unquote(raw)
				if err != nil {
					return "", "", false
				}
				return value, code[i+1:], true
			}
		}
		return "", "", false
	}
	end := len(code)
	for i, r := range code {
		if r == ' ' || r == '\t' {
			end = i
			break
		}
	}
	return code[:end], code[end:], true
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
