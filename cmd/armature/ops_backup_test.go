package main

import (
	"bytes"
	"encoding/json"
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

func setupOpsBackupFreshClone(t *testing.T) string {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
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
	var snapEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &snapEnv))
	require.Contains(t, snapEnv, "count")
	require.Contains(t, snapEnv, "snapshots")
	require.Contains(t, snapEnv, "help")
	require.FileExists(t, bundle)

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "verify", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	var verifyEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &verifyEnv))
	require.Contains(t, verifyEnv, "verifications")
	require.Contains(t, verifyEnv, "help")

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "restore-plan", "--from", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	var planEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &planEnv))
	require.Contains(t, planEnv, "plans")
	require.Contains(t, planEnv, "help")
	assert.Contains(t, out.String(), "fast-forward")
}

func TestOpsBackupVerifyRestorePlan_FreshCloneNoLayout_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupFreshClone(t)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute(), "snapshot must work without armature layout")
	require.FileExists(t, bundle)

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "verify", bundle, "--repo", repo, "--format", "human", "--non-interactive"})
	require.NoError(t, root.Execute(), "verify must work on a fresh clone")
	assert.Contains(t, out.String(), "verify ok")

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "restore-plan", "--from", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute(), "restore-plan must work on a fresh clone")
	assert.Contains(t, out.String(), `"plans"`)
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
