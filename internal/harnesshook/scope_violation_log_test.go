package harnesshook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeViolationLogging_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	result, err := LogPassThroughScopeViolation(gitDir, []string{"internal/"}, tmpDir, []string{"cmd/main.go"}, "stale binding")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "expected the out-of-scope path to be flagged")
	require.NotEmpty(t, result.Violations)
	assert.Equal(t, "cmd/main.go", result.Violations[0].Path)

	logPath := filepath.Join(gitDir, "armature-hook.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err, "expected armature-hook.log to be written")

	logContent := string(data)
	assert.Contains(t, logContent, "violation:", "expected a violation: marker even though the hook passed through")
	assert.Contains(t, logContent, "cmd/main.go", "expected the out-of-scope path to be recorded")
	assert.Contains(t, logContent, "stale binding", "expected the pass-through reason to be recorded")

	tmpDir2 := t.TempDir()
	gitDir2 := filepath.Join(tmpDir2, ".git")
	require.NoError(t, os.MkdirAll(gitDir2, 0o755))
	result2, err := LogPassThroughScopeViolation(gitDir2, []string{"internal/"}, tmpDir2, []string{"internal/foo.go"}, "stale binding")
	require.NoError(t, err)
	assert.True(t, result2.Allowed)
	_, statErr := os.Stat(filepath.Join(gitDir2, "armature-hook.log"))
	assert.True(t, os.IsNotExist(statErr), "expected no log file to be written when the path is in scope")
}

func TestLogDecisionAndPassThrough_StripNewlines(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	require.NoError(t, LogPassThrough(gitDir, "stale\nissue binding"))
	require.NoError(t, LogViolation(gitDir, "file write\rwith no resolved binding"))
	require.NoError(t, LogDecision(gitDir, "TASK-1\n", "session", "pre-tool-use", "Edit", "block", "out of\nscope"))

	data, err := os.ReadFile(filepath.Join(gitDir, "armature-hook.log"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "pass-through: stale issue binding")
	assert.NotContains(t, lines[0], "\nissue")
	assert.Contains(t, lines[1], "violation: file write with no resolved binding")
	assert.Contains(t, lines[2], "decision: issue_id=TASK-1  resolution_step=session")
	assert.Contains(t, lines[2], "block_reason=out of scope")
}
