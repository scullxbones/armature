package harnesshook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveBindingFromFilePath_REQ_HOOKBIND_T2 verifies that ResolveBindingFromFilePath
// walks up from a file path to find the containing worktree's git dir and reads
// the armature-issue-id file.
func TestResolveBindingFromFilePath_REQ_HOOKBIND_T2(t *testing.T) {
	t.Parallel()
	// Create a temporary directory structure simulating a worktree
	tmpDir := t.TempDir()
	worktreeDir := filepath.Join(tmpDir, "worktree")
	actualGitDir := filepath.Join(tmpDir, "actual-git-dir")
	fileDir := filepath.Join(worktreeDir, "some", "deep", "path")
	filePath := filepath.Join(fileDir, "myfile.go")

	// Create the directories
	err := os.MkdirAll(actualGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	// Write a .git file that points to the actual git dir (like in a worktree)
	gitFileContent := "gitdir: " + actualGitDir + "\n"
	gitFile := filepath.Join(worktreeDir, ".git")
	err = os.WriteFile(gitFile, []byte(gitFileContent), 0o644)
	require.NoError(t, err)

	// Write armature-issue-id in the actual git dir
	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-path"), 0o644)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-from-path", binding)
}

// TestResolveBindingFromFilePath_NoGitDir verifies that ResolveBindingFromFilePath
// returns an empty string when no .git directory is found.
func TestResolveBindingFromFilePath_NoGitDir(t *testing.T) {
	t.Parallel()
	// Create a temporary directory without any git structure
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "some", "file.go")
	err := os.MkdirAll(filepath.Dir(filePath), 0o755)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "", binding)
}

// TestResolveBindingFromFilePath_NoIssueIDFile verifies that ResolveBindingFromFilePath
// returns an empty string when the git dir exists but armature-issue-id file doesn't.
func TestResolveBindingFromFilePath_NoIssueIDFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "", binding)
}

// TestResolveBindingFromFilePath_StopsAtFirstGitDir verifies that ResolveBindingFromFilePath
// stops at the first .git directory it finds when walking up the directory tree.
func TestResolveBindingFromFilePath_StopsAtFirstGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parentGitDir := filepath.Join(tmpDir, ".git")
	childDir := filepath.Join(tmpDir, "child")
	childGitDir := filepath.Join(childDir, ".git")
	fileDir := filepath.Join(childDir, "subdir")
	filePath := filepath.Join(fileDir, "file.go")

	// Create both .git directories
	err := os.MkdirAll(parentGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(childGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	// Write issue ID in child git dir only
	issueIDFile := filepath.Join(childGitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-child"), 0o644)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-from-child", binding)
}

// TestResolveBindingFromFilePath_TrimsWhitespace verifies that ResolveBindingFromFilePath
// trims whitespace from the armature-issue-id file content.
func TestResolveBindingFromFilePath_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	// Write issue ID with whitespace
	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("  task-with-spaces  \n"), 0o644)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-with-spaces", binding)
}

// TestResolveBindingFromEvent_PreToolUse_WithFilePath verifies that
// ResolveBindingFromEvent resolves binding from tool_input.file_path
// for PreToolUse events.
func TestResolveBindingFromEvent_PreToolUse_WithFilePath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-path"), 0o644)
	require.NoError(t, err)

	// Create a DecodedEventInfo with a file path
	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: filePath,
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding")

	require.NoError(t, err)
	assert.Equal(t, "task-from-path", binding)
}

// TestResolveBindingFromEvent_PreToolUse_NoFilePath_FallsBackToSession verifies that
// ResolveBindingFromEvent falls back to session binding when no file path is available
// for PreToolUse events.
func TestResolveBindingFromEvent_PreToolUse_NoFilePath_FallsBackToSession(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: "",
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding")

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding)
}

// TestResolveBindingFromEvent_Stop_UsesSessionBinding verifies that
// ResolveBindingFromEvent uses session binding for Stop events, ignoring any
// file paths.
func TestResolveBindingFromEvent_Stop_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-path"), 0o644)
	require.NoError(t, err)

	// Even though there's a file path, Stop events should use session binding
	eventInfo := &DecodedEventInfo{
		Kind:     EventStop,
		FilePath: filePath,
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding")

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding)
}

// TestResolveBindingFromEvent_Bash_UsesSessionBinding verifies that
// ResolveBindingFromEvent uses session binding for Bash events.
func TestResolveBindingFromEvent_Bash_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventKind("bash"), // Bash is not a PreToolUse/PostToolUse/Stop
		FilePath: "/some/path/file.go",
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding")

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding)
}

// TestResolveBindingFromEvent_EmptySessionBinding verifies that ResolveBindingFromEvent
// handles empty session bindings gracefully.
func TestResolveBindingFromEvent_EmptySessionBinding(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventStop,
		FilePath: "",
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "")

	require.NoError(t, err)
	assert.Equal(t, "", binding)
}
