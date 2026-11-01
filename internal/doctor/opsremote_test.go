package doctor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateD11OpsRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		obs      d11Observation
		severity Severity
		contains string
		items    []string
	}{
		{
			name:     "verified ok",
			severity: SeverityOK,
			contains: "verified",
		},
		{
			name:     "fresh local-only under threshold ok",
			obs:      d11Observation{localOnlyCount: 2, localOnlyAge: time.Hour},
			severity: SeverityOK,
			contains: "verified",
		},
		{
			name:     "missing tracking warns",
			obs:      d11Observation{missingTracking: true},
			severity: SeverityWarning,
			contains: "no verified remote tracking tip",
			items:    []string{d11RemediationHint},
		},
		{
			name: "stale local-only warns",
			obs: d11Observation{
				localOnlyCount: 3,
				localOnlyAge:   localOnlyOpsStaleAfter + time.Minute,
			},
			severity: SeverityWarning,
			contains: "Local-only ops",
			items:    []string{d11RemediationHint},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := evaluateD11OpsRemote(tc.obs)
			assert.Equal(t, "D11", got.Check)
			assert.Equal(t, tc.severity, got.Severity)
			assert.Contains(t, got.Message, tc.contains)
			assert.Equal(t, tc.items, got.Items)
			assert.NotContains(t, got.Message, "docs/ops-backup.md")
			for _, item := range got.Items {
				assert.NotContains(t, item, "docs/ops-backup.md")
				assert.Contains(t, item, "arm ops-backup")
			}
		})
	}
}

func TestDoctorD11_WarnsOnMissingRemoteTracking_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree := gittest.InitRepo(t)
	runGit(t, worktree, "checkout", "-b", "_armature")
	runGit(t, worktree, "commit", "--allow-empty", "-m", "local-only ops")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityWarning, d11.Severity)
	assert.Contains(t, d11.Message, "no verified remote tracking tip")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
	assert.NotContains(t, d11.Message, "docs/ops-backup.md")
}

func TestDoctorD11_WarnsOnStaleLocalOnlyOps_REQ_TOPTIER_S12_T2(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	staleWhen := time.Now().Add(-(localOnlyOpsStaleAfter + 2*time.Hour))
	runGitEnv(t, worktree, map[string]string{
		"GIT_AUTHOR_DATE":    staleWhen.Format(time.RFC3339),
		"GIT_COMMITTER_DATE": staleWhen.Format(time.RFC3339),
	}, "commit", "--allow-empty", "-m", "unpublished local ops")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityWarning, d11.Severity)
	assert.Contains(t, d11.Message, "Local-only ops")
	assert.Contains(t, d11.Message, "unpublished")
	require.Equal(t, []string{d11RemediationHint}, d11.Items)
	assert.NotContains(t, d11.Message, "docs/ops-backup.md")
}

func TestRun_D11_OKWhenInSync(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Contains(t, d11.Message, "verified")
	assert.Empty(t, d11.Items)
}

func TestRun_D11_SkipWhenWorktreeMissing(t *testing.T) {
	t.Parallel()

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", "", false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Equal(t, "Ops remote tracking not checked", d11.Message)
}

func TestRun_D11_FreshLocalOnlyUnderThresholdOK(t *testing.T) {
	t.Parallel()

	worktree, _ := opsWorktreeWithOrigin(t)
	runGit(t, worktree, "commit", "--allow-empty", "-m", "fresh unpublished")

	issuesDir := initIssuesDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "ops", "test-worker.log"), []byte(""), 0o644))

	report, err := Run(issuesDir, filepath.Join(issuesDir, "state"), "", worktree, false, time.Now())
	require.NoError(t, err)

	d11 := findCheck(t, report, "D11")
	assert.Equal(t, SeverityOK, d11.Severity)
	assert.Contains(t, d11.Message, "verified")
}

func runGitEnv(t *testing.T, dir string, env map[string]string, args ...string) {
	t.Helper()
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	cmd.Env = append(os.Environ(), cmd.Env...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}
