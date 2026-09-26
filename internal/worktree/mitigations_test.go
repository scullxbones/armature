package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyMitigations_NoMainGoWork_NoOp_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(worktreeRoot, 0o755))

	require.NoError(t, ApplyMitigations(repoRoot, worktreeRoot))

	_, err := os.Stat(filepath.Join(repoRoot, "go.work"))
	assert.True(t, os.IsNotExist(err), "no go.work should be created in the main tree")
	_, err = os.Stat(filepath.Join(worktreeRoot, "go.work"))
	assert.True(t, os.IsNotExist(err), "no go.work should be created in the worktree")
}

func TestApplyMitigations_RemovesWorktreeFromUseBlock_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task-01")
	goWork := "go 1.26\n\nuse (\n\t.\n\t./.worktrees/task-01\n)\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.work"), []byte(goWork), 0o644))

	require.NoError(t, ApplyMitigations(repoRoot, worktreeRoot))

	content, err := os.ReadFile(filepath.Join(repoRoot, "go.work"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "task-01")
	assert.Contains(t, string(content), "\t.\n", "unrelated use entries must be preserved")
}

func TestApplyMitigations_RemovesWorktreeFromSingleLineUse_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task-02")
	goWork := "go 1.26\n\nuse .\nuse ./.worktrees/task-02\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.work"), []byte(goWork), 0o644))

	require.NoError(t, ApplyMitigations(repoRoot, worktreeRoot))

	content, err := os.ReadFile(filepath.Join(repoRoot, "go.work"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "task-02")
	assert.Contains(t, string(content), "use .\n", "unrelated use entries must be preserved")
}

func TestApplyMitigations_GoWorkWithoutWorktree_Unchanged_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task-03")
	goWork := "go 1.26\n\nuse (\n\t.\n\t./other\n)\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.work"), []byte(goWork), 0o644))

	require.NoError(t, ApplyMitigations(repoRoot, worktreeRoot))

	content, err := os.ReadFile(filepath.Join(repoRoot, "go.work"))
	require.NoError(t, err)
	assert.Equal(t, goWork, string(content))
}

func TestApplyMitigations_ParsesQuotedPathsAndTrailingComments_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task with space")
	content := "go 1.26\n\nuse (\n\t\"./.worktrees/task with space\" // managed\n\t./other // unrelated\n)\nuse \"./.worktrees/task with space\" // duplicate\n"
	newContent, changed := removeWorktreeFromGoWork(content, repoRoot, worktreeRoot)
	assert.True(t, changed)
	assert.NotContains(t, newContent, "task with space")
	assert.Contains(t, newContent, "./other // unrelated")
}

func TestApplyMitigations_PreservesCRLFWhenRemovingQuotedPath_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "task-crlf")
	content := "go 1.26\r\n\r\nuse (\r\n\t\"./.worktrees/task-crlf\" // managed\r\n\t./other\r\n)\r\n"
	newContent, changed := removeWorktreeFromGoWork(content, repoRoot, worktreeRoot)
	assert.True(t, changed)
	assert.Equal(t, "go 1.26\r\n\r\nuse (\r\n\t./other\r\n)\r\n", newContent)
}

func TestNormalizePath_NonexistentFallsBackToAbs_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	got := NormalizePath("relative/does-not-exist")
	assert.True(t, filepath.IsAbs(got), "expected an absolute path, got %q", got)
}
