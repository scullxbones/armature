package doctor_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateD12OpsWorktreeLag(t *testing.T) {
	t.Parallel()

	fetchErr := errors.New("git fetch origin _armature: connection refused")

	ok := doctor.EvaluateD12OpsWorktreeLag(0, nil)
	assert.Equal(t, "D12", ok.Check)
	assert.Equal(t, doctor.SeverityOK, ok.Severity)
	assert.Equal(t, "Ops worktree is not behind origin/_armature", ok.Message)
	assert.Empty(t, ok.Items)

	warn := doctor.EvaluateD12OpsWorktreeLag(3, nil)
	assert.Equal(t, doctor.SeverityWarning, warn.Severity)
	assert.Equal(t, "Ops worktree is 3 commit(s) behind origin/_armature", warn.Message)
	assert.Equal(t, []string{"3"}, warn.Items)
	assert.NotContains(t, warn.Message, "Could not fetch")
	assert.NotContains(t, warn.Message, "may be stale")

	staleOK := doctor.EvaluateD12OpsWorktreeLag(0, fetchErr)
	assert.Equal(t, doctor.SeverityError, staleOK.Severity)
	assert.Contains(t, staleOK.Message, "Could not fetch origin/_armature")
	assert.Contains(t, staleOK.Message, "may be stale")
	assert.Contains(t, staleOK.Message, fetchErr.Error())
	assert.Contains(t, staleOK.Message, "appears not behind")
	assert.Equal(t, []string{fetchErr.Error()}, staleOK.Items)

	staleBehind := doctor.EvaluateD12OpsWorktreeLag(3, fetchErr)
	assert.Equal(t, doctor.SeverityError, staleBehind.Severity)
	assert.Contains(t, staleBehind.Message, "Could not fetch origin/_armature")
	assert.Contains(t, staleBehind.Message, "may be stale")
	assert.Contains(t, staleBehind.Message, fetchErr.Error())
	assert.Contains(t, staleBehind.Message, "3 commit(s) behind")
	assert.Equal(t, []string{"3", fetchErr.Error()}, staleBehind.Items)
}

func TestRun_D12_SkipWhenWorktreeMissing(t *testing.T) {
	t.Parallel()
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityOK, d12.Severity)
	assert.Equal(t, "Ops worktree lag not checked", d12.Message)
}

func TestRun_D12_SkipWhenOriginRefMissing(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	gitInit(t, worktree)
	gitConfigIdentity(t, worktree)
	runGit(t, worktree, "commit", "--allow-empty", "-m", "ops")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityOK, d12.Severity)
	assert.Equal(t, "Ops worktree lag not checked", d12.Message)
}

func TestRun_D12_OKWhenInSync(t *testing.T) {
	t.Parallel()
	worktree, _ := opsWorktreeWithOrigin(t)
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityOK, d12.Severity)
	assert.Equal(t, "Ops worktree is not behind origin/_armature", d12.Message)
}

func TestRun_D12_WarningWhenBehindOrigin(t *testing.T) {
	t.Parallel()
	worktree, originClone := opsWorktreeWithOrigin(t)
	runGit(t, originClone, "commit", "--allow-empty", "-m", "remote ops ahead")
	runGit(t, originClone, "push", "origin", "_armature")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityWarning, d12.Severity)
	assert.Contains(t, d12.Message, "behind origin/_armature")
	require.NotEmpty(t, d12.Items)
}

func TestRun_D12_ErrorWhenFetchFailsNotBehind(t *testing.T) {
	t.Parallel()
	worktree, _ := opsWorktreeWithOrigin(t)
	runGit(t, worktree, "remote", "set-url", "origin", "file:///tmp/armature-d12-missing-origin.git")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityError, d12.Severity)
	assert.Contains(t, d12.Message, "Could not fetch origin/_armature")
	assert.Contains(t, d12.Message, "may be stale")
	require.NotEmpty(t, d12.Items)
}

func TestRun_D12_ErrorWhenFetchFailsBehind(t *testing.T) {
	t.Parallel()
	worktree, originClone := opsWorktreeWithOrigin(t)
	runGit(t, originClone, "commit", "--allow-empty", "-m", "remote ops ahead")
	runGit(t, originClone, "push", "origin", "_armature")
	runGit(t, worktree, "fetch", "origin", "+refs/heads/_armature:refs/remotes/origin/_armature")
	runGit(t, worktree, "remote", "set-url", "origin", "file:///tmp/armature-d12-missing-origin.git")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := doctor.Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)
	d12 := findCheck(t, report, "D12")
	assert.Equal(t, doctor.SeverityError, d12.Severity)
	assert.Contains(t, d12.Message, "Could not fetch origin/_armature")
	assert.Contains(t, d12.Message, "may be stale")
	assert.Contains(t, d12.Message, "behind")
	require.NotEmpty(t, d12.Items)
}

func opsWorktreeWithOrigin(t *testing.T) (worktree, originClone string) {
	t.Helper()
	bare := t.TempDir()
	runGit(t, t.TempDir(), "init", "--bare", bare)

	worktree = t.TempDir()
	gitInit(t, worktree)
	gitConfigIdentity(t, worktree)
	runGit(t, worktree, "checkout", "-b", "_armature")
	runGit(t, worktree, "commit", "--allow-empty", "-m", "ops base")
	runGit(t, worktree, "remote", "add", "origin", "file://"+bare)
	runGit(t, worktree, "push", "-u", "origin", "_armature")

	originClone = t.TempDir()
	runGit(t, t.TempDir(), "clone", "file://"+bare, originClone)
	gitConfigIdentity(t, originClone)
	runGit(t, originClone, "checkout", "_armature")
	return worktree, originClone
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
}

func gitConfigIdentity(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "gc.auto", "0")
	runGit(t, dir, "config", "maintenance.auto", "false")
}
