package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectProjectType_GoModPresent_REQ_LNGHZN_S5_T3 verifies that a Go project
// is correctly detected when go.mod exists in the repo root.
func TestDetectProjectType_GoModPresent_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a go.mod file
	goModPath := filepath.Join(dir, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	projType := DetectProjectType(dir)
	assert.Equal(t, ProjectTypeGo, projType)
}

// TestDetectProjectType_GoWorkPresent_REQ_LNGHZN_S5_T3 verifies that a Go project
// is correctly detected when go.work exists in the repo root.
func TestDetectProjectType_GoWorkPresent_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a go.work file
	goWorkPath := filepath.Join(dir, "go.work")
	err := os.WriteFile(goWorkPath, []byte("go 1.26\n"), 0o644)
	require.NoError(t, err)

	projType := DetectProjectType(dir)
	assert.Equal(t, ProjectTypeGo, projType)
}

// TestDetectProjectType_BothGoModAndGoWorkPresent_REQ_LNGHZN_S5_T3 verifies that
// a Go project is correctly detected when both go.mod and go.work exist.
func TestDetectProjectType_BothGoModAndGoWorkPresent_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create both go.mod and go.work
	goModPath := filepath.Join(dir, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	goWorkPath := filepath.Join(dir, "go.work")
	err = os.WriteFile(goWorkPath, []byte("go 1.26\n"), 0o644)
	require.NoError(t, err)

	projType := DetectProjectType(dir)
	assert.Equal(t, ProjectTypeGo, projType)
}

// TestDetectProjectType_NoGoFiles_REQ_LNGHZN_S5_T3 verifies that when neither
// go.mod nor go.work exists, the project type is Unknown.
func TestDetectProjectType_NoGoFiles_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	projType := DetectProjectType(dir)
	assert.Equal(t, ProjectTypeUnknown, projType)
}

// TestDetectProjectType_NonexistentPath_REQ_LNGHZN_S5_T3 verifies that detection
// returns Unknown for a non-existent path (graceful degradation).
func TestDetectProjectType_NonexistentPath_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	projType := DetectProjectType("/nonexistent/path")
	assert.Equal(t, ProjectTypeUnknown, projType)
}

// TestApplyMitigations_GoProjectCreatesWorktreeGoWork_REQ_LNGHZN_S5_T3 verifies that
// for a Go project, applying mitigations creates a go.work file in the worktree.
func TestApplyMitigations_GoProjectCreatesWorktreeGoWork_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// Create go.mod in repo root to simulate a Go project
	goModPath := filepath.Join(repoRoot, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	// Apply mitigations for the worktree
	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify go.work was created in the worktree
	worktreeGoWork := filepath.Join(worktreeRoot, "go.work")
	content, err := os.ReadFile(worktreeGoWork)
	require.NoError(t, err)
	assert.Contains(t, string(content), "go")
}

// TestApplyMitigations_NonGoProject_NoFiles_REQ_LNGHZN_S5_T3 verifies that
// applying mitigations for a non-Go project creates no files.
func TestApplyMitigations_NonGoProject_NoFiles_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// Apply mitigations with no Go files present
	err := ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify no go.work was created
	worktreeGoWork := filepath.Join(worktreeRoot, "go.work")
	_, err = os.Stat(worktreeGoWork)
	assert.True(t, os.IsNotExist(err), "go.work should not exist for non-Go project")
}

// TestApplyMitigations_GoProjectIdempotent_REQ_LNGHZN_S5_T3 verifies that
// applying mitigations multiple times is idempotent (no errors, consistent results).
func TestApplyMitigations_GoProjectIdempotent_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// Create go.mod in repo root
	goModPath := filepath.Join(repoRoot, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	// Apply mitigations twice
	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	firstContent, err := os.ReadFile(filepath.Join(worktreeRoot, "go.work"))
	require.NoError(t, err)

	// Clean up and re-apply
	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	secondContent, err := os.ReadFile(filepath.Join(worktreeRoot, "go.work"))
	require.NoError(t, err)

	// Content should be consistent
	assert.Equal(t, firstContent, secondContent)
}

// TestApplyMitigations_PreservesExistingGoWork_REQ_LNGHZN_S5_T3 verifies that
// if a go.work already exists in the worktree, it is not overwritten.
func TestApplyMitigations_PreservesExistingGoWork_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := t.TempDir()

	// Create go.mod in repo root
	goModPath := filepath.Join(repoRoot, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	// Pre-create go.work in worktree with custom content
	existingGoWork := filepath.Join(worktreeRoot, "go.work")
	customContent := []byte("go 1.26\nuse ./custom\n")
	err = os.WriteFile(existingGoWork, customContent, 0o644)
	require.NoError(t, err)

	// Apply mitigations
	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify existing go.work was preserved
	content, err := os.ReadFile(existingGoWork)
	require.NoError(t, err)
	assert.Equal(t, customContent, content)
}

// TestApplyMitigations_WorktreePathNonexistent_REQ_LNGHZN_S5_T3 verifies that
// applying mitigations gracefully handles a non-existent worktree path by creating it.
func TestApplyMitigations_WorktreePathNonexistent_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(t.TempDir(), "subdir", "worktree")

	// Create go.mod in repo root
	goModPath := filepath.Join(repoRoot, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	// Apply mitigations (worktreeRoot doesn't exist yet)
	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify go.work was created (and parent directories were created)
	worktreeGoWork := filepath.Join(worktreeRoot, "go.work")
	_, err = os.Stat(worktreeGoWork)
	require.NoError(t, err, "go.work should exist after mitigation")
}

// TestGetMitigationsForProjectType_Go_REQ_LNGHZN_S5_T3 verifies that
// GetMitigationsForProjectType returns appropriate mitigations for Go projects.
func TestGetMitigationsForProjectType_Go_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	mitigations := GetMitigationsForProjectType(ProjectTypeGo)
	assert.NotEmpty(t, mitigations)
	assert.Contains(t, mitigations, MitigationGoWorkIsolation)
}

// TestGetMitigationsForProjectType_Unknown_REQ_LNGHZN_S5_T3 verifies that
// GetMitigationsForProjectType returns no mitigations for unknown project types.
func TestGetMitigationsForProjectType_Unknown_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	mitigations := GetMitigationsForProjectType(ProjectTypeUnknown)
	assert.Empty(t, mitigations)
}

// TestProvisionWorktreeWithMitigations_REQ_LNGHZN_S5_T3 is an integration test
// that verifies the full provisioning flow with mitigations.
func TestProvisionWorktreeWithMitigations_REQ_LNGHZN_S5_T3(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	worktreeRoot := filepath.Join(repoRoot, ".worktrees", "test-task")

	// Create go.mod to simulate a Go project
	goModPath := filepath.Join(repoRoot, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/repo\n"), 0o644)
	require.NoError(t, err)

	// Create .worktrees directory
	err = os.MkdirAll(filepath.Dir(worktreeRoot), 0o755)
	require.NoError(t, err)

	// Detect project type and apply mitigations
	projType := DetectProjectType(repoRoot)
	assert.Equal(t, ProjectTypeGo, projType)

	err = ApplyMitigations(repoRoot, worktreeRoot)
	require.NoError(t, err)

	// Verify mitigations were applied
	worktreeGoWork := filepath.Join(worktreeRoot, "go.work")
	_, err = os.Stat(worktreeGoWork)
	require.NoError(t, err, "go.work should be created in worktree")
}
