package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupReadmeOverlapCascade(t *testing.T) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	plantVerifiedTask(t, repo, "TOPTIER-S6-T3", "README.md")
	plantVerifiedTask(t, repo, "TOPTIER-S15-T2", "README.md")
	plantVerifiedTask(t, repo, "TOPTIER-S8-T2", "README.md")
	_, err = runTrls(t, repo, "link", "--source", "TOPTIER-S6-T3", "--dep", "TOPTIER-S8-T2", "--rel", "blocked_by")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "link", "--source", "TOPTIER-S15-T2", "--dep", "TOPTIER-S6-T3", "--rel", "blocked_by")
	require.NoError(t, err)
	return repo
}

func workerLogSnapshot(t *testing.T, repo string) []byte {
	t.Helper()
	ctx := getTestContext(t, repo)
	_, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return data
}

func TestUnlinkRefusesIntroducedOverlap_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := setupReadmeOverlapCascade(t)
	before := workerLogSnapshot(t, repo)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"unlink", "--repo", repo, "--format", "agent",
		"--source", "TOPTIER-S15-T2", "--dep", "TOPTIER-S6-T3")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "UNLINK-1", cf.Code)
	assert.Contains(t, cf.Cause, "TOPTIER-S6-T3")
	assert.Contains(t, cf.Cause, "TOPTIER-S15-T2")
	joined := strings.Join(cf.NextActions, "\n")
	assert.NotContains(t, joined, "arm merged")
	assert.Contains(t, joined, "arm link")

	after := workerLogSnapshot(t, repo)
	assert.Equal(t, before, after, "refused unlink must write no op")
	assert.NotContains(t, string(after), `["unlink"`)
}

func TestUnlinkCauseNamesNextPair_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := setupReadmeOverlapCascade(t)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"unlink", "--repo", repo, "--format", "agent",
		"--source", "TOPTIER-S15-T2", "--dep", "TOPTIER-S6-T3")
	assert.Equal(t, 1, code)

	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "UNLINK-1", cf.Code)
	assert.Contains(t, cf.Cause, "next pair")
	assert.Contains(t, cf.Cause, "TOPTIER-S6-T3")
	assert.Contains(t, cf.Cause, "TOPTIER-S15-T2")
	assert.Contains(t, cf.Cause, "README.md")
	assert.NotContains(t, cf.Cause, "skip-validate-gate")
	assert.NotContains(t, strings.Join(cf.NextActions, "\n"), "merged")
}

func TestLinkRefusesIntroducedCycle_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "link-cycle-a", "--title", "A", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "link-cycle-b", "--title", "B", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "link", "--source", "link-cycle-a", "--dep", "link-cycle-b", "--rel", "blocked_by")
	require.NoError(t, err)
	before := workerLogSnapshot(t, repo)

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"link", "--repo", repo, "--format", "agent",
		"--source", "link-cycle-b", "--dep", "link-cycle-a", "--rel", "blocked_by")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, stdout.String())
	assert.Equal(t, "LINK-1", cf.Code)
	assert.NotContains(t, strings.Join(cf.NextActions, "\n"), "arm merged")
	after := workerLogSnapshot(t, repo)
	assert.Equal(t, before, after)
}

func TestGraphEditOverrideRequiresReason_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := setupReadmeOverlapCascade(t)
	before := workerLogSnapshot(t, repo)

	_, err := runTrls(t, repo, "unlink",
		"--source", "TOPTIER-S15-T2", "--dep", "TOPTIER-S6-T3",
		"--override-validate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--reason")

	_, err = runTrls(t, repo, "unlink",
		"--source", "TOPTIER-S15-T2", "--dep", "TOPTIER-S6-T3",
		"--override-validate", "--reason", "cascade anyway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controlling terminal")
	assert.Equal(t, before, workerLogSnapshot(t, repo))
	assert.NotContains(t, err.Error(), "skip-validate-gate")
}

func TestUnlinkOverrideDoesNotClaimGreen_REQ_AGTFND(t *testing.T) {
	repo := setupReadmeOverlapCascade(t)
	tty, err := os.CreateTemp(t.TempDir(), "tty")
	require.NoError(t, err)
	_, err = tty.WriteString("OVERRIDE-VALIDATE\n")
	require.NoError(t, err)
	require.NoError(t, tty.Close())
	tty, err = os.Open(tty.Name())
	require.NoError(t, err)
	t.Cleanup(func() { _ = tty.Close() })

	origTTY := openControllingTTY
	openControllingTTY = func() (*os.File, error) { return tty, nil }
	t.Cleanup(func() { openControllingTTY = origTTY })

	root := testRootCmd(t)
	require.NoError(t, root.PersistentFlags().Set("non-interactive", "false"))
	require.NoError(t, root.PersistentFlags().Set("format", "human"))
	unlinkCmd, _, findErr := root.Find([]string{"unlink"})
	require.NoError(t, findErr)
	attachExecutionState(unlinkCmd, getTestContext(t, repo))
	err = runGraphEdit(unlinkCmd, ops.OpUnlink, "TOPTIER-S15-T2", "TOPTIER-S6-T3", "blocked_by", true, "human cascade")
	require.NoError(t, err)

	logBytes := string(workerLogSnapshot(t, repo))
	assert.Contains(t, logBytes, `["unlink"`)
	assert.Contains(t, logBytes, "graph edit override: human cascade")
	assert.Contains(t, logBytes, `"skipped_validate_gate":true`)

	out, valErr := runTrls(t, repo, "validate", "--ci", "--format", "human")
	require.Error(t, valErr, "override must not claim green: %s", out)
	assert.Contains(t, out+"\n"+valErr.Error(), "scope overlap")
}
