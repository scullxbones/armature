package e2eharness_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/e2eharness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHarnessNew_REQ_TOPTIER_S3_T1 verifies that New() creates a harness with bare
// origin repo and work directory cloned from it.
func TestHarnessNew_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	armBin := "arm"
	h := e2eharness.New(t, armBin)

	// Verify origin repo exists and is bare
	assert.DirExists(t, h.OriginDir)
	gitConfigPath := filepath.Join(h.OriginDir, "config")
	assert.FileExists(t, gitConfigPath)

	// Verify work directory exists
	assert.DirExists(t, h.WorkDir)
	gitDir := filepath.Join(h.WorkDir, ".git")
	assert.DirExists(t, gitDir)

	// Verify temp dir is set
	assert.NotEmpty(t, h.TempDir)
	assert.DirExists(t, h.TempDir)
}

// TestHarnessClone_REQ_TOPTIER_S3_T1 verifies that Clone() creates a new clone
// at the specified path with git configured.
func TestHarnessClone_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := e2eharness.New(t, "arm")
	workerPath := filepath.Join(h.TempDir, "worker-1")

	err := h.Clone("worker-1", workerPath)
	require.NoError(t, err)

	// Verify clone exists
	assert.DirExists(t, workerPath)
	gitDir := filepath.Join(workerPath, ".git")
	assert.DirExists(t, gitDir)

	// Verify it's tracked in WorkerDirs
	assert.Equal(t, workerPath, h.GetWorkerDir("worker-1"))
}

// TestHarnessGetWorkerDir_ReturnsEmpty_REQ_TOPTIER_S3_T1 verifies that
// GetWorkerDir() returns empty string for non-existent workers.
func TestHarnessGetWorkerDir_ReturnsEmpty_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := e2eharness.New(t, "arm")

	result := h.GetWorkerDir("nonexistent-worker")
	assert.Equal(t, "", result)
}

// TestHarnessMultipleClones_REQ_TOPTIER_S3_T1 verifies that multiple worker
// clones can be created and tracked independently.
func TestHarnessMultipleClones_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := e2eharness.New(t, "arm")

	// Create multiple clones
	worker1Path := filepath.Join(h.TempDir, "worker-1")
	worker2Path := filepath.Join(h.TempDir, "worker-2")

	require.NoError(t, h.Clone("worker-1", worker1Path))
	require.NoError(t, h.Clone("worker-2", worker2Path))

	// Verify both are tracked
	assert.Equal(t, worker1Path, h.GetWorkerDir("worker-1"))
	assert.Equal(t, worker2Path, h.GetWorkerDir("worker-2"))

	// Verify both exist on disk
	assert.DirExists(t, worker1Path)
	assert.DirExists(t, worker2Path)
}

// TestHarnessArmBinPathSet_REQ_TOPTIER_S3_T1 verifies that the harness
// correctly stores the arm binary path.
func TestHarnessArmBinPathSet_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	armBin := "/path/to/arm"
	h := e2eharness.New(t, armBin)

	assert.Equal(t, armBin, h.ArmBinPath)
}

// TestHarnessOriginIsAccessible_REQ_TOPTIER_S3_T1 verifies that the work
// directory is properly linked to the origin (can pull from it).
func TestHarnessOriginIsAccessible_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := e2eharness.New(t, "arm")

	// Verify the work directory has origin as a remote
	remoteFile := filepath.Join(h.WorkDir, ".git", "config")
	require.FileExists(t, remoteFile)

	configContent, err := os.ReadFile(remoteFile)
	require.NoError(t, err)

	// Should contain reference to origin
	assert.Contains(t, string(configContent), "origin")
}
