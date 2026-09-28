package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsUnsupportedNoteInReadme_REQ_CLAIMORD_W20(t *testing.T) {
	t.Parallel()
	body := readRepoDoc(t, "README.md")
	assert.Contains(t, body, "Windows is not supported")
	assert.Contains(t, strings.ToLower(body), "flock")
	assert.Contains(t, body, "POSIX")
	assert.Contains(t, strings.ToLower(body), "worker-id")
	assert.Contains(t, body, "LockFileEx")
	assert.Contains(t, body, "filelock_windows.go")
	assert.Contains(t, body, "unvalidated")
	assert.NotContains(t, body, "Windows would need `LockFileEx`")
	assert.Contains(t, body, "Validation rejects those")
	assert.NotContains(t, body, "does not reject those IDs")
}

func TestWindowsUnsupportedNoteInConcepts_REQ_CLAIMORD_W20(t *testing.T) {
	t.Parallel()
	body := readRepoDoc(t, filepath.Join("docs", "concepts.md"))
	assert.Contains(t, body, "Windows is not supported")
	assert.Contains(t, body, "README.md#windows-is-not-supported")
	assert.Contains(t, body, "once per worktree")
	assert.Contains(t, body, "git config --worktree")
	assert.NotContains(t, body, "clone-scoped")
}

func readRepoDoc(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		candidate := filepath.Join(dir, rel)
		if _, statErr := os.Stat(candidate); statErr == nil {
			b, readErr := os.ReadFile(candidate)
			require.NoError(t, readErr)
			return string(b)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo file %s not found from %s", rel, dir)
		}
		dir = parent
	}
}
