package harnesshook

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// ResolvedBinding carries the resolved issue ID, git directory, the resolution step,
// and (for path-resolved bindings) the worktree root directory.
type ResolvedBinding struct {
	IssueID        string
	GitDir         string
	ResolutionStep string // "file_path", "event_cwd", "session", or "" for unbound
	Root           string // Worktree root directory (only set for path-resolved bindings: file_path, event_cwd)
}

// ExtractFilePathFromToolInput extracts the file path from the raw tool_input map.
// It checks for common file path keys in the order they're likely to be used.
//
// For multi-file "changes" arrays, only the first entry's path is used for
// binding resolution, whereas scope checking (harnesspolicy.ScopePolicy /
// the Event.Paths built from extractPaths) evaluates every path in the array.
// This asymmetry is intentional and fail-safe, not a bug: if the first path
// resolves a binding but a later path in the same tool call falls outside
// that issue's declared scope, the scope check still blocks it — the extra
// paths can only ever cause additional blocking, never additional access
// that binding resolution didn't already grant.
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

// ReadIssueBindingFile reads the issue ID bound to gitDir, preferring the
// current armature-issue-id file and falling back to the legacy
// armature-task-id file for worktrees claimed before the rename (commit
// d52d78be). Returns "" if neither file exists, and also "" if either file
// exists but cannot be read (e.g. permission denied) — callers that need to
// distinguish "unbound" from "read failed" should use
// ReadIssueBindingFileErr instead. This is the single shared implementation
// behind every "read the binding for a git dir" call site
// (ResolveBindingFromDir, cmd/armature session/merged-gate resolution).
func ReadIssueBindingFile(gitDir string) string {
	issueID, _ := ReadIssueBindingFileErr(gitDir) //nolint:errcheck // error-swallowing variant kept for existing best-effort call sites
	return issueID
}

// ReadIssueBindingFileErr is the error-returning counterpart to
// ReadIssueBindingFile. It reads the issue ID bound to gitDir the same way
// (armature-issue-id, falling back to legacy armature-task-id), but returns a
// non-nil error when a binding file exists and could not be read for a reason
// other than "does not exist" (e.g. permission denied). Callers that must
// fail closed on such errors (rather than silently treating the worktree as
// unbound) should use this variant.
func ReadIssueBindingFileErr(gitDir string) (string, error) {
	issueIDPath := filepath.Join(gitDir, "armature-issue-id")
	data, err := os.ReadFile(issueIDPath) //nolint:gosec // G304: derived from a trusted git directory
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", issueIDPath, err)
	}

	taskIDPath := filepath.Join(gitDir, "armature-task-id")
	data, err = os.ReadFile(taskIDPath) //nolint:gosec // G304: derived from a trusted git directory
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", taskIDPath, err)
	}

	return "", nil
}

// ResolveBindingFromDir walks up the directory tree starting from dir to find
// the containing worktree's .git directory and reads the armature-issue-id file.
// Returns a ResolvedBinding with both the issue ID and the git directory where it was found,
// or an empty IssueID if no .git directory is found or if the armature-issue-id file doesn't exist.
// When a git dir is found but has no armature-issue-id file, GitDir is still populated
// (IssueID empty) so callers can distinguish "no worktree found" from "worktree found but unbound".
func ResolveBindingFromDir(dir string) (ResolvedBinding, error) {
	currentDir := dir

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
					// Report the worktree location even though the .git file couldn't
					// be read, so callers can log violations against the right
					// worktree instead of dropping the location entirely. GitDir falls
					// back to the .git file path itself since the real gitdir target
					// is unknown.
					return ResolvedBinding{GitDir: gitDir, Root: currentDir}, nil
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

			// Try to read armature-issue-id from the git dir, falling back to the
			// legacy armature-task-id file (worktrees claimed before rename d52d78be).
			if issueID := ReadIssueBindingFile(actualGitDir); issueID != "" {
				return ResolvedBinding{
					IssueID: issueID,
					GitDir:  actualGitDir,
					Root:    currentDir,
				}, nil
			}

			// Git dir exists but no armature-issue-id or armature-task-id file; stop searching, but report
			// the git dir found so callers can log against the correct worktree.
			return ResolvedBinding{GitDir: actualGitDir, Root: currentDir}, nil
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

// ResolveBindingFromFilePath walks up the directory tree from filePath to find
// the containing worktree's .git directory and reads the armature-issue-id file.
// It delegates to ResolveBindingFromDir by starting the walk from the file's parent directory.
func ResolveBindingFromFilePath(filePath string) (ResolvedBinding, error) {
	return ResolveBindingFromDir(filepath.Dir(filePath))
}

// isShellTool reports whether the given tool name identifies a shell tool from the
// supported list, which per ADR-0007 resolves at the session level (steps 3-4)
// rather than via path-based resolution. supportedShellTools is derived from the
// selected platform's PlatformCapabilities.SupportedShellTools.
func isShellTool(tool string, supportedShellTools []string) bool {
	return slices.Contains(supportedShellTools, tool)
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
// Shell tools (identified by supportedShellTools) and Stop events skip steps 1-2 and
// resolve at the session level only (steps 3-4). supportedShellTools should be derived
// from the selected platform's PlatformCapabilities.SupportedShellTools.
// Returns a ResolvedBinding with the issue ID, git directory, and resolution step (file_path, event_cwd, or session).
// When steps 1-2 locate a worktree git dir but it has no binding, that git dir is returned
// (IssueID empty, ResolutionStep empty) so callers can log a violation against the correct
// worktree rather than the session's git dir.
func ResolveBindingFromEvent(eventInfo *DecodedEventInfo, sessionBinding, sessionGitDir string, supportedShellTools []string) (ResolvedBinding, error) {
	sessionFallback := ResolvedBinding{
		IssueID:        sessionBinding,
		GitDir:         sessionGitDir,
		ResolutionStep: "session",
	}

	// Stop events and shell tools resolve at the session level only (steps 3-4)
	if eventInfo.Kind == EventStop || isShellTool(eventInfo.Tool, supportedShellTools) {
		return sessionFallback, nil
	}

	// For PreToolUse and PostToolUse events, follow the 4-step chain:
	if eventInfo.Kind == EventPreToolUse || eventInfo.Kind == EventPostToolUse {
		var unboundGitDir, unboundRoot string

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
				unboundRoot = pathBinding.Root
			}
		}

		// Step 2: Try path-based resolution from event-payload cwd
		if eventInfo.Cwd != "" && filepath.IsAbs(eventInfo.Cwd) {
			cwdBinding, err := ResolveBindingFromDir(eventInfo.Cwd)
			if err != nil {
				return ResolvedBinding{}, err
			}
			if cwdBinding.IssueID != "" {
				cwdBinding.ResolutionStep = "event_cwd"
				return cwdBinding, nil
			}
			if unboundGitDir == "" && cwdBinding.GitDir != "" {
				unboundGitDir = cwdBinding.GitDir
				unboundRoot = cwdBinding.Root
			}
		}

		// Steps 1-2 found a worktree but no binding: report that git dir so the
		// caller logs a violation in the worktree that actually contains the
		// unbound write, not the session's git dir (ADR-0007 / finding 1).
		// When returning an unbound path-resolved binding, also include the root
		// so callers can use it for evaluator setup if they later bind it. Root is
		// captured from the pathBinding/cwdBinding result already in hand above,
		// rather than re-walking the directory tree (finding P3).
		if unboundGitDir != "" {
			return ResolvedBinding{GitDir: unboundGitDir, Root: unboundRoot}, nil
		}
	}

	// Step 3: Fall back to session binding
	return sessionFallback, nil
}
