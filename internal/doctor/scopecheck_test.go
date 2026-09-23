package doctor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/doctor"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func commitAll(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "test commit")
}

func TestDoctorScopeCheck_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	t.Run("StrayBinaryOutsideScope", func(t *testing.T) {
		t.Parallel()
		tmpDir := initGitRepo(t)

		scopeDir := filepath.Join(tmpDir, "internal", "util")
		require.NoError(t, os.MkdirAll(scopeDir, 0o755))

		strayBinary := filepath.Join(tmpDir, "stray_binary")
		require.NoError(t, os.WriteFile(strayBinary, []byte{0x7f, 'E', 'L', 'F'}, 0o755))

		now := time.Now()
		index := materialize.Index{
			"TASK-001": {Status: "claimed", Type: "task"},
		}
		allIssues := map[string]*materialize.Issue{
			"TASK-001": {
				ID:        "TASK-001",
				Status:    "claimed",
				Type:      "task",
				Scope:     []string{"internal/util/"},
				ClaimedAt: now.Unix(),
				Updated:   now.Unix(),
			},
		}

		finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
		assert.Equal(t, doctor.SeverityError, finding.Severity)
		assert.Equal(t, "D8", finding.Check)
		require.NotEmpty(t, finding.Items)
		found := false
		for _, item := range finding.Items {
			if item == "TASK-001: stray_binary" {
				found = true
			}
		}
		assert.True(t, found, "expected the stray binary to be reported: %v", finding.Items)
	})

	t.Run("OutOfScopeFileViaScopePolicyCheckPaths", func(t *testing.T) {
		t.Parallel()

		policy := harnesspolicy.NewScopePolicyWithRoot([]string{"internal/auth/"}, "/workspace/repo")

		result := policy.CheckPaths([]string{"cmd/main.go"})
		assert.False(t, result.Allowed)
		require.NotEmpty(t, result.Violations)
		assert.Equal(t, "cmd/main.go", result.Violations[0].Path)
		assert.Contains(t, result.Message(), "outside task scope")

		inScopeResult := policy.CheckPaths([]string{"internal/auth/login.go"})
		assert.True(t, inScopeResult.Allowed)
		assert.Empty(t, inScopeResult.Violations)
	})

	t.Run("MainWorktreeLeakScopedToTaskDeclaredScope", func(t *testing.T) {
		t.Parallel()
		tmpDir := initGitRepo(t)

		scopeDir := filepath.Join(tmpDir, "internal", "auth")
		require.NoError(t, os.MkdirAll(scopeDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "login.go"), []byte("package auth"), 0o644))
		commitAll(t, tmpDir)

		leakedDir := filepath.Join(tmpDir, "cmd")
		require.NoError(t, os.MkdirAll(leakedDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(leakedDir, "leaked.go"), []byte("package main"), 0o644))

		now := time.Now()
		index := materialize.Index{
			"TASK-001": {Status: "in-progress", Type: "task"},
		}
		allIssues := map[string]*materialize.Issue{
			"TASK-001": {
				ID:        "TASK-001",
				Status:    "in-progress",
				Type:      "task",
				Scope:     []string{"internal/auth/"},
				ClaimedAt: now.Unix(),
				Updated:   now.Unix(),
			},
		}

		finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
		assert.Equal(t, doctor.SeverityError, finding.Severity)
		require.NotEmpty(t, finding.Items)
		found := false
		for _, item := range finding.Items {
			if item == "TASK-001: cmd/leaked.go" {
				found = true
			}
		}
		assert.True(t, found, "expected the leaked out-of-scope file to be reported: %v", finding.Items)
	})
}

func TestCheckD8ScopeViolations_NoActiveTasks_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	index := materialize.Index{}
	allIssues := map[string]*materialize.Issue{}

	untracked := filepath.Join(tmpDir, "untracked.txt")
	err := os.WriteFile(untracked, []byte("content"), 0o644)
	require.NoError(t, err)

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, time.Now())
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

func TestCheckD8ScopeViolations_NoOutOfScopeArtifacts_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err := os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:        "TASK-001",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/auth/"},
			ClaimedAt: time.Now().Unix(),
			Updated:   time.Now().Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, time.Now())
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

func TestCheckD8ScopeViolations_OutOfScopeArtifacts_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err := os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	outOfScopeDir := filepath.Join(tmpDir, "cmd")
	err = os.MkdirAll(outOfScopeDir, 0o755)
	require.NoError(t, err)

	outOfScopeFile := filepath.Join(outOfScopeDir, "main.go")
	err = os.WriteFile(outOfScopeFile, []byte("package main"), 0o644)
	require.NoError(t, err)

	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	now := time.Now()
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:        "TASK-001",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/auth/"},
			ClaimedAt: now.Unix(),
			Updated:   now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityError, finding.Severity)
	assert.Equal(t, "D8", finding.Check)
	require.NotEmpty(t, finding.Items)
	assert.Contains(t, finding.Items, "TASK-001: cmd/main.go")
}

func TestCheckD8ScopeViolations_CommittedOutOfScopeFile_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	require.NoError(t, os.MkdirAll(scopeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "login.go"), []byte("package auth"), 0o644))

	otherDir := filepath.Join(tmpDir, "internal", "billing")
	require.NoError(t, os.MkdirAll(otherDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, "invoice.go"), []byte("package billing"), 0o644))

	commitAll(t, tmpDir)

	now := time.Now()
	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:        "TASK-001",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/auth/"},
			ClaimedAt: now.Unix(),
			Updated:   now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityOK, finding.Severity, "committed out-of-scope files must not be flagged: %v", finding.Items)
	assert.Empty(t, finding.Items)
}

func TestCheckD8ScopeViolations_IgnoresGeneralHygiene_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err := os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	unrelatedFile := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(unrelatedFile, []byte("# Project"), 0o644)
	require.NoError(t, err)

	now := time.Now()
	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:        "TASK-001",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/auth/"},
			ClaimedAt: now.Unix(),
			Updated:   now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

func TestCheckD8ScopeViolations_DoesNotCrossAttributeBetweenActiveTasks_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeADir := filepath.Join(tmpDir, "internal", "a")
	require.NoError(t, os.MkdirAll(scopeADir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scopeADir, "foo.go"), []byte("package a"), 0o644))

	scopeBDir := filepath.Join(tmpDir, "internal", "b")
	require.NoError(t, os.MkdirAll(scopeBDir, 0o755))

	now := time.Now()
	index := materialize.Index{
		"TASK-A": {Status: "claimed", Type: "task"},
		"TASK-B": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-A": {
			ID:        "TASK-A",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/a/"},
			ClaimedAt: now.Unix(),
			Updated:   now.Unix(),
		},
		"TASK-B": {
			ID:        "TASK-B",
			Status:    "claimed",
			Type:      "task",
			Scope:     []string{"internal/b/"},
			ClaimedAt: now.Unix(),
			Updated:   now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	for _, item := range finding.Items {
		assert.NotEqual(t, "TASK-B: internal/a/foo.go", item, "TASK-A's in-scope file must not be cross-attributed to TASK-B: %v", finding.Items)
	}
	assert.Equal(t, doctor.SeverityOK, finding.Severity,
		"no genuine out-of-scope artifact exists; TASK-A's file is explained by TASK-A's own scope: %v", finding.Items)
}

func TestCheckD8ScopeViolations_RecentlyCompleted_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "util")
	err := os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	outOfScopeFile := filepath.Join(tmpDir, "stray.bin")
	err = os.WriteFile(outOfScopeFile, []byte("binary"), 0o644)
	require.NoError(t, err)

	now := time.Now()

	index := materialize.Index{
		"TASK-001": {Status: "done", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:      "TASK-001",
			Status:  "done",
			Type:    "task",
			Scope:   []string{"internal/util/"},
			Updated: now.Add(-2 * time.Minute).Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityError, finding.Severity, "in-grace-period out-of-scope artifact should be flagged")
	assert.Equal(t, "D8", finding.Check)
	assert.Contains(t, finding.Items, "TASK-001: stray.bin")
}

func TestCheckD8ScopeViolations_RecentlyCompleted_OutsideGracePeriod_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)

	scopeDir := filepath.Join(tmpDir, "internal", "util")
	err := os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	outOfScopeFile := filepath.Join(tmpDir, "stray.bin")
	err = os.WriteFile(outOfScopeFile, []byte("binary"), 0o644)
	require.NoError(t, err)

	now := time.Now()

	index := materialize.Index{
		"TASK-001": {Status: "done", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:      "TASK-001",
			Status:  "done",
			Type:    "task",
			Scope:   []string{"internal/util/"},
			Updated: now.Add(-2 * time.Hour).Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityOK, finding.Severity, "out-of-grace-period task should not be checked")
}

func TestCheckD8ScopeViolations_GitDirtyProbeFailure(t *testing.T) {
	t.Parallel()
	notGit := t.TempDir()
	now := time.Now()
	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:      "TASK-001",
			Status:  "claimed",
			Type:    "task",
			Scope:   []string{"internal/auth/"},
			Updated: now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, notGit, now)
	assert.Equal(t, "D8", finding.Check)
	assert.Equal(t, doctor.SeverityError, finding.Severity)
	assert.Equal(t, "Could not list git-dirty paths", finding.Message)
	require.NotEmpty(t, finding.Items)
	assert.Contains(t, finding.Items[0], "git status --porcelain")
}

func TestCheckD8ScopeViolations_EmptyDirtySetIsOK(t *testing.T) {
	t.Parallel()
	tmpDir := initGitRepo(t)
	now := time.Now()
	index := materialize.Index{
		"TASK-001": {Status: "claimed", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:      "TASK-001",
			Status:  "claimed",
			Type:    "task",
			Scope:   []string{"internal/auth/"},
			Updated: now.Unix(),
		},
	}

	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityOK, finding.Severity, "empty dirty set must not be treated as a probe failure: %v", finding.Items)
	assert.Empty(t, finding.Items)
}
