package harnesshook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilePathWalkUpResolvesWorktreeBinding_REQ_HOOKBIND_T2 verifies that ResolveBindingFromFilePath
// walks up from a file path to find the containing worktree's git dir and reads
// the armature-issue-id file, and returns both the issue ID and the git dir.
func TestFilePathWalkUpResolvesWorktreeBinding_REQ_HOOKBIND_T2(t *testing.T) {
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
	assert.Equal(t, "task-from-path", binding.IssueID)
	assert.Equal(t, actualGitDir, binding.GitDir)
	assert.Equal(t, worktreeDir, binding.Root, "Root should be the worktree root, not the gitdir parent (finding P3)")
}

// TestResolveBindingFromFilePath_LinkedWorktree_RootIsWorktreeRoot verifies that
// for a linked-worktree layout (.git is a file pointing elsewhere, e.g.
// <parent>/.git/worktrees/<name>), ResolvedBinding.Root is the worktree root
// directory itself, not the parent of the actual git dir the .git file points to.
func TestResolveBindingFromFilePath_LinkedWorktree_RootIsWorktreeRoot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parentRepo := filepath.Join(tmpDir, "parent-repo")
	worktreeDir := filepath.Join(tmpDir, "linked-worktree")
	actualGitDir := filepath.Join(parentRepo, ".git", "worktrees", "linked-worktree")
	fileDir := filepath.Join(worktreeDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	require.NoError(t, os.MkdirAll(actualGitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))

	gitFile := filepath.Join(worktreeDir, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: "+actualGitDir+"\n"), 0o644))

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(issueIDFile, []byte("linked-worktree-task"), 0o644))

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "linked-worktree-task", binding.IssueID)
	assert.Equal(t, actualGitDir, binding.GitDir)
	assert.Equal(t, worktreeDir, binding.Root, "Root must be the worktree root (where .git lives), not actualGitDir's parent")
}

// TestResolveBindingFromFilePath_NoGitDir verifies that ResolveBindingFromFilePath
// returns a ResolvedBinding with empty IssueID when no .git directory is found.
func TestResolveBindingFromFilePath_NoGitDir(t *testing.T) {
	t.Parallel()
	// Create a temporary directory without any git structure
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "some", "file.go")
	err := os.MkdirAll(filepath.Dir(filePath), 0o755)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
}

// TestResolveBindingFromFilePath_NoIssueIDFile verifies that ResolveBindingFromFilePath
// returns a ResolvedBinding with empty IssueID when the git dir exists but armature-issue-id file doesn't.
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
	assert.Equal(t, "", binding.IssueID)
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
	assert.Equal(t, "task-from-child", binding.IssueID)
	assert.Equal(t, childGitDir, binding.GitDir)
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
	assert.Equal(t, "task-with-spaces", binding.IssueID)
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

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

	require.NoError(t, err)
	assert.Equal(t, "task-from-path", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
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
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

// TestStopEventUsesSessionBinding_REQ_HOOKBIND_T2 verifies that
// ResolveBindingFromEvent uses session binding for Stop events, ignoring any
// file paths.
func TestStopEventUsesSessionBinding_REQ_HOOKBIND_T2(t *testing.T) {
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
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

// TestResolveBindingFromEvent_Bash_UsesSessionBinding verifies that
// ResolveBindingFromEvent uses session binding for Bash events.
func TestResolveBindingFromEvent_Bash_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse, // Bash arrives as PreToolUse with Tool=="Bash"
		Tool:     "Bash",
		FilePath: "/some/path/file.go",
	}
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

// TestResolveBindingFromEvent_EmptySessionBinding verifies that ResolveBindingFromEvent
// handles empty session bindings gracefully.
func TestResolveBindingFromEvent_EmptySessionBinding(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventStop,
		FilePath: "",
	}
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(eventInfo, "", sessionGitDir)

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
}

// TestBindingResolutionChain_REQ_HOOKBIND_T2 verifies the complete 4-step binding resolution
// chain per ADR-0007: (1) file_path walk-up, (2) event-payload cwd, (3) session binding, (4) env.
// This test exercises the most-specific-first priority and ensures event-payload cwd is consulted
// between file_path and session binding for PreToolUse/PostToolUse events.
func TestBindingResolutionChain_REQ_HOOKBIND_T2(t *testing.T) {
	t.Parallel()

	t.Run("Step1_FilePathResolvesToBinding", func(t *testing.T) {
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
		err = os.WriteFile(issueIDFile, []byte("from-file-path"), 0o644)
		require.NoError(t, err)

		// Step 1 should resolve from file path, ignoring session binding
		eventInfo := &DecodedEventInfo{
			Kind:     EventPreToolUse,
			FilePath: filePath,
			Cwd:      "",
		}

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

		require.NoError(t, err)
		assert.Equal(t, "from-file-path", binding.IssueID)
		assert.Equal(t, gitDir, binding.GitDir)
	})

	t.Run("Step2_EventPayloadCwdResolvesToBinding", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		cwdDir := filepath.Join(tmpDir, "event-cwd", "path")

		err := os.MkdirAll(gitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(cwdDir, 0o755)
		require.NoError(t, err)

		issueIDFile := filepath.Join(gitDir, "armature-issue-id")
		err = os.WriteFile(issueIDFile, []byte("from-event-cwd"), 0o644)
		require.NoError(t, err)

		// Step 2: event-payload cwd should resolve when file_path is empty
		eventInfo := &DecodedEventInfo{
			Kind:     EventPreToolUse,
			FilePath: "",
			Cwd:      cwdDir,
		}

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

		require.NoError(t, err)
		assert.Equal(t, "from-event-cwd", binding.IssueID)
		assert.Equal(t, gitDir, binding.GitDir)
	})

	t.Run("Step2_FilePathTakesPrecedenceOverEventCwd", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		filePathGitDir := filepath.Join(tmpDir, "file-git", ".git")
		eventCwdGitDir := filepath.Join(tmpDir, "cwd-git", ".git")
		fileDir := filepath.Join(tmpDir, "file-git", "some", "path")
		cwdDir := filepath.Join(tmpDir, "cwd-git", "some", "path")
		filePath := filepath.Join(fileDir, "file.go")

		err := os.MkdirAll(filePathGitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(eventCwdGitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(fileDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(cwdDir, 0o755)
		require.NoError(t, err)

		// Write different issue IDs to both git dirs
		fileIssueIDFile := filepath.Join(filePathGitDir, "armature-issue-id")
		err = os.WriteFile(fileIssueIDFile, []byte("from-file-path"), 0o644)
		require.NoError(t, err)

		cwdIssueIDFile := filepath.Join(eventCwdGitDir, "armature-issue-id")
		err = os.WriteFile(cwdIssueIDFile, []byte("from-event-cwd"), 0o644)
		require.NoError(t, err)

		// Step 1 (file_path) should take precedence over step 2 (event cwd)
		eventInfo := &DecodedEventInfo{
			Kind:     EventPreToolUse,
			FilePath: filePath,
			Cwd:      cwdDir,
		}

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

		require.NoError(t, err)
		assert.Equal(t, "from-file-path", binding.IssueID, "file_path should take precedence over event cwd")
		assert.Equal(t, filePathGitDir, binding.GitDir)
	})

	t.Run("Step3_SessionBindingFallback", func(t *testing.T) {
		t.Parallel()
		// No file path, no event cwd -> should use session binding
		eventInfo := &DecodedEventInfo{
			Kind:     EventPreToolUse,
			FilePath: "",
			Cwd:      "",
		}
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

		require.NoError(t, err)
		assert.Equal(t, "session-binding", binding.IssueID)
		assert.Equal(t, sessionGitDir, binding.GitDir)
	})

	t.Run("BashEventUsesSessionBindingOnly", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		cwdDir := filepath.Join(tmpDir, "cwd", "path")
		fileDir := filepath.Join(tmpDir, "some", "path")
		filePath := filepath.Join(fileDir, "file.go")

		err := os.MkdirAll(gitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(cwdDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(fileDir, 0o755)
		require.NoError(t, err)

		issueIDFile := filepath.Join(gitDir, "armature-issue-id")
		err = os.WriteFile(issueIDFile, []byte("from-path"), 0o644)
		require.NoError(t, err)

		// Bash events should ignore both file_path and event cwd
		eventInfo := &DecodedEventInfo{
			Kind:     EventPreToolUse,
			Tool:     "Bash",
			FilePath: filePath,
			Cwd:      cwdDir,
		}
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

		require.NoError(t, err)
		assert.Equal(t, "session-binding", binding.IssueID, "bash events should use session binding only")
		assert.Equal(t, sessionGitDir, binding.GitDir)
	})

	t.Run("StopEventUsesSessionBindingOnly", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		cwdDir := filepath.Join(tmpDir, "cwd", "path")
		fileDir := filepath.Join(tmpDir, "some", "path")
		filePath := filepath.Join(fileDir, "file.go")

		err := os.MkdirAll(gitDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(cwdDir, 0o755)
		require.NoError(t, err)
		err = os.MkdirAll(fileDir, 0o755)
		require.NoError(t, err)

		issueIDFile := filepath.Join(gitDir, "armature-issue-id")
		err = os.WriteFile(issueIDFile, []byte("from-path"), 0o644)
		require.NoError(t, err)

		// Stop events should ignore both file_path and event cwd
		eventInfo := &DecodedEventInfo{
			Kind:     EventStop,
			FilePath: filePath,
			Cwd:      cwdDir,
		}
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

		require.NoError(t, err)
		assert.Equal(t, "session-binding", binding.IssueID, "stop events should use session binding only")
		assert.Equal(t, sessionGitDir, binding.GitDir)
	})
}

// TestResolveBindingFromEvent_RelativeFilePath_JoinsWithEventCwd verifies that a
// relative tool_input.file_path is resolved against the event-payload cwd rather
// than the hook process's own (untrusted) working directory (finding 5).
func TestResolveBindingFromEvent_RelativeFilePath_JoinsWithEventCwd(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	worktreeDir := filepath.Join(tmpDir, "worktree")
	fileDir := filepath.Join(worktreeDir, "some", "path")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-issue-id"), []byte("task-relative"), 0o644))

	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: filepath.Join("some", "path", "file.go"), // relative
		Cwd:      worktreeDir,
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

	require.NoError(t, err)
	assert.Equal(t, "task-relative", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
}

// TestResolveBindingFromEvent_RelativeFilePath_NoCwdFallsBackToSession verifies that
// a relative file_path with no event cwd available does not get resolved against the
// hook process cwd; it falls through to session binding instead.
func TestResolveBindingFromEvent_RelativeFilePath_NoCwdFallsBackToSession(t *testing.T) {
	t.Parallel()
	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: "some/relative/file.go",
	}
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", sessionGitDir)

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

// TestResolveBindingFromEvent_UnboundWorktree_ReturnsWorktreeGitDir verifies that
// when the file path's worktree is found but has no armature-issue-id binding, the
// worktree's own git dir is returned (not the session git dir), so violations get
// logged where `arm merged` actually looks (finding 1).
func TestResolveBindingFromEvent_UnboundWorktree_ReturnsWorktreeGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))
	// No armature-issue-id file written: worktree exists but is unbound.

	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: filePath,
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir, "should return the unbound worktree's git dir, not the session git dir")
}

// TestResolveBindingFromFilePath_FallsBackToLegacyTaskIDFile verifies that
// ResolveBindingFromFilePath falls back to reading armature-task-id when
// armature-issue-id doesn't exist, for compatibility with worktrees claimed
// before the binding file was renamed (commit d52d78be).
func TestResolveBindingFromFilePath_FallsBackToLegacyTaskIDFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	// Write legacy armature-task-id file only (no armature-issue-id)
	taskIDFile := filepath.Join(gitDir, "armature-task-id")
	err = os.WriteFile(taskIDFile, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "legacy-task-id", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
}

// TestResolveBindingFromFilePath_PrefersIssueIDOverTaskID verifies that
// when both armature-issue-id and armature-task-id exist, armature-issue-id
// takes precedence.
func TestResolveBindingFromFilePath_PrefersIssueIDOverTaskID(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	// Write both files
	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("new-issue-id"), 0o644)
	require.NoError(t, err)

	taskIDFile := filepath.Join(gitDir, "armature-task-id")
	err = os.WriteFile(taskIDFile, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	binding, err := ResolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "new-issue-id", binding.IssueID, "armature-issue-id should take precedence")
	assert.Equal(t, gitDir, binding.GitDir)
}

// TestResolveBindingFromEvent_EventCwdAtWorktreeRoot_ResolvesBinding verifies that
// when event Cwd is the worktree root (step 2 of the resolution chain), the binding
// is found at <cwd>/.git/armature-issue-id. This test catches the bug where
// ResolveBindingFromFilePath(cwd) would do filepath.Dir(cwd), skipping the root's
// own .git directory (finding P2).
func TestResolveBindingFromEvent_EventCwdAtWorktreeRoot_ResolvesBinding(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	worktreeRoot := tmpDir // cwd is the worktree root itself
	gitDir := filepath.Join(worktreeRoot, ".git")

	// Create .git directory and armature-issue-id file at the worktree root
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("issue-at-root"), 0o644)
	require.NoError(t, err)

	// Create event with Cwd = worktree root, no FilePath (step 2 resolution)
	eventInfo := &DecodedEventInfo{
		Kind:     EventPreToolUse,
		FilePath: "",
		Cwd:      worktreeRoot,
		Tool:     "Edit", // non-shell tool
	}

	binding, err := ResolveBindingFromEvent(eventInfo, "session-binding", "/session/git/dir")

	require.NoError(t, err)
	assert.Equal(t, "issue-at-root", binding.IssueID, "should find binding at event cwd worktree root")
	assert.Equal(t, gitDir, binding.GitDir)
	assert.Equal(t, "event_cwd", binding.ResolutionStep, "should resolve via event_cwd step")
}

// TestResolveBindingFromDir_UnreadableGitFile_ReportsBestEffortLocation verifies
// that when the .git file in a worktree exists but can't be read (e.g. permission
// denied), ResolveBindingFromDir still reports the discovered worktree location
// (GitDir/Root) instead of dropping it, so callers can log violations against the
// right worktree (finding P3).
func TestResolveBindingFromDir_UnreadableGitFile_ReportsBestEffortLocation(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: chmod 000 does not prevent root from reading files")
	}
	t.Parallel()
	tmpDir := t.TempDir()
	gitFile := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: /somewhere\n"), 0o644))
	require.NoError(t, os.Chmod(gitFile, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(gitFile, 0o644) //nolint:errcheck // best-effort cleanup so TempDir removal doesn't fail
	})

	binding, err := ResolveBindingFromDir(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
	assert.Equal(t, gitFile, binding.GitDir, "should report the .git file location even though it couldn't be read")
	assert.Equal(t, tmpDir, binding.Root)
}
