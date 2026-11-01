package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOpsBackupRepo(t *testing.T) string {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := writeArmatureLayout(repo, "ops-backup-worker")
	require.NoError(t, err)
	gittest.Git(t, repo, "checkout", "--orphan", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	gittest.Git(t, repo, "checkout", "main")
	return repo
}

func TestOpsBackupSnapshotVerifyRestorePlan_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupRepo(t)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), `"status":"ok"`)
	require.FileExists(t, bundle)

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "verify", bundle, "--repo", repo, "--format", "human", "--non-interactive"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "verify ok")

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "restore-plan", "--from", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "fast-forward")
}

func TestOpsBackupSnapshot_RefreshFail_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := writeArmatureLayout(repo, "ops-backup-worker")
	require.NoError(t, err)
	bundle := filepath.Join(t.TempDir(), "missing.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	err = root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPS-BACKUP-1")
	assert.NoFileExists(t, bundle)
}

func TestOpsBackupVerify_BadBundle_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupRepo(t)
	bad := filepath.Join(t.TempDir(), "not-a-bundle")
	require.NoError(t, os.WriteFile(bad, []byte("nope"), 0o644))

	root := testRootCmd(t)
	root.SetArgs([]string{"ops-backup", "verify", bad, "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPS-BACKUP-1")
}
