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
// session binding, following the ADR-0007 priority:
// 1. For PreToolUse/PostToolUse events with a file path: walk up from the file path
// 2. For Bash and Stop events: use session binding only
// 3. Fall back to session binding if no file path is available
func ResolveBindingFromEvent(eventInfo *DecodedEventInfo, sessionBinding string) (string, error) {
	// Bash and Stop events resolve at the session level only
	if eventInfo.Kind == EventStop || eventInfo.Kind == EventKind("bash") {
		return sessionBinding, nil
	}

	// For PreToolUse and PostToolUse events, try path-based resolution first
	if eventInfo.Kind == EventPreToolUse || eventInfo.Kind == EventPostToolUse {
		if eventInfo.FilePath != "" {
			pathBinding, err := ResolveBindingFromFilePath(eventInfo.FilePath)
			if err != nil {
				return "", err
			}
			if pathBinding != "" {
				return pathBinding, nil
			}
		}
	}

	// Fall back to session binding
	return sessionBinding, nil
}
