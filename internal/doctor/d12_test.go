package doctor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateD12OpsWorktreeLag(t *testing.T) {
	t.Parallel()

	ok := doctor.EvaluateD12OpsWorktreeLag(0)
	assert.Equal(t, "D12", ok.Check)
	assert.Equal(t, doctor.SeverityOK, ok.Severity)
	assert.Equal(t, "Ops worktree is not behind origin/_armature", ok.Message)

	warn := doctor.EvaluateD12OpsWorktreeLag(3)
	assert.Equal(t, doctor.SeverityWarning, warn.Severity)
	assert.Contains(t, warn.Message, "3 commit(s) behind origin/_armature")
	assert.Equal(t, []string{"3"}, warn.Items)
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
	worktree := gittest.InitRepo(t)
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

func opsWorktreeWithOrigin(t *testing.T) (worktree, originClone string) {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	worktree = fx.Dir
	runGit(t, worktree, "checkout", "-b", "_armature")
	runGit(t, worktree, "commit", "--allow-empty", "-m", "ops base")
	runGit(t, worktree, "push", "-u", "origin", "_armature")

	originClone = filepath.Join(t.TempDir(), "origin-clone")
	runGit(t, t.TempDir(), "clone", fx.Origin, originClone)
	runGit(t, originClone, "checkout", "_armature")
	return worktree, originClone
}
