package harnesshook

import (
	"os"
	"path/filepath"
	"strings"
)

// DecodedEventInfo carries the minimal information extracted from a decoded
// hook event needed for binding resolution.
type DecodedEventInfo struct {
	Kind     EventKind
	FilePath string
	Cwd      string // Current working directory from the hook event payload (for step 2 of resolution chain)
	Tool     string // Tool name from the event payload (e.g. "Bash"); used to detect shell events
}

// ResolvedBinding carries the resolved issue ID, git directory, and the resolution step
// that determined the binding (file_path, event_cwd, session, or none for unbound).
type ResolvedBinding struct {
	IssueID        string
	GitDir         string
	ResolutionStep string // "file_path", "event_cwd", "session", or "" for unbound
}

// ExtractFilePathFromToolInput extracts the file path from the raw tool_input map.
// It checks for common file path keys in the order they're likely to be used.
func ExtractFilePathFromToolInput(toolInput map[string]any) string {
	if toolInput == nil {
		return ""
	}

	// Check for direct file_path or path keys
	for _, key := range []string{"file_path", "path"} {
		if value, ok := toolInput[key].(string); ok && value != "" {
			return value
		}
	}

	// Check for changes array (common in Edit/Write events)
	if changes, ok := toolInput["changes"].([]any); ok && len(changes) > 0 {
		if change, ok := changes[0].(map[string]any); ok {
			if path, ok := change["path"].(string); ok && path != "" {
				return path
			}
		}
	}

	return ""
}

// ResolveBindingFromFilePath walks up the directory tree from filePath to find
// the containing worktree's .git directory and reads the armature-issue-id file.
// Returns a ResolvedBinding with both the issue ID and the git directory where it was found,
// or an empty IssueID if no .git directory is found or if the armature-issue-id file doesn't exist.
// When a git dir is found but has no armature-issue-id file, GitDir is still populated
// (IssueID empty) so callers can distinguish "no worktree found" from "worktree found but unbound".
func ResolveBindingFromFilePath(filePath string) (ResolvedBinding, error) {
	currentDir := filepath.Dir(filePath)

	for {
		gitDir := filepath.Join(currentDir, ".git")

		// Check if .git exists and is a directory (regular git repo) or a file (worktree)
		info, err := os.Stat(gitDir)
		if err == nil {
			var actualGitDir string
			if info.IsDir() {
				// Regular git repo: .git is a directory
				actualGitDir = gitDir
			} else {
				// Worktree: .git is a file containing "gitdir: <path>"
				data, err := os.ReadFile(gitDir) //nolint:gosec // G304: derived from repo structure
				if err != nil {
					return ResolvedBinding{}, nil
				}
				gitdirLine := strings.TrimSpace(string(data))
				gitdirLine = strings.TrimPrefix(gitdirLine, "gitdir: ")
				if !filepath.IsAbs(gitdirLine) {
					// Relative paths in worktree .git file are relative to the worktree root
					actualGitDir = filepath.Join(currentDir, gitdirLine)
				} else {
					actualGitDir = gitdirLine
				}
			}

			// Try to read armature-issue-id from the git dir
			issueIDPath := filepath.Join(actualGitDir, "armature-issue-id")
			if data, err := os.ReadFile(issueIDPath); err == nil { //nolint:gosec // G304: derived from git dir
				return ResolvedBinding{
					IssueID: strings.TrimSpace(string(data)),
					GitDir:  actualGitDir,
				}, nil
			}

			// Git dir exists but no armature-issue-id file; stop searching, but report
			// the git dir found so callers can log against the correct worktree.
			return ResolvedBinding{GitDir: actualGitDir}, nil
		}

		// Move up to parent directory
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached the filesystem root; no git dir found
			return ResolvedBinding{}, nil
		}
		currentDir = parent
	}
}

// isShellTool reports whether the given tool name identifies a shell/bash tool,
// which per ADR-0007 resolves at the session level (steps 3-4) rather than via
// path-based resolution.
func isShellTool(tool string) bool {
	return tool == "Bash"
}

// absolutizeFilePath resolves filePath against cwd if filePath is relative.
// Returns ok=false if filePath is relative and cwd is unavailable or itself
// relative (untrustworthy as a resolution base).
func absolutizeFilePath(filePath, cwd string) (string, bool) {
	if filePath == "" {
		return "", false
	}
	if filepath.IsAbs(filePath) {
		return filePath, true
	}
	if cwd == "" || !filepath.IsAbs(cwd) {
		return "", false
	}
	return filepath.Join(cwd, filePath), true
}

// ResolveBindingFromEvent resolves the binding based on the decoded event and
// session binding, following the ADR-0007 4-step priority chain:
// 1. For PreToolUse/PostToolUse events (non-shell): walk up from tool_input.file_path to find armature-issue-id
// 2. For PreToolUse/PostToolUse events (non-shell): walk up from event-payload cwd to find armature-issue-id
// 3. Hook process cwd / session binding (fallback)
// 4. ARMATURE_ISSUE_ID environment variable (handled by caller if needed)
//
// Bash (shell) and Stop events skip steps 1-2 and resolve at the session level only (steps 3-4).
// Returns a ResolvedBinding with the issue ID, git directory, and resolution step (file_path, event_cwd, or session).
// When steps 1-2 locate a worktree git dir but it has no binding, that git dir is returned
// (IssueID empty, ResolutionStep empty) so callers can log a violation against the correct
// worktree rather than the session's git dir.
func ResolveBindingFromEvent(eventInfo *DecodedEventInfo, sessionBinding, sessionGitDir string) (ResolvedBinding, error) {
	sessionFallback := ResolvedBinding{
		IssueID:        sessionBinding,
		GitDir:         sessionGitDir,
		ResolutionStep: "session",
	}

	// Bash/shell and Stop events resolve at the session level only (steps 3-4)
	if eventInfo.Kind == EventStop || isShellTool(eventInfo.Tool) {
		return sessionFallback, nil
	}

	// For PreToolUse and PostToolUse events, follow the 4-step chain:
	if eventInfo.Kind == EventPreToolUse || eventInfo.Kind == EventPostToolUse {
		var unboundGitDir string

		// Step 1: Try path-based resolution from tool_input.file_path
		if abs, ok := absolutizeFilePath(eventInfo.FilePath, eventInfo.Cwd); ok {
			pathBinding, err := ResolveBindingFromFilePath(abs)
			if err != nil {
				return ResolvedBinding{}, err
			}
			if pathBinding.IssueID != "" {
				pathBinding.ResolutionStep = "file_path"
				return pathBinding, nil
			}
			if pathBinding.GitDir != "" {
				unboundGitDir = pathBinding.GitDir
			}
		}

		// Step 2: Try path-based resolution from event-payload cwd
		if eventInfo.Cwd != "" && filepath.IsAbs(eventInfo.Cwd) {
			cwdBinding, err := ResolveBindingFromFilePath(eventInfo.Cwd)
			if err != nil {
				return ResolvedBinding{}, err
			}
			if cwdBinding.IssueID != "" {
				cwdBinding.ResolutionStep = "event_cwd"
				return cwdBinding, nil
			}
			if unboundGitDir == "" && cwdBinding.GitDir != "" {
				unboundGitDir = cwdBinding.GitDir
			}
		}

		// Steps 1-2 found a worktree but no binding: report that git dir so the
		// caller logs a violation in the worktree that actually contains the
		// unbound write, not the session's git dir (ADR-0007 / finding 1).
		if unboundGitDir != "" {
			return ResolvedBinding{GitDir: unboundGitDir}, nil
		}
	}

	// Step 3: Fall back to session binding
	return sessionFallback, nil
}
