package doctor_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/gittest"
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

func TestEvaluateD12OpsWorktreeLag_RedactsFetchErrorCredentials(t *testing.T) {
	t.Parallel()

	userinfoSecret := "ghp_fakeSecretTokenForTest1234567890"
	querySecret := "SUPER" + "SECRET"
	pathSecret := "ghp_fakeSecretTokenForTest1234567890"
	cases := []struct {
		name   string
		secret string
		url    string
		keep   string
		phrase string
	}{
		{
			name:   "userinfo",
			secret: userinfoSecret,
			url:    "https://x-access-token:" + userinfoSecret + "@github.com/org/repo.git/",
			keep:   "https://github.com/***",
			phrase: "403",
		},
		{
			name:   "query-sig",
			secret: querySecret,
			url:    "https://host/repo.git?sig=" + querySecret,
			keep:   "https://host/***",
			phrase: "403",
		},
		{
			name:   "query-token",
			secret: querySecret,
			url:    "https://host/repo.git?token=" + querySecret,
			keep:   "https://host/***",
			phrase: "403",
		},
		{
			name:   "query-access-token",
			secret: querySecret,
			url:    "https://host/repo.git?access_token=" + querySecret,
			keep:   "https://host/***",
			phrase: "403",
		},
		{
			name:   "query-presigned",
			secret: querySecret,
			url:    "https://bucket.s3.amazonaws.com/repo.git?X-Amz-Signature=" + querySecret,
			keep:   "https://bucket.s3.amazonaws.com/***",
			phrase: "403",
		},
		{
			name:   "path-token",
			secret: pathSecret,
			url:    "https://host/" + pathSecret + "/org/repo.git/",
			keep:   "https://host/***",
			phrase: "403",
		},
		{
			name:   "path-jwt",
			secret: "eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiJ0ZXN0In0." + "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			url: "https://host/t/" +
				"eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiJ0ZXN0In0." + "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" +
				"/repo.git/",
			keep:   "https://host/***",
			phrase: "403",
		},
		{
			name:   "opaque-path",
			secret: "OPAQUE" + "SECRET123",
			url:    "https://127.0.0.1:1/signed/" + "OPAQUE" + "SECRET123" + "/repo.git",
			keep:   "https://127.0.0.1:1/***",
			phrase: "Could not resolve host",
		},
		{
			name:   "scp-style",
			secret: "OPAQUE" + "SECRET123",
			url:    "git@github.com:org/" + "OPAQUE" + "SECRET123" + "/repo.git",
			keep:   "github.com:***",
			phrase: "Authentication failed",
		},
		{
			name:   "ftp-opaque-path",
			secret: "OPAQUE" + "SECRET123",
			url:    "ftp://127.0.0.1:1/signed/" + "OPAQUE" + "SECRET123" + "/repo.git",
			keep:   "ftp://127.0.0.1:1/***",
			phrase: "Could not resolve host",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fetchErr := fmt.Errorf(
				"git fetch origin _armature: exit status 128\n"+
					"fatal: unable to access '%s': %s",
				tc.url, tc.phrase,
			)
			assertFindingOmitsSecret := func(t *testing.T, f doctor.Finding) {
				t.Helper()
				assert.Equal(t, doctor.SeverityError, f.Severity)
				assert.Contains(t, f.Message, "Could not fetch origin/_armature")
				assert.Contains(t, f.Message, "may be stale")
				assert.Contains(t, f.Message, tc.keep)
				assert.Contains(t, f.Message, tc.phrase)
				assert.NotContains(t, f.Message, tc.secret)
				for _, item := range f.Items {
					assert.NotContains(t, item, tc.secret)
				}

				raw, err := json.Marshal(f)
				require.NoError(t, err)
				assert.NotContains(t, string(raw), tc.secret)

				var human strings.Builder
				fmt.Fprintf(&human, "✗ %s: %s\n", f.Check, f.Message)
				for _, item := range f.Items {
					fmt.Fprintf(&human, "    - %s\n", item)
				}
				assert.NotContains(t, human.String(), tc.secret)
			}

			staleOK := doctor.EvaluateD12OpsWorktreeLag(0, fetchErr)
			assert.Contains(t, staleOK.Message, "appears not behind")
			assertFindingOmitsSecret(t, staleOK)

			staleBehind := doctor.EvaluateD12OpsWorktreeLag(3, fetchErr)
			assert.Contains(t, staleBehind.Message, "3 commit(s) behind")
			assertFindingOmitsSecret(t, staleBehind)
		})
	}
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

func TestRun_D12_ErrorWhenFetchFailsNotBehind(t *testing.T) {
	t.Parallel()
	worktree, _ := opsWorktreeWithOrigin(t)
	missingOrigin := filepath.Join(t.TempDir(), "missing-origin.git")
	runGit(t, worktree, "remote", "set-url", "origin", "file://"+missingOrigin)

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
	missingOrigin := filepath.Join(t.TempDir(), "missing-origin.git")
	runGit(t, worktree, "remote", "set-url", "origin", "file://"+missingOrigin)

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
