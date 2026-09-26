package harnesshook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityTruncateOutputShort_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	output := []byte("short output")
	result := truncateOutput(output)

	assert.Equal(t, "short output", result.Head)
	assert.Equal(t, "", result.Tail)
	assert.NotEmpty(t, result.Hash)
}

func TestActivityTruncateOutputExactBoundary_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	output := make([]byte, maxOutputChunkSize*2)
	for i := range output {
		output[i] = 'a'
	}

	result := truncateOutput(output)

	assert.Equal(t, string(output), result.Head)
	assert.Equal(t, "", result.Tail)
}

func TestActivityTruncateOutputLong_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	totalSize := maxOutputChunkSize * 3
	output := make([]byte, totalSize)

	for i := range maxOutputChunkSize {
		output[i] = 'H'
	}
	for i := maxOutputChunkSize; i < totalSize-maxOutputChunkSize; i++ {
		output[i] = 'M'
	}
	for i := totalSize - maxOutputChunkSize; i < totalSize; i++ {
		output[i] = 'T'
	}

	result := truncateOutput(output)

	assert.Len(t, result.Head, maxOutputChunkSize)
	assert.True(t, strings.Contains(result.Head, "H"))

	assert.Len(t, result.Tail, maxOutputChunkSize)
	assert.True(t, strings.Contains(result.Tail, "T"))

	expectedHash := fmt.Sprintf("%x", sha256.Sum256(output))
	assert.Equal(t, expectedHash, result.Hash)
}

func TestActivityGetWorktreeHEAD_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	headContent := "ref: refs/heads/main\n"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(headContent), 0o600))

	branchDir := filepath.Join(gitDir, "refs", "heads")
	require.NoError(t, os.MkdirAll(branchDir, 0o750))
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	branchPath := filepath.Join(branchDir, "main")
	require.NoError(t, os.WriteFile(branchPath, []byte(shaValue+"\n"), 0o600))

	sha, err := getWorktreeHEAD(gitDir)

	require.NoError(t, err)
	assert.Equal(t, shaValue, sha)
}

func TestActivityGetWorktreeHEADDetached_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	shaValue := "abcdef1234567890abcdef1234567890abcdef12"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	sha, err := getWorktreeHEAD(gitDir)

	require.NoError(t, err)
	assert.Equal(t, shaValue, sha)
}

func TestActivityAppendActivityCreatesLog_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err := AppendActivity(gitDir, command, exitCode, true, output)

	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, `"command"`)
	assert.Contains(t, logContent, `"exit_code":0`)
	assert.Contains(t, logContent, shaValue)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(content), &decoded), "activity log line must be valid JSON (JSONL format)")
}

func TestActivityAppendActivityMultipleEntries_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	for i := range 3 {
		command := fmt.Sprintf("command %d", i)
		exitCode := i
		output := []byte(fmt.Sprintf("output %d", i))

		err := AppendActivity(gitDir, command, exitCode, true, output)
		require.NoError(t, err)
	}

	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	assert.Len(t, nonEmptyLines, 3)
	for i := range 3 {
		assert.Contains(t, nonEmptyLines[i], fmt.Sprintf("command %d", i))
		assert.Contains(t, nonEmptyLines[i], fmt.Sprintf(`"exit_code":%d`, i))
	}
}

func TestActivityEnvVarKillSwitchHasNoEffect_REQ_EXECEV_T1(t *testing.T) {
	gitDir := t.TempDir()

	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "true")

	err := AppendActivity(gitDir, "echo hello", 0, true, []byte("hello\n"))
	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.NoError(t, err, "activity log should be created; ARMATURE_DISABLE_ACTIVITY_LOGGING must not disable capture")
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		fullArgs := append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)
		cmd := exec.CommandContext(context.Background(), "git", fullArgs...)
		// Isolate from the developer's global/system git config (e.g. a global
		// commit.gpgsign=true would hang the empty commit on a GPG pinentry).
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}
	runGit("init", "-b", "main")
	runGit("commit", "--allow-empty", "-m", "initial")
	return filepath.Join(dir, ".git")
}

func TestActivityRepoConfigKillSwitchDisablesLogging_REQ_EXECEV_T1(t *testing.T) {
	// Not parallel: pins the env-var kill-switch (unset) so a t.Setenv from
	// another test can't race this test's default-on assertion.
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")
	gitDir := initTestGitRepo(t)

	setGitConfigBool(t, gitDir, "true")

	err := AppendActivity(gitDir, "echo hello", 0, true, []byte("hello\n"))
	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.True(t, os.IsNotExist(err), "activity log should not exist when repo-level kill-switch is set")
}

func setGitConfigBool(t *testing.T, gitDir, value string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "--git-dir="+gitDir,
		"config", "--local", "--bool", "armature.disable-activity-logging", value)
	require.NoError(t, cmd.Run())
}

func TestActivityRepoConfigEnabledByDefault_REQ_EXECEV_T1(t *testing.T) {
	// Not parallel: pins the env-var kill-switch (unset) so a t.Setenv from
	// another test can't race this test's default-on assertion.
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")
	gitDir := initTestGitRepo(t)

	err := AppendActivity(gitDir, "echo hello", 0, true, []byte("hello\n"))
	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.NoError(t, err, "activity log should exist when repo config kill-switch is unset")
}

func TestActivityRepoConfigKillSwitchFalseLeavesEnabled_REQ_EXECEV_T1(t *testing.T) {
	// Not parallel: pins the env-var kill-switch (unset) so a t.Setenv from
	// another test can't race this test's default-on assertion.
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")
	gitDir := initTestGitRepo(t)

	setGitConfigBool(t, gitDir, "false")

	err := AppendActivity(gitDir, "echo hello", 0, true, []byte("hello\n"))
	require.NoError(t, err)

	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.NoError(t, err, "activity log should exist when repo config kill-switch is explicitly false")
}

func TestActivityFailOpenOnHEADError_REQ_EXECEV_T1(t *testing.T) {
	// Not parallel: this test redirects the global os.Stderr and must not observe
	// the kill-switch env var set by other tests, so pin it via t.Setenv (which
	// also forces serial execution).
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")
	gitDir := t.TempDir()

	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err = AppendActivity(gitDir, command, exitCode, true, output)

	assert.NoError(t, err)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	stderrOutput := buf.String()
	assert.Contains(t, stderrOutput, "warning")
}

func TestActivityFailOpenOnLogWriteError_REQ_EXECEV_T1(t *testing.T) {
	// Not parallel: redirects global os.Stderr and depends on the kill-switch
	// env var being unset; t.Setenv pins it and forces serial execution.
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")
	gitDir := t.TempDir()

	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	require.NoError(t, os.Chmod(gitDir, 0o500))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(gitDir, 0o755))
	})

	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err = AppendActivity(gitDir, command, exitCode, true, output)

	assert.NoError(t, err)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	stderrOutput := buf.String()
	assert.Contains(t, stderrOutput, "warning")
}

func TestActivityFormatLogEntry_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	entry := ActivityEntry{
		Command:       "test command",
		ExitCode:      0,
		ExitCodeKnown: true,
		OutputHead:    "test output",
		OutputTail:    "",
		OutputHash:    "abc123",
		WorktreeHead:  "def456",
		Timestamp:     "2026-07-05T12:00:00Z",
	}

	logLine := formatActivityLogEntry(entry)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(logLine), &decoded))

	assert.Contains(t, logLine, "2026-07-05T12:00:00Z")
	assert.Contains(t, logLine, `"command"`)
	assert.Contains(t, logLine, `"exit_code":0`)
	assert.Contains(t, logLine, `"exit_code_known":true`)
	assert.Contains(t, logLine, `"head_sha":"def456"`)
	assert.Contains(t, logLine, `"output_hash":"abc123"`)
}

func TestActivityFormatLogEntryUnknownExitCode_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	entry := ActivityEntry{
		Command:       "test command",
		ExitCode:      0,
		ExitCodeKnown: false,
		WorktreeHead:  "def456",
		OutputHash:    "abc123",
		Timestamp:     "2026-07-05T12:00:00Z",
	}

	logLine := formatActivityLogEntry(entry)

	assert.Contains(t, logLine, `"exit_code_known":false`)
}

func TestActivityTruncateOutputRuneBoundary_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	filler := strings.Repeat("a", maxOutputChunkSize-1)
	middle := strings.Repeat("b", maxOutputChunkSize)
	tailFiller := strings.Repeat("c", maxOutputChunkSize-1)
	output := []byte(filler + "é" + middle + "é" + tailFiller)

	result := truncateOutput(output)

	assert.True(t, utf8.ValidString(result.Head), "head must not split a UTF-8 rune")
	assert.True(t, utf8.ValidString(result.Tail), "tail must not split a UTF-8 rune")
}

func TestActivityLogEntryWithTimestamp_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	beforeTime := time.Now().UTC()
	command := "test"
	exitCode := 0
	output := []byte("test output")

	err := AppendActivity(gitDir, command, exitCode, true, output)
	require.NoError(t, err)

	afterTime := time.Now().UTC()

	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var decoded struct {
		Timestamp string `json:"timestamp"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(content), &decoded))

	logTime, err := time.Parse(time.RFC3339, decoded.Timestamp)
	require.NoError(t, err)

	assert.True(t, logTime.After(beforeTime.Add(-time.Second)))
	assert.True(t, logTime.Before(afterTime.Add(time.Second)))
}

func TestActivityHashConsistency_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	output := []byte("test output for hashing")
	result1 := truncateOutput(output)
	result2 := truncateOutput(output)

	assert.Equal(t, result1.Hash, result2.Hash)

	expectedHash := fmt.Sprintf("%x", sha256.Sum256(output))
	assert.Equal(t, expectedHash, result1.Hash)
}

func TestActivityFallbackGetHEAD_UsesGitDirDirectly_REQ_EXECEV_M7(t *testing.T) {
	t.Parallel()
	gitDir := initTestGitRepo(t)

	revParseCmd := exec.CommandContext(context.Background(), "git", "--git-dir="+gitDir, "rev-parse", "HEAD")
	expectedOut, err := revParseCmd.Output()
	require.NoError(t, err)
	expected := strings.TrimSpace(string(expectedOut))

	packCmd := exec.CommandContext(context.Background(), "git", "--git-dir="+gitDir, "pack-refs", "--all")
	require.NoError(t, packCmd.Run())
	_, statErr := os.Stat(filepath.Join(gitDir, "refs", "heads", "main"))
	require.True(t, os.IsNotExist(statErr), "loose ref file must be gone after pack-refs for this test to exercise the fallback")

	sha, err := getWorktreeHEAD(gitDir)
	require.NoError(t, err)
	assert.Equal(t, expected, sha)
}

func TestActivityFallbackGetHEAD_InvalidGitDir_REQ_EXECEV_M7(t *testing.T) {
	t.Parallel()
	_, err := fallbackGetHEAD(t.TempDir())
	assert.Error(t, err)
}

func TestActivityAppendActivityCapsOversizedCommand_REQ_EXECEV_M5(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(shaValue+"\n"), 0o600))

	hugeCommand := strings.Repeat("x", maxCommandSize*3)
	require.NoError(t, AppendActivity(gitDir, hugeCommand, 0, true, []byte("ok")))

	content, err := os.ReadFile(filepath.Join(gitDir, "armature-activity.log"))
	require.NoError(t, err)

	var decoded struct {
		Command string `json:"command"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(content), &decoded))
	assert.LessOrEqual(t, len(decoded.Command), maxCommandSize, "command must be capped at write time")
	assert.Less(t, len(decoded.Command), len(hugeCommand), "command must actually have been truncated")
}

func TestAppendActivity_EmptyOrMissingCommandWritesNothing_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	newGitDir := func(t *testing.T) string {
		t.Helper()
		gitDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("1234567890abcdef1234567890abcdef12345678\n"), 0o600))
		return gitDir
	}

	assertNoLog := func(t *testing.T, gitDir string) {
		t.Helper()
		_, err := os.Stat(filepath.Join(gitDir, "armature-activity.log"))
		assert.True(t, os.IsNotExist(err), "empty or missing command must not create an activity log")
	}

	t.Run("empty command string", func(t *testing.T) {
		t.Parallel()
		gitDir := newGitDir(t)
		require.NoError(t, AppendActivity(gitDir, "", 0, true, []byte("unused")))
		assertNoLog(t, gitDir)
	})

	t.Run("event with empty command", func(t *testing.T) {
		t.Parallel()
		gitDir := newGitDir(t)
		adapter := NewCodexAdapter()
		evt, err := adapter.Decode([]byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":""},"tool_response":{"exit_code":0}}`))
		require.NoError(t, err)
		require.Equal(t, "", evt.Command)
		require.NoError(t, AppendActivity(gitDir, evt.Command, evt.ExitCode, evt.ExitCodeKnown, evt.Output))
		assertNoLog(t, gitDir)
	})

	t.Run("event with no tool_input", func(t *testing.T) {
		t.Parallel()
		gitDir := newGitDir(t)
		adapter := NewCodexAdapter()
		evt, err := adapter.Decode([]byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_response":{"exit_code":0}}`))
		require.NoError(t, err)
		require.Equal(t, "", evt.Command)
		require.NoError(t, AppendActivity(gitDir, evt.Command, evt.ExitCode, evt.ExitCodeKnown, evt.Output))
		assertNoLog(t, gitDir)
	})
}

func TestFallbackActivityJSONL_CannotEmitEmptyCommand_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	assert.Empty(t, fallbackActivityJSONL(ActivityEntry{
		Timestamp:    "2026-09-24T00:00:00Z",
		WorktreeHead: "abc",
	}))
	assert.Empty(t, fallbackActivityJSONL(ActivityEntry{
		Command:      "   ",
		Timestamp:    "2026-09-24T00:00:00Z",
		WorktreeHead: "abc",
	}))

	line := fallbackActivityJSONL(ActivityEntry{
		Command:       `grep "foo"`,
		ExitCode:      0,
		ExitCodeKnown: true,
		Timestamp:     "2026-09-24T00:00:00Z",
		WorktreeHead:  "abc",
		OutputHash:    "def",
	})
	require.NotEmpty(t, line)
	var decoded activityLogLine
	require.NoError(t, json.Unmarshal([]byte(line), &decoded))
	assert.Equal(t, `grep "foo"`, decoded.Command)
	assert.Equal(t, 0, decoded.ExitCode)
	assert.True(t, decoded.ExitCodeKnown)
}
