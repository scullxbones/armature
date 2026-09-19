package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesAddCommand_WarnOnRelativePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(stdoutBuf)
	cmd.SetErr(stderrBuf)
	cmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "docs/relative/path.md", "--type", "filesystem", "--title", "Relative"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stdoutBuf.String(), "added source")
	assert.Contains(t, stderrBuf.String(), "relative")
}

func TestSourcesSyncCommand_ErrorOnUnreachablePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	addBuf := new(bytes.Buffer)
	addCmd := newRootCmd()
	addCmd.SetOut(addBuf)
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "/nonexistent/path/does_not_exist.md", "--type", "filesystem"})

	err := addCmd.Execute()
	require.NoError(t, err)

	syncBuf := new(bytes.Buffer)
	syncErrBuf := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncBuf)
	syncCmd.SetErr(syncErrBuf)
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err = syncCmd.Execute()
	assert.Error(t, err, "sync should error on unreachable filesystem path")
	assert.NotContains(t, syncErrBuf.String(), "skip", "should emit error, not silent skip")
}

func TestSourcesSyncCommand_SuccessWithReachablePath(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	err := os.WriteFile(tmpfile, []byte("test source content"), 0600)
	require.NoError(t, err)

	addBuf := new(bytes.Buffer)
	addCmd := newRootCmd()
	addCmd.SetOut(addBuf)
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})

	err = addCmd.Execute()
	require.NoError(t, err)

	syncBuf := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncBuf)
	syncCmd.SetErr(new(bytes.Buffer))
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err = syncCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, syncBuf.String(), "synced")
}

func TestSourcesSyncCommand_PartialFailure(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "good_source.txt")
	require.NoError(t, os.WriteFile(tmpfile, []byte("good content"), 0600))

	addGood := newRootCmd()
	addGood.SetOut(new(bytes.Buffer))
	addGood.SetErr(new(bytes.Buffer))
	addGood.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})
	require.NoError(t, addGood.Execute())

	addBad := newRootCmd()
	addBad.SetOut(new(bytes.Buffer))
	addBad.SetErr(new(bytes.Buffer))
	addBad.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", "/nonexistent/path/missing.md", "--type", "filesystem"})
	require.NoError(t, addBad.Execute())

	syncOut := new(bytes.Buffer)
	syncErr := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncOut)
	syncCmd.SetErr(syncErr)
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})

	err := syncCmd.Execute()
	assert.NoError(t, err, "sync should exit 0 when at least one source succeeded")
	assert.Contains(t, syncOut.String(), "synced", "should report the successful sync")
	assert.NotEmpty(t, syncErr.String(), "should still print error for failed source")
}

func TestSourcesVerifyCommand_StaleAfterSyncFailure(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(tmpfile, []byte("initial content"), 0600))

	addCmd := newRootCmd()
	addCmd.SetOut(new(bytes.Buffer))
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})
	require.NoError(t, addCmd.Execute())

	syncCmd1 := newRootCmd()
	syncOut1 := new(bytes.Buffer)
	syncCmd1.SetOut(syncOut1)
	syncCmd1.SetErr(new(bytes.Buffer))
	syncCmd1.SetArgs([]string{"sources", "sync", "--repo", repo})
	require.NoError(t, syncCmd1.Execute())
	assert.Contains(t, syncOut1.String(), "synced")

	verifyCmd1 := newRootCmd()
	verifyOut1 := new(bytes.Buffer)
	verifyCmd1.SetOut(verifyOut1)
	verifyCmd1.SetErr(new(bytes.Buffer))
	verifyCmd1.SetArgs([]string{"sources", "verify", "--repo", repo})
	err := verifyCmd1.Execute()
	require.NoError(t, err)
	assert.Contains(t, verifyOut1.String(), "OK")

	require.NoError(t, os.Remove(tmpfile))

	syncCmd2 := newRootCmd()
	syncOut2 := new(bytes.Buffer)
	syncErr2 := new(bytes.Buffer)
	syncCmd2.SetOut(syncOut2)
	syncCmd2.SetErr(syncErr2)
	syncCmd2.SetArgs([]string{"sources", "sync", "--repo", repo})
	err = syncCmd2.Execute()
	assert.Error(t, err)
	assert.Contains(t, syncErr2.String(), "fetch")

	verifyCmd2 := newRootCmd()
	verifyOut2 := new(bytes.Buffer)
	verifyCmd2.SetOut(verifyOut2)
	verifyCmd2.SetErr(new(bytes.Buffer))
	verifyCmd2.SetArgs([]string{"sources", "verify", "--repo", repo})
	err = verifyCmd2.Execute()
	assert.Error(t, err)
	assert.Contains(t, verifyOut2.String(), "STALE")
	assert.NotContains(t, verifyOut2.String(), "OK")
}

func gitWorktreeStatus(t *testing.T, repo string) string {
	t.Helper()
	worktreeDir := filepath.Join(repo, ".armature")
	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = worktreeDir
	out, err := cmd.Output()
	require.NoError(t, err, "git status in _armature worktree should succeed")
	return strings.TrimSpace(string(out))
}

func TestSourcesAdd_AutoCommits_REQ_LNGHZN_B1(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(tmpfile, []byte("auto-commit content"), 0600))

	addCmd := newRootCmd()
	addCmd.SetOut(new(bytes.Buffer))
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem", "--title", "Auto Commit"})
	require.NoError(t, addCmd.Execute())

	status := gitWorktreeStatus(t, repo)
	assert.NotContains(t, status, "sources", "manifest write should be auto-committed, leaving no pending sources changes; got status:\n%s", status)
}

func TestSourcesSync_AutoCommits_REQ_LNGHZN_B1(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(tmpfile, []byte("auto-commit sync content"), 0600))

	addCmd := newRootCmd()
	addCmd.SetOut(new(bytes.Buffer))
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})
	require.NoError(t, addCmd.Execute())

	syncCmd := newRootCmd()
	syncCmd.SetOut(new(bytes.Buffer))
	syncCmd.SetErr(new(bytes.Buffer))
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})
	require.NoError(t, syncCmd.Execute())

	status := gitWorktreeStatus(t, repo)
	assert.NotContains(t, status, "sources", "manifest+cache writes should be auto-committed, leaving no pending sources changes; got status:\n%s", status)
}

func TestSourcesCommandOutputParity_REQ_ARCHIMP_S18_T2(t *testing.T) {
	repo := setupRepoWithTask(t)

	tmpfile := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(tmpfile, []byte("parity content"), 0600))

	addCmd := newRootCmd()
	addCmd.SetOut(new(bytes.Buffer))
	addCmd.SetErr(new(bytes.Buffer))
	addCmd.SetArgs([]string{"sources", "add", "--repo", repo,
		"--url", tmpfile, "--type", "filesystem"})
	require.NoError(t, addCmd.Execute())

	syncBuf := new(bytes.Buffer)
	syncCmd := newRootCmd()
	syncCmd.SetOut(syncBuf)
	syncCmd.SetErr(new(bytes.Buffer))
	syncCmd.SetArgs([]string{"sources", "sync", "--repo", repo})
	require.NoError(t, syncCmd.Execute())

	syncLine := regexp.MustCompile(`(?m)^synced \S+  fp=[0-9a-f]{8}$`)
	assert.Regexp(t, syncLine, syncBuf.String(), "sync output format changed")

	verifyBuf := new(bytes.Buffer)
	verifyCmd := newRootCmd()
	verifyCmd.SetOut(verifyBuf)
	verifyCmd.SetErr(new(bytes.Buffer))
	verifyCmd.SetArgs([]string{"sources", "verify", "--repo", repo})
	require.NoError(t, verifyCmd.Execute())

	verifyLine := regexp.MustCompile(`(?m)^\S+ +OK$`)
	assert.Regexp(t, verifyLine, verifyBuf.String(), "verify output format changed")
}
