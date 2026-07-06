package review_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/harnesshook"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActivityWriterParserRoundTrip_REQ_EXECEV verifies the full write→read pipeline
// between internal/harnesshook.AppendActivity (the writer) and
// internal/review.LoadActivityEntries (the parser). C2/C3/M1 stemmed from these two
// components (plus the skill docs) being authored against different imagined log
// formats with no cross-component test catching the mismatch; this test is that
// missing guardrail. It exercises: quoted content, embedded newlines, unicode,
// and >2KB output that must be truncated to head+tail while still round-tripping
// exactly for the parts that are kept.
func TestActivityWriterParserRoundTrip_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "1234567890abcdef1234567890abcdef12345678"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	command := `grep "foo" main.go` + "\nsecond line with a \"quote\" and unicode: héllo wörld 日本語"

	// Build output larger than 2KB (2*maxOutputChunkSize) so the writer truncates
	// it to head+tail form, and make head/tail distinguishable.
	head := strings.Repeat("H", 1200)
	middle := strings.Repeat("M", 4000)
	tail := strings.Repeat("T", 1200)
	output := []byte(head + middle + tail)

	require.NoError(t, harnesshook.AppendActivity(gitDir, command, 7, true, output))

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := review.LoadActivityEntries(logPath)
	require.Len(t, entries, 1)

	details, ok := entries[0]
	require.True(t, ok, "entry should be recovered at physical line 0")

	assert.Equal(t, command, details.Command, "command must round-trip exactly, including quotes and newlines")
	assert.Equal(t, 7, details.ExitCode)
	assert.True(t, details.ExitCodeKnown)

	// FormatActivityEntryDetails must reflect the recovered command/exit status
	// (rendered via %q, so compare against the quoted form).
	formatted := review.FormatActivityEntryDetails(details)
	assert.Contains(t, formatted, fmt.Sprintf("%q", command))
	assert.Contains(t, formatted, "exit_code=7")
}

// TestActivityWriterParserRoundTrip_UnknownExitCode_REQ_EXECEV verifies that an
// entry written with an unknown exit code round-trips as unknown, not as a
// silently-coerced exit_code=0 (M2).
func TestActivityWriterParserRoundTrip_UnknownExitCode_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "abcdef1234567890abcdef1234567890abcdef12"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	require.NoError(t, harnesshook.AppendActivity(gitDir, "go test ./...", 0, false, []byte("FAIL")))

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := review.LoadActivityEntries(logPath)
	require.Len(t, entries, 1)

	details := entries[0]
	assert.False(t, details.ExitCodeKnown, "exit code must be recorded as unknown, not coerced to 0/success")
	assert.Contains(t, review.FormatActivityEntryDetails(details), "exit_code=unknown")
}

// TestActivityWriterParserRoundTrip_MultipleEntriesHeadSHA_REQ_EXECEV verifies that
// head_sha round-trips correctly across multiple appended entries, matching the
// worktree HEAD at each append.
func TestActivityWriterParserRoundTrip_MultipleEntriesHeadSHA_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "fedcba0987654321fedcba0987654321fedcba09"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	for i := 0; i < 3; i++ {
		require.NoError(t, harnesshook.AppendActivity(gitDir, fmt.Sprintf("command-%d", i), i, true, []byte("ok")))
	}

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := review.LoadActivityEntries(logPath)
	require.Len(t, entries, 3)

	for i := 0; i < 3; i++ {
		details, ok := entries[i]
		require.True(t, ok, "entry %d should be present", i)
		assert.Equal(t, fmt.Sprintf("command-%d", i), details.Command)
		assert.Equal(t, i, details.ExitCode)
	}
}
