package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoctorModernRepoDoesNotLeakStateDirToCWD verifies that running `arm doctor`
// against a modern (dual-branch, bootstrapped) repo resolves appCtx.StateDir via
// root's PersistentPreRunE (delegation), rather than leaving it empty. Regression
// guard for a bug where doctor's own PersistentPreRunE skipped stateDirFor, causing
// internal/doctor's Materialize to write checkpoint.json/index.json/ready.json/
// traceability.json/issues/ as relative paths into the process's current working
// directory instead of the resolved state dir.
func TestDoctorModernRepoDoesNotLeakStateDirToCWD(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Bootstrap into modern dual-branch layout.
	bootstrapBuf := new(bytes.Buffer)
	bootstrapCmd := newRootCmd()
	bootstrapCmd.SetOut(bootstrapBuf)
	bootstrapCmd.SetArgs([]string{"bootstrap", "--repo", repo})
	require.NoError(t, bootstrapCmd.Execute())

	// Snapshot the test process's own cwd before running doctor, and confirm
	// doctor does not write any of the well-known materialize artifacts there.
	cwd, err := os.Getwd()
	require.NoError(t, err)

	stray := []string{"checkpoint.json", "index.json", "ready.json", "traceability.json", "issues"}
	for _, name := range stray {
		_ = os.Remove(filepath.Join(cwd, name)) //nolint:errcheck // best-effort pre-clean in case of prior failed run
	}
	t.Cleanup(func() {
		for _, name := range stray {
			_ = os.RemoveAll(filepath.Join(cwd, name)) //nolint:errcheck // best-effort cleanup
		}
	})

	doctorBuf := new(bytes.Buffer)
	doctorCmd := newRootCmd()
	doctorCmd.SetOut(doctorBuf)
	doctorCmd.SetArgs([]string{"doctor", "--repo", repo})
	// doctor may return a non-nil error if it finds warnings/errors in the fresh
	// repo; what matters here is that it doesn't write into the cwd.
	_ = doctorCmd.Execute() //nolint:errcheck // intentionally ignored: doctor may report warnings/errors, only cwd-leak matters here

	for _, name := range stray {
		assert.NoFileExists(t, filepath.Join(cwd, name), "doctor must not write %s into the test process cwd", name)
	}

	// And confirm appCtx.StateDir was actually populated (non-empty), which is
	// the root cause fix: doctor's PersistentPreRunE must delegate to root's so
	// StateDir gets set via stateDirFor.
	assert.NotEmpty(t, appCtx.StateDir)
}

// TestDoctorLegacyRepoEmitsDiagnostic verifies that when doctor falls back to the
// legacy single-branch layout detection path, it emits a clear diagnostic to
// stderr pointing the user at `arm bootstrap` to migrate.
func TestDoctorLegacyRepoEmitsDiagnostic(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	// Hand-construct a legacy single-branch .armature/ops layout (no dual-branch
	// worktree, no armature.ops-worktree-path git config), which root's
	// PersistentPreRunE / config.ResolveContext cannot resolve.
	legacyOps := filepath.Join(repo, ".armature", "ops")
	require.NoError(t, os.MkdirAll(legacyOps, 0o755))

	_, errOut, _ := runTrlsWithStderr(t, repo, "doctor") //nolint:errcheck // intentionally ignored: only stderr diagnostic matters here

	assert.Contains(t, errOut, "legacy single-branch layout detected")
	assert.Contains(t, errOut, "arm bootstrap")
}
