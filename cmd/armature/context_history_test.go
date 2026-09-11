package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextHistoryScansFullHistoryByDefault_REQ_AOC_S2_T4(t *testing.T) {
	cmd := newContextHistoryCmd()
	limit := cmd.Flags().Lookup("limit")
	require.NotNil(t, limit)
	assert.Equal(t, "0", limit.DefValue, "default --limit must not silently cap history")

	repo, createSHA, noteSHA := plantContextHistoryBeyondSilentCap(t)

	out, err := runTrls(t, repo, "context-history", "--issue", "HIST-01", "--format", "json")
	require.NoError(t, err)

	decoded := decodeContractEnvelope(t, out, "commits")
	_, hasLimit := decoded["limit"]
	assert.False(t, hasLimit, "unbounded scan must not disclose a limit adjunct")

	var commits []contextHistoryRow
	require.NoError(t, json.Unmarshal(decoded["commits"], &commits))
	shas := map[string]bool{}
	for _, c := range commits {
		shas[c.SHA] = true
	}
	assert.True(t, shas[createSHA], "full-history scan must include the oldest context change")
	assert.True(t, shas[noteSHA], "full-history scan must include the newest context change")
}

func TestContextHistoryExplicitLimitIsDisclosed_REQ_AOC_S2_T4(t *testing.T) {
	repo, createSHA, noteSHA := plantContextHistoryBeyondSilentCap(t)

	out, err := runTrls(t, repo, "context-history", "--issue", "HIST-01", "--limit", "10", "--format", "json")
	require.NoError(t, err)

	decoded := decodeContractEnvelope(t, out, "commits")
	require.Contains(t, decoded, "limit")
	var limit int
	require.NoError(t, json.Unmarshal(decoded["limit"], &limit))
	assert.Equal(t, 10, limit)

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], "limit")

	var commits []contextHistoryRow
	require.NoError(t, json.Unmarshal(decoded["commits"], &commits))
	shas := map[string]bool{}
	for _, c := range commits {
		shas[c.SHA] = true
	}
	assert.True(t, shas[noteSHA], "a 10-commit window must still include the tip change")
	assert.False(t, shas[createSHA], "a 10-commit window must exclude the oldest change")
}

func plantContextHistoryBeyondSilentCap(t *testing.T) (repo, createSHA, noteSHA string) {
	t.Helper()
	repo = initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create",
		"--id", "HIST-01",
		"--title", "History fixture",
		"--type", "task",
		"--scope", "hist.go",
		"--dod", "History fixture is complete and tested",
		"--acceptance", `[{"type":"test_passes"}]`,
	)
	require.NoError(t, err)

	ctx := getTestContext(t, repo)
	require.NotEmpty(t, ctx.WorktreePath)
	createSHA = gitHeadSHA(t, ctx.WorktreePath)

	for i := 0; i < 120; i++ {
		cmd := exec.Command("git", "-C", ctx.WorktreePath, "commit", "--allow-empty", "-m", fmt.Sprintf("padding %d", i))
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "padding commit %d: %s", i, out)
	}

	_, err = runTrls(t, repo, "note", "--issue", "HIST-01", "--msg", "context change after padding")
	require.NoError(t, err)
	noteSHA = gitHeadSHA(t, ctx.WorktreePath)
	return repo, createSHA, noteSHA
}

func gitHeadSHA(t *testing.T, worktree string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", worktree, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}
