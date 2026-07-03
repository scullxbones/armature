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
}

// ResolveBindingFromFilePath walks up the directory tree from filePath to find
// the containing worktree's .git directory and reads the armature-issue-id file.
// Returns an empty string if no .git directory is found or if the armature-issue-id
// file doesn't exist.
func ResolveBindingFromFilePath(filePath string) (string, error) {
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
					return "", nil
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
				return strings.TrimSpace(string(data)), nil
			}

			// Git dir exists but no armature-issue-id file; stop searching
			return "", nil
		}

		// Move up to parent directory
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached the filesystem root; no git dir found
			return "", nil
		}
		currentDir = parent
	}
}

// ResolveBindingFromEvent resolves the binding based on the decoded event and
// session binding, following the ADR-0007 4-step priority chain:
// 1. For PreToolUse/PostToolUse events: walk up from tool_input.file_path to find armature-issue-id
// 2. For PreToolUse/PostToolUse events: walk up from event-payload cwd to find armature-issue-id
// 3. Hook process cwd / session binding (fallback)
// 4. ARMATURE_ISSUE_ID environment variable (handled by caller if needed)
//
// Bash and Stop events skip steps 1-2 and resolve at the session level only (steps 3-4).
func ResolveBindingFromEvent(eventInfo *DecodedEventInfo, sessionBinding string) (string, error) {
	// Bash and Stop events resolve at the session level only (steps 3-4)
	if eventInfo.Kind == EventStop || eventInfo.Kind == EventKind("bash") {
		return sessionBinding, nil
	}

	// For PreToolUse and PostToolUse events, follow the 4-step chain:
	if eventInfo.Kind == EventPreToolUse || eventInfo.Kind == EventPostToolUse {
		// Step 1: Try path-based resolution from tool_input.file_path
		if eventInfo.FilePath != "" {
			pathBinding, err := ResolveBindingFromFilePath(eventInfo.FilePath)
			if err != nil {
				return "", err
			}
			if pathBinding != "" {
				return pathBinding, nil
			}
		}

		// Step 2: Try path-based resolution from event-payload cwd
		if eventInfo.Cwd != "" {
			cwdBinding, err := ResolveBindingFromFilePath(eventInfo.Cwd)
			if err != nil {
				return "", err
			}
			if cwdBinding != "" {
				return cwdBinding, nil
			}
		}
	}

	// Step 3: Fall back to session binding
	return sessionBinding, nil
}
