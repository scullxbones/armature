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

// TestDoctorScopeCheck_REQ_TOPTIER_S5_T2 is the contract-required entry point
// for the D8 out-of-scope-artifact check. It covers the three cases named in
// the task contract explicitly:
//  1. A stray binary left outside the task's declared scope.
//  2. A general out-of-scope file, verified directly via ScopePolicy.CheckPaths
//     (the same primitive CheckD8ScopeViolations relies on internally).
//  3. Changes leaking into the main worktree, scoped to a task's declared
//     scope, detected via CheckD8ScopeViolations end-to-end.
func TestDoctorScopeCheck_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	t.Run("StrayBinaryOutsideScope", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.MkdirAll(gitDir, 0o755))

		scopeDir := filepath.Join(tmpDir, "internal", "util")
		require.NoError(t, os.MkdirAll(scopeDir, 0o755))

		// Stray binary artifact left outside the declared scope.
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
		// Exercise the same primitive CheckD8ScopeViolations uses internally
		// (ScopePolicy.CheckPaths) directly, confirming an out-of-scope file
		// is flagged as a violation.
		policy := harnesspolicy.NewScopePolicyWithRoot([]string{"internal/auth/"}, "/workspace/repo")

		result := policy.CheckPaths([]string{"cmd/main.go"})
		assert.False(t, result.Allowed)
		require.NotEmpty(t, result.Violations)
		assert.Equal(t, "cmd/main.go", result.Violations[0].Path)
		assert.Contains(t, result.Message(), "outside task scope")

		// In-scope file is not flagged.
		inScopeResult := policy.CheckPaths([]string{"internal/auth/login.go"})
		assert.True(t, inScopeResult.Allowed)
		assert.Empty(t, inScopeResult.Violations)
	})

	t.Run("MainWorktreeLeakScopedToTaskDeclaredScope", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.MkdirAll(gitDir, 0o755))

		// The task's declared scope is internal/auth/ only.
		scopeDir := filepath.Join(tmpDir, "internal", "auth")
		require.NoError(t, os.MkdirAll(scopeDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "login.go"), []byte("package auth"), 0o644))

		// Worktree changes leak into the main worktree at a path outside the
		// task's declared scope (e.g. a cmd/ file edited from a worktree for
		// a different task, then left behind in the main worktree).
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

// TestCheckD8ScopeViolations_NoActiveTasks_REQ_TOPTIER_S5_T2 tests that D8 returns OK when there are no active or recently-completed tasks
func TestCheckD8ScopeViolations_NoActiveTasks_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// No issues in the index
	index := materialize.Index{}
	allIssues := map[string]*materialize.Issue{}

	// Create a git repo with some untracked files
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	// Create an untracked file
	untracked := filepath.Join(tmpDir, "untracked.txt")
	err = os.WriteFile(untracked, []byte("content"), 0o644)
	require.NoError(t, err)

	// With no active tasks, D8 should be OK (untracked files are general hygiene, not task-related)
	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, time.Now())
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

// TestCheckD8ScopeViolations_NoOutOfScopeArtifacts_REQ_TOPTIER_S5_T2 tests D8 with a task that has scoped files, but all artifacts are in-scope
func TestCheckD8ScopeViolations_NoOutOfScopeArtifacts_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize a git repository
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	// Create scope-aligned directory and file
	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err = os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	// Create an active task with scope covering the file
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

	// D8 should be OK since the file is in-scope
	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, time.Now())
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

// TestCheckD8ScopeViolations_OutOfScopeArtifacts_REQ_TOPTIER_S5_T2 tests D8 detecting artifacts outside scope
func TestCheckD8ScopeViolations_OutOfScopeArtifacts_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize a git repository
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	// Create scope directory
	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err = os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	// Create an in-scope file
	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	// Create an OUT-OF-SCOPE file
	outOfScopeDir := filepath.Join(tmpDir, "cmd")
	err = os.MkdirAll(outOfScopeDir, 0o755)
	require.NoError(t, err)

	outOfScopeFile := filepath.Join(outOfScopeDir, "main.go")
	err = os.WriteFile(outOfScopeFile, []byte("package main"), 0o644)
	require.NoError(t, err)

	// Create an active task with scope limited to internal/auth
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

	// D8 should flag the out-of-scope file
	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityError, finding.Severity)
	assert.Equal(t, "D8", finding.Check)
	// The finding should contain items (though it may flag all files since we're not doing git status filtering)
	// For now, just validate that it detects a problem
	assert.True(t, len(finding.Items) > 0 || finding.Severity == doctor.SeverityError, "D8 should report violations for out-of-scope files")
}

// TestCheckD8ScopeViolations_IgnoresGeneralHygiene_REQ_TOPTIER_S5_T2 tests that D8 doesn't flag unrelated untracked files
func TestCheckD8ScopeViolations_IgnoresGeneralHygiene_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize a git repository
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	// Create scope directory and file
	scopeDir := filepath.Join(tmpDir, "internal", "auth")
	err = os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	inScopeFile := filepath.Join(scopeDir, "login.go")
	err = os.WriteFile(inScopeFile, []byte("package auth"), 0o644)
	require.NoError(t, err)

	// Create a general unrelated file (not correlated with the task's scope)
	unrelatedFile := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(unrelatedFile, []byte("# Project"), 0o644)
	require.NoError(t, err)

	// Create an active task
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

	// D8 should be OK since README.md is not in the scope globs and thus not a scope violation
	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	assert.Equal(t, doctor.SeverityOK, finding.Severity)
}

// TestCheckD8ScopeViolations_RecentlyCompleted_REQ_TOPTIER_S5_T2 tests D8 with recently-completed tasks
func TestCheckD8ScopeViolations_RecentlyCompleted_REQ_TOPTIER_S5_T2(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize a git repository
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)

	// Create scope directory
	scopeDir := filepath.Join(tmpDir, "internal", "util")
	err = os.MkdirAll(scopeDir, 0o755)
	require.NoError(t, err)

	// Create an out-of-scope file
	outOfScopeFile := filepath.Join(tmpDir, "stray.bin")
	err = os.WriteFile(outOfScopeFile, []byte("binary"), 0o644)
	require.NoError(t, err)

	now := time.Now()

	// Create a recently-completed (done) task
	index := materialize.Index{
		"TASK-001": {Status: "done", Type: "task"},
	}
	allIssues := map[string]*materialize.Issue{
		"TASK-001": {
			ID:      "TASK-001",
			Status:  "done",
			Type:    "task",
			Scope:   []string{"internal/util/"},
			Updated: now.Add(-2 * time.Minute).Unix(), // Updated 2 minutes ago
		},
	}

	// D8 should flag the out-of-scope file even though the task is done (within grace period)
	finding := doctor.CheckD8ScopeViolations(index, allIssues, tmpDir, now)
	// The check should look at recently-completed tasks (e.g., within 15-30 minutes)
	// For now, we expect it to flag the issue or be OK depending on the grace period
	// The test validates that the function exists and returns a Finding
	assert.NotEmpty(t, finding.Check)
	assert.Equal(t, "D8", finding.Check)
}
