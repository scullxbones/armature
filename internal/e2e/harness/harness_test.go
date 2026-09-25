package harness_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarnessNew_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	armBin := "arm"
	h := harness.New(t, armBin)

	assert.DirExists(t, h.OriginDir)
	gitConfigPath := filepath.Join(h.OriginDir, "config")
	assert.FileExists(t, gitConfigPath)

	assert.DirExists(t, h.WorkDir)
	gitDir := filepath.Join(h.WorkDir, ".git")
	assert.DirExists(t, gitDir)

	assert.NotEmpty(t, h.TempDir)
	assert.DirExists(t, h.TempDir)
}

func TestHarnessClone_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := harness.New(t, "arm")
	workerPath := filepath.Join(h.TempDir, "worker-1")

	err := h.Clone("worker-1", workerPath)
	require.NoError(t, err)

	assert.DirExists(t, workerPath)
	gitDir := filepath.Join(workerPath, ".git")
	assert.DirExists(t, gitDir)

	assert.Equal(t, workerPath, h.GetWorkerDir("worker-1"))
}

func TestHarnessGetWorkerDir_ReturnsEmpty_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := harness.New(t, "arm")

	result := h.GetWorkerDir("nonexistent-worker")
	assert.Equal(t, "", result)
}

func TestHarnessMultipleClones_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := harness.New(t, "arm")

	worker1Path := filepath.Join(h.TempDir, "worker-1")
	worker2Path := filepath.Join(h.TempDir, "worker-2")

	require.NoError(t, h.Clone("worker-1", worker1Path))
	require.NoError(t, h.Clone("worker-2", worker2Path))

	assert.Equal(t, worker1Path, h.GetWorkerDir("worker-1"))
	assert.Equal(t, worker2Path, h.GetWorkerDir("worker-2"))

	assert.DirExists(t, worker1Path)
	assert.DirExists(t, worker2Path)
}

func TestHarnessArmBinPathSet_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	armBin := "/path/to/arm"
	h := harness.New(t, armBin)

	assert.Equal(t, armBin, h.ArmBinPath)
}

func TestHarnessOriginIsAccessible_REQ_TOPTIER_S3_T1(t *testing.T) {
	t.Parallel()

	h := harness.New(t, "arm")

	remoteFile := filepath.Join(h.WorkDir, ".git", "config")
	require.FileExists(t, remoteFile)

	configContent, err := os.ReadFile(remoteFile)
	require.NoError(t, err)

	assert.Contains(t, string(configContent), "origin")
}
