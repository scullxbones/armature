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

func loadActivityEntries(t *testing.T, logPath string) map[int]review.ActivityEntryDetails {
	t.Helper()
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)
	activity := &review.Activity{
		Digest:     review.FingerprintActivity(content),
		EntryCount: strings.Count(string(content), "\n"),
		LogPath:    logPath,
	}
	entries, errs := review.ValidateActivityDigestAndLoadEntries(activity)
	require.Empty(t, errs)
	return entries
}

func TestActivityWriterParserRoundTrip_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "1234567890abcdef1234567890abcdef12345678"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	command := `grep "foo" main.go` + "\nsecond line with a \"quote\" and unicode: héllo wörld 日本語"

	head := strings.Repeat("H", 1200)
	middle := strings.Repeat("M", 4000)
	tail := strings.Repeat("T", 1200)
	output := []byte(head + middle + tail)

	require.NoError(t, harnesshook.AppendActivity(gitDir, command, 7, true, output))

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := loadActivityEntries(t, logPath)
	require.Len(t, entries, 1)

	details, ok := entries[0]
	require.True(t, ok, "entry should be recovered at physical line 0")

	assert.Equal(t, command, details.Command, "command must round-trip exactly, including quotes and newlines")
	assert.Equal(t, 7, details.ExitCode)
	assert.True(t, details.ExitCodeKnown)

	formatted := review.FormatActivityEntryDetails(details)
	assert.Contains(t, formatted, fmt.Sprintf("%q", command))
	assert.Contains(t, formatted, "exit_code=7")
}

func TestActivityWriterParserRoundTrip_UnknownExitCode_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "abcdef1234567890abcdef1234567890abcdef12"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	require.NoError(t, harnesshook.AppendActivity(gitDir, "go test ./...", 0, false, []byte("FAIL")))

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := loadActivityEntries(t, logPath)
	require.Len(t, entries, 1)

	details := entries[0]
	assert.False(t, details.ExitCodeKnown, "exit code must be recorded as unknown, not coerced to 0/success")
	assert.Contains(t, review.FormatActivityEntryDetails(details), "exit_code=unknown")
}

func TestActivityWriterParserRoundTrip_MultipleEntriesHeadSHA_REQ_EXECEV(t *testing.T) {
	t.Parallel()

	gitDir := t.TempDir()
	headSHA := "fedcba0987654321fedcba0987654321fedcba09"
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(headSHA+"\n"), 0o600))

	for i := 0; i < 3; i++ {
		require.NoError(t, harnesshook.AppendActivity(gitDir, fmt.Sprintf("command-%d", i), i, true, []byte("ok")))
	}

	logPath := filepath.Join(gitDir, "armature-activity.log")
	entries := loadActivityEntries(t, logPath)
	require.Len(t, entries, 3)

	for i := 0; i < 3; i++ {
		details, ok := entries[i]
		require.True(t, ok, "entry %d should be present", i)
		assert.Equal(t, fmt.Sprintf("command-%d", i), details.Command)
		assert.Equal(t, i, details.ExitCode)
	}
}
