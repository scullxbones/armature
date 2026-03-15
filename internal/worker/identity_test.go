package worker

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", name, args, out)
}

func TestGenerateAndStoreWorkerID(t *testing.T) {
	repo := initTempRepo(t)

	id, err := InitWorker(repo)
	require.NoError(t, err)
	assert.Len(t, id, 36) // UUID format

	// Check reads back
	got, err := GetWorkerID(repo)
	require.NoError(t, err)
	assert.Equal(t, id, got)
}

func TestGetWorkerID_NotSet(t *testing.T) {
	repo := initTempRepo(t)

	_, err := GetWorkerID(repo)
	assert.Error(t, err)
}

func TestCheckWorkerID(t *testing.T) {
	repo := initTempRepo(t)

	ok, _ := CheckWorkerID(repo)
	assert.False(t, ok)

	InitWorker(repo)

	ok, id := CheckWorkerID(repo)
	assert.True(t, ok)
	assert.Len(t, id, 36)
}
