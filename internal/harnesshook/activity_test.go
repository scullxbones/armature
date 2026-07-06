package harnesshook

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, "", result.Marker)
}

func TestActivityTruncateOutputExactBoundary_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	// Create output exactly 2*maxOutputChunkSize bytes
	output := make([]byte, maxOutputChunkSize*2)
	for i := range output {
		output[i] = 'a'
	}

	result := truncateOutput(output)

	// At exactly 2*maxOutputChunkSize, it should not be truncated
	assert.Equal(t, string(output), result.Head)
	assert.Equal(t, "", result.Tail)
	assert.Equal(t, "", result.Marker)
}

func TestActivityTruncateOutputLong_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	// Create output larger than 2*maxOutputChunkSize
	totalSize := maxOutputChunkSize * 3
	output := make([]byte, totalSize)

	// Fill with different patterns to distinguish head and tail
	for i := 0; i < maxOutputChunkSize; i++ {
		output[i] = 'H' // HEAD
	}
	for i := maxOutputChunkSize; i < totalSize-maxOutputChunkSize; i++ {
		output[i] = 'M' // MIDDLE
	}
	for i := totalSize - maxOutputChunkSize; i < totalSize; i++ {
		output[i] = 'T' // TAIL
	}

	result := truncateOutput(output)

	// Check head
	assert.Len(t, result.Head, maxOutputChunkSize)
	assert.True(t, strings.Contains(result.Head, "H"))

	// Check tail
	assert.Len(t, result.Tail, maxOutputChunkSize)
	assert.True(t, strings.Contains(result.Tail, "T"))

	// Check marker is set
	assert.NotEmpty(t, result.Marker)

	// Check hash is correct for full output
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(output))
	assert.Equal(t, expectedHash, result.Hash)
}

func TestActivityGetWorktreeHEAD_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	// Create a HEAD file pointing to a branch
	headContent := "ref: refs/heads/main\n"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(headContent), 0o600))

	// Create the branch ref file
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

	// Create a HEAD file in detached state (direct SHA)
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

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Append an activity
	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err := AppendActivity(gitDir, command, exitCode, output)

	require.NoError(t, err)

	// Verify log file was created and contains the entry
	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logContent := string(content)
	assert.Contains(t, logContent, "activity:")
	assert.Contains(t, logContent, "command=")
	assert.Contains(t, logContent, "exit_code=0")
	assert.Contains(t, logContent, shaValue)
}

func TestActivityAppendActivityMultipleEntries_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Append multiple activities
	for i := 0; i < 3; i++ {
		command := fmt.Sprintf("command %d", i)
		exitCode := i
		output := []byte(fmt.Sprintf("output %d", i))

		err := AppendActivity(gitDir, command, exitCode, output)
		require.NoError(t, err)
	}

	// Verify log file contains all entries
	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	// Filter out empty lines
	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	assert.Len(t, nonEmptyLines, 3)
	for i := 0; i < 3; i++ {
		assert.Contains(t, nonEmptyLines[i], fmt.Sprintf("command %d", i))
		assert.Contains(t, nonEmptyLines[i], fmt.Sprintf("exit_code=%d", i))
	}
}

func TestActivityKillSwitchDisablesLogging_REQ_EXECEV_T1(t *testing.T) {
	gitDir := t.TempDir()

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Set kill-switch environment variable
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "true")

	// Try to append an activity
	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err := AppendActivity(gitDir, command, exitCode, output)

	// Should not error
	require.NoError(t, err)

	// Verify log file was not created
	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.True(t, os.IsNotExist(err), "activity log should not exist when disabled")
}

func TestActivityKillSwitchDisabledByDefault_REQ_EXECEV_T1(t *testing.T) {
	gitDir := t.TempDir()

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Unset kill-switch
	t.Setenv("ARMATURE_DISABLE_ACTIVITY_LOGGING", "")

	// Try to append an activity
	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err := AppendActivity(gitDir, command, exitCode, output)

	// Should not error
	require.NoError(t, err)

	// Verify log file was created (logging is enabled by default)
	logPath := filepath.Join(gitDir, "armature-activity.log")
	_, err = os.ReadFile(logPath)
	assert.NoError(t, err, "activity log should exist when not disabled")
}

func TestActivityFailOpenOnHEADError_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	// Don't create HEAD file, so getWorktreeHEAD fails
	// Capture stderr to verify warning is printed
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err = AppendActivity(gitDir, command, exitCode, output)

	// Fail-open: should not return an error
	assert.NoError(t, err)

	_ = w.Close() //nolint:errcheck // test code
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r) //nolint:errcheck // test code

	// Should have printed a warning to stderr
	stderrOutput := buf.String()
	assert.Contains(t, stderrOutput, "warning")
}

func TestActivityFailOpenOnLogWriteError_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Make the git dir read-only so we can't write to it
	require.NoError(t, os.Chmod(gitDir, 0o500))
	t.Cleanup(func() {
		_ = os.Chmod(gitDir, 0o755) //nolint:errcheck // cleanup code
	})

	// Capture stderr to verify warning is printed
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	command := "echo hello"
	exitCode := 0
	output := []byte("hello\n")

	err = AppendActivity(gitDir, command, exitCode, output)

	// Fail-open: should not return an error
	assert.NoError(t, err)

	_ = w.Close() //nolint:errcheck // test code
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r) //nolint:errcheck // test code

	// Should have printed a warning to stderr
	stderrOutput := buf.String()
	assert.Contains(t, stderrOutput, "warning")
}

func TestActivityFormatLogEntry_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	entry := ActivityEntry{
		Command:      "test command",
		ExitCode:     0,
		OutputHead:   "test output",
		OutputTail:   "",
		OutputHash:   "abc123",
		WorktreeHead: "def456",
		Timestamp:    "2026-07-05T12:00:00Z",
	}

	logLine := formatActivityLogEntry(entry)

	assert.Contains(t, logLine, "2026-07-05T12:00:00Z")
	assert.Contains(t, logLine, "activity:")
	assert.Contains(t, logLine, "command=")
	assert.Contains(t, logLine, "exit_code=0")
	assert.Contains(t, logLine, "head_sha=def456")
	assert.Contains(t, logLine, "output_hash=abc123")
}

func TestActivityLogEntryWithTimestamp_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	gitDir := t.TempDir()

	// Create HEAD file
	shaValue := "1234567890abcdef1234567890abcdef12345678"
	headPath := filepath.Join(gitDir, "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte(shaValue+"\n"), 0o600))

	// Append an activity
	beforeTime := time.Now().UTC()
	command := "test"
	exitCode := 0
	output := []byte("test output")

	err := AppendActivity(gitDir, command, exitCode, output)
	require.NoError(t, err)

	afterTime := time.Now().UTC()

	// Verify log file contains timestamp
	logPath := filepath.Join(gitDir, "armature-activity.log")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logContent := string(content)
	// Parse timestamp from log line (format: YYYY-MM-DDTHH:MM:SSZ)
	parts := strings.Split(logContent, " ")
	assert.Greater(t, len(parts), 0)

	// Try to parse the timestamp
	logTime, err := time.Parse(time.RFC3339, parts[0])
	require.NoError(t, err)

	// Verify it's within the expected range
	assert.True(t, logTime.After(beforeTime.Add(-time.Second)))
	assert.True(t, logTime.Before(afterTime.Add(time.Second)))
}

func TestActivityHashConsistency_REQ_EXECEV_T1(t *testing.T) {
	t.Parallel()
	output := []byte("test output for hashing")
	result1 := truncateOutput(output)
	result2 := truncateOutput(output)

	// Hash should be consistent
	assert.Equal(t, result1.Hash, result2.Hash)

	// Hash should be the correct SHA256
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(output))
	assert.Equal(t, expectedHash, result1.Hash)
}
