package harnesshook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bindingEvent(kind EventKind, tool, filePath, cwd string) Event {
	ev := Event{Kind: kind, Tool: tool, Cwd: cwd}
	if filePath != "" {
		ev.ToolInput = map[string]any{"file_path": filePath}
	}
	return ev
}

func TestFilePathWalkUpResolvesWorktreeBinding_REQ_HOOKBIND_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	worktreeDir := filepath.Join(tmpDir, "worktree")
	actualGitDir := filepath.Join(tmpDir, "actual-git-dir")
	fileDir := filepath.Join(worktreeDir, "some", "deep", "path")
	filePath := filepath.Join(fileDir, "myfile.go")

	err := os.MkdirAll(actualGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	gitFileContent := "gitdir: " + actualGitDir + "\n"
	gitFile := filepath.Join(worktreeDir, ".git")
	err = os.WriteFile(gitFile, []byte(gitFileContent), 0o644)
	require.NoError(t, err)

	issueIDFile := filepath.Join(actualGitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-path"), 0o644)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-from-path", binding.IssueID)
	assert.Equal(t, actualGitDir, binding.GitDir)
	assert.Equal(t, worktreeDir, binding.Root, "Root should be the worktree root, not the gitdir parent (finding P3)")
}

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

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "linked-worktree-task", binding.IssueID)
	assert.Equal(t, actualGitDir, binding.GitDir)
	assert.Equal(t, worktreeDir, binding.Root, "Root must be the worktree root (where .git lives), not actualGitDir's parent")
}

func TestResolveBindingFromFilePath_NoGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "some", "file.go")
	err := os.MkdirAll(filepath.Dir(filePath), 0o755)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
}

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

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
}

func TestResolveBindingFromFilePath_StopsAtFirstGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	parentGitDir := filepath.Join(tmpDir, ".git")
	childDir := filepath.Join(tmpDir, "child")
	childGitDir := filepath.Join(childDir, ".git")
	fileDir := filepath.Join(childDir, "subdir")
	filePath := filepath.Join(fileDir, "file.go")

	err := os.MkdirAll(parentGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(childGitDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(fileDir, 0o755)
	require.NoError(t, err)

	issueIDFile := filepath.Join(childGitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("task-from-child"), 0o644)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-from-child", binding.IssueID)
	assert.Equal(t, childGitDir, binding.GitDir)
}

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

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("  task-with-spaces  \n"), 0o644)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "task-with-spaces", binding.IssueID)
}

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

	event := bindingEvent(EventPreToolUse, "", filePath, "")

	binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "task-from-path", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_PreToolUse_NoFilePath_FallsBackToSession(t *testing.T) {
	t.Parallel()
	event := bindingEvent(EventPreToolUse, "", "", "")
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

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

	event := bindingEvent(EventStop, "", filePath, "")
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_Bash_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	event := bindingEvent(EventPreToolUse, "Bash", "/some/path/file.go", "")
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_DevinExec_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	worktreeDir := filepath.Join(tmpDir, "worktree")
	worktreeGitDir := filepath.Join(tmpDir, "worktree-git-dir")
	fileDir := filepath.Join(worktreeDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	require.NoError(t, os.MkdirAll(worktreeGitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))

	gitFile := filepath.Join(worktreeDir, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: "+worktreeGitDir+"\n"), 0o644))

	issueIDFile := filepath.Join(worktreeGitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(issueIDFile, []byte("worktree-binding"), 0o644))

	event := bindingEvent(EventPreToolUse, "exec", filePath, "")
	sessionGitDir := "/session/git/dir"

	devinAdapter := newDevinAdapter()
	devinShellTools := devinAdapter.Capabilities().SupportedShellTools

	binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, devinShellTools)

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID, "exec tool should skip path-based resolution and use session binding")
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_CodexShell_UsesSessionBinding(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	worktreeDir := filepath.Join(tmpDir, "worktree")
	worktreeGitDir := filepath.Join(tmpDir, "worktree-git-dir")
	fileDir := filepath.Join(worktreeDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")

	require.NoError(t, os.MkdirAll(worktreeGitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))

	gitFile := filepath.Join(worktreeDir, ".git")
	require.NoError(t, os.WriteFile(gitFile, []byte("gitdir: "+worktreeGitDir+"\n"), 0o644))

	issueIDFile := filepath.Join(worktreeGitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(issueIDFile, []byte("worktree-binding"), 0o644))

	codexAdapter := newCodexAdapter()
	codexShellTools := codexAdapter.Capabilities().SupportedShellTools
	require.Contains(t, codexShellTools, "shell")
	require.Contains(t, codexShellTools, "local_shell")

	sessionGitDir := "/session/git/dir"

	for _, tool := range []string{"shell", "local_shell"} {
		event := bindingEvent(EventPreToolUse, tool, filePath, "")

		binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, codexShellTools)

		require.NoError(t, err)
		assert.Equal(t, "session-binding", binding.IssueID, "%s tool should skip path-based resolution and use session binding", tool)
		assert.Equal(t, sessionGitDir, binding.GitDir)
	}
}

func TestResolveBindingFromEvent_EmptySessionBinding(t *testing.T) {
	t.Parallel()
	event := bindingEvent(EventStop, "", "", "")
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(event, "", sessionGitDir, []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
}

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

		event := bindingEvent(EventPreToolUse, "", filePath, "")

		binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

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

		event := bindingEvent(EventPreToolUse, "", "", cwdDir)

		binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

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

		fileIssueIDFile := filepath.Join(filePathGitDir, "armature-issue-id")
		err = os.WriteFile(fileIssueIDFile, []byte("from-file-path"), 0o644)
		require.NoError(t, err)

		cwdIssueIDFile := filepath.Join(eventCwdGitDir, "armature-issue-id")
		err = os.WriteFile(cwdIssueIDFile, []byte("from-event-cwd"), 0o644)
		require.NoError(t, err)

		event := bindingEvent(EventPreToolUse, "", filePath, cwdDir)

		binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

		require.NoError(t, err)
		assert.Equal(t, "from-file-path", binding.IssueID, "file_path should take precedence over event cwd")
		assert.Equal(t, filePathGitDir, binding.GitDir)
	})

	t.Run("Step3_SessionBindingFallback", func(t *testing.T) {
		t.Parallel()
		event := bindingEvent(EventPreToolUse, "", "", "")
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

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

		event := bindingEvent(EventPreToolUse, "Bash", filePath, cwdDir)
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

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

		event := bindingEvent(EventStop, "", filePath, cwdDir)
		sessionGitDir := "/session/git/dir"

		binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

		require.NoError(t, err)
		assert.Equal(t, "session-binding", binding.IssueID, "stop events should use session binding only")
		assert.Equal(t, sessionGitDir, binding.GitDir)
	})
}

func TestResolveBindingFromEvent_RelativeFilePath_JoinsWithEventCwd(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	worktreeDir := filepath.Join(tmpDir, "worktree")
	fileDir := filepath.Join(worktreeDir, "some", "path")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-issue-id"), []byte("task-relative"), 0o644))

	event := bindingEvent(EventPreToolUse, "", filepath.Join("some", "path", "file.go"), worktreeDir)

	binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "task-relative", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_RelativeFilePath_NoCwdFallsBackToSession(t *testing.T) {
	t.Parallel()
	event := bindingEvent(EventPreToolUse, "", "some/relative/file.go", "")
	sessionGitDir := "/session/git/dir"

	binding, err := ResolveBindingFromEvent(event, "session-binding", sessionGitDir, []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "session-binding", binding.IssueID)
	assert.Equal(t, sessionGitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_UnboundWorktree_ReturnsWorktreeGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	fileDir := filepath.Join(tmpDir, "some", "path")
	filePath := filepath.Join(fileDir, "file.go")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, os.MkdirAll(fileDir, 0o755))

	event := bindingEvent(EventPreToolUse, "", filePath, "")

	binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir, "should return the unbound worktree's git dir, not the session git dir")
}

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

	taskIDFile := filepath.Join(gitDir, "armature-task-id")
	err = os.WriteFile(taskIDFile, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "legacy-task-id", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
}

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

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("new-issue-id"), 0o644)
	require.NoError(t, err)

	taskIDFile := filepath.Join(gitDir, "armature-task-id")
	err = os.WriteFile(taskIDFile, []byte("legacy-task-id"), 0o644)
	require.NoError(t, err)

	binding, err := resolveBindingFromFilePath(filePath)

	require.NoError(t, err)
	assert.Equal(t, "new-issue-id", binding.IssueID, "armature-issue-id should take precedence")
	assert.Equal(t, gitDir, binding.GitDir)
}

func TestResolveBindingFromEvent_EventCwdAtWorktreeRoot_ResolvesBinding(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	worktreeRoot := tmpDir
	gitDir := filepath.Join(worktreeRoot, ".git")

	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	issueIDFile := filepath.Join(gitDir, "armature-issue-id")
	err = os.WriteFile(issueIDFile, []byte("issue-at-root"), 0o644)
	require.NoError(t, err)

	event := bindingEvent(EventPreToolUse, "Edit", "", worktreeRoot)

	binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "issue-at-root", binding.IssueID, "should find binding at event cwd worktree root")
	assert.Equal(t, gitDir, binding.GitDir)
	assert.Equal(t, "event_cwd", binding.ResolutionStep, "should resolve via event_cwd step")
}

func TestResolveBindingFromDir_MissingBindingIsUnbound_REQ_369(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))

	binding, err := ResolveBindingFromDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir)
	assert.Equal(t, tmpDir, binding.Root)
}

func TestResolveBindingFromDir_UnreadableBindingIsError_REQ_369(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: chmod 000 does not prevent root from reading files")
	}
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	path := filepath.Join(gitDir, "armature-issue-id")
	require.NoError(t, os.WriteFile(path, []byte("task-01\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(path, 0o644))
	})

	_, err := ResolveBindingFromDir(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

func TestResolveBindingFromDir_CorruptBindingIsError_REQ_369(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	path := filepath.Join(gitDir, "armature-issue-id")
	require.NoError(t, os.Mkdir(path, 0o755))

	_, err := ResolveBindingFromDir(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

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

func TestResolveBindingFromEvent_UnboundWorktreeViaCwdOnly_ReturnsWorktreeGitDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	event := bindingEvent(EventPreToolUse, "", "", tmpDir)

	binding, err := ResolveBindingFromEvent(event, "session-binding", "/session/git/dir", []string{"Bash"})

	require.NoError(t, err)
	assert.Equal(t, "", binding.IssueID)
	assert.Equal(t, gitDir, binding.GitDir, "should return the unbound worktree's git dir found via cwd, not the session git dir")
}

func TestFirstPathFromToolInput(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", FirstPathFromToolInput(nil), "nil input must return empty string")

	assert.Equal(t, "", FirstPathFromToolInput(map[string]any{}), "no matching key must return empty string")

	assert.Equal(t, "/a/file_path.go",
		FirstPathFromToolInput(map[string]any{"file_path": "/a/file_path.go"}))

	assert.Equal(t, "/a/path.go",
		FirstPathFromToolInput(map[string]any{"path": "/a/path.go"}))

	assert.Equal(t, "/a/file_path.go",
		FirstPathFromToolInput(map[string]any{"file_path": "/a/file_path.go", "path": "/a/path.go"}),
		"file_path must take precedence over path")

	assert.Equal(t, "",
		FirstPathFromToolInput(map[string]any{"file_path": ""}),
		"empty string value must not be treated as present")

	assert.Equal(t, "/a/changed.go",
		FirstPathFromToolInput(map[string]any{
			"changes": []any{map[string]any{"path": "/a/changed.go"}},
		}), "must fall back to the first changes[] entry's path")

	assert.Equal(t, "",
		FirstPathFromToolInput(map[string]any{"changes": []any{}}),
		"empty changes array must return empty string")

	assert.Equal(t, "",
		FirstPathFromToolInput(map[string]any{
			"changes": []any{map[string]any{"path": ""}},
		}), "empty path within a changes entry must return empty string")

	assert.Equal(t, "",
		FirstPathFromToolInput(map[string]any{
			"changes": []any{"not-a-map"},
		}), "a non-map changes entry must return empty string, not panic")
}
