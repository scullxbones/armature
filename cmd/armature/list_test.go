package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	listHostURLA = "https://github.com/scullxbones/armature/issues/1"
	listHostURLB = "https://github.com/scullxbones/armature/issues/2"
)

func appendExternalRefOp(t *testing.T, repo, targetID, url, role, withdraw string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	payload := ops.Payload{}
	if withdraw != "" {
		payload.WithdrawExternalRef = withdraw
	} else {
		payload.ExternalRef = url
		payload.ExternalRole = role
	}
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpExternalRef, TargetID: targetID, Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: payload,
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
}

func runListJSON(t *testing.T, repo string, extra ...string) string {
	t.Helper()
	args := append([]string{"--format", "json", "list"}, extra...)
	out, err := runTrls(t, repo, args...)
	require.NoError(t, err)
	return out
}

func decodeListEnvelope(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON value, got %q", stdout)
	require.True(t, strings.HasPrefix(raw, "{"), "envelope must be an object, not a bare array")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")
	return decoded
}

func TestListDefaultRowIsFourFields_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithStoryAndTask(t)

	decoded := decodeListEnvelope(t, runListJSON(t, repo))

	var issues []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	require.NotEmpty(t, issues)

	wantKeys := []string{"id", "type", "status", "title"}
	for _, row := range issues {
		require.Len(t, row, 4, "default list row must have exactly four keys")
		for _, key := range wantKeys {
			_, ok := row[key]
			assert.True(t, ok, "default list row missing %q", key)
			_, isString := row[key].(string)
			assert.True(t, isString, "default list row %q must be a string", key)
		}
	}

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Equal(t, output.ListShowHelp, help[0])
}

func TestListOmitsOutcome_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", "completed with a long outcome body", "--force")
	require.NoError(t, err)

	out := runListJSON(t, repo)
	assert.NotContains(t, out, `"outcome"`, "outcome must be omitted from list output, not truncated")

	decoded := decodeListEnvelope(t, out)
	var issues []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	require.NotEmpty(t, issues)
	for _, row := range issues {
		_, hasOutcome := row["outcome"]
		assert.False(t, hasOutcome, "list row must not carry outcome")
	}
}

func TestListGroupHonouredInStructuredOutput_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithStoryAndTask(t)

	ungrouped := runListJSON(t, repo)
	grouped := runListJSON(t, repo, "--group")
	assert.NotEqual(t, ungrouped, grouped, "--group must change structured output")

	plain := decodeListEnvelope(t, ungrouped)
	_, hasGroups := plain["groups"]
	assert.False(t, hasGroups, "ungrouped list must not carry a groups adjunct")

	withGroup := decodeListEnvelope(t, grouped)
	require.Contains(t, withGroup, "groups")

	var issues []output.ListIssue
	require.NoError(t, json.Unmarshal(withGroup["issues"], &issues))
	require.NotEmpty(t, issues)

	var groups []output.ListGroup
	require.NoError(t, json.Unmarshal(withGroup["groups"], &groups))
	require.NotEmpty(t, groups)

	seen := map[string]bool{}
	var groupedIDs []string
	index := make(map[string]output.ListFacts, len(issues))
	ids := make([]string, 0, len(issues))
	for _, row := range issues {
		index[row.ID] = output.ListFacts{Type: row.Type, Status: row.Status, Title: row.Title}
		ids = append(ids, row.ID)
	}
	assert.Equal(t, output.ListGroupsByStatus(index, ids), groups, "group order must match ListGroupsByStatus")
	for _, g := range groups {
		assert.NotEmpty(t, g.Status)
		assert.NotEmpty(t, g.IDs)
		for _, id := range g.IDs {
			assert.False(t, seen[id], "grouped ids must not duplicate")
			seen[id] = true
			groupedIDs = append(groupedIDs, id)
		}
	}

	var issueIDs []string
	for _, row := range issues {
		issueIDs = append(issueIDs, row.ID)
		assert.True(t, seen[row.ID], "every issue must appear in groups")
	}
	assert.ElementsMatch(t, issueIDs, groupedIDs)

	var count int
	require.NoError(t, json.Unmarshal(withGroup["count"], &count))
	assert.Equal(t, len(issues), count)
}

func TestListCountIsTrueTotal_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithStoryAndTask(t)

	decoded := decodeListEnvelope(t, runListJSON(t, repo))
	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	var issues []output.ListIssue
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	assert.Equal(t, len(issues), count, "count must equal payload length")
	assert.Equal(t, 3, count, "count is the true unfiltered total, not a cap")

	emptyOut := runListJSON(t, repo, "--status", "cancelled")
	empty := decodeListEnvelope(t, emptyOut)
	require.NoError(t, json.Unmarshal(empty["count"], &count))
	assert.Equal(t, 0, count)
	var emptyIssues []output.ListIssue
	require.NoError(t, json.Unmarshal(empty["issues"], &emptyIssues))
	assert.Empty(t, emptyIssues)
	assert.Equal(t, "[]", strings.TrimSpace(string(empty["issues"])))

	var help []string
	require.NoError(t, json.Unmarshal(empty["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], "filter")
	foundShow := false
	for _, h := range help {
		if strings.Contains(h, "arm show") {
			foundShow = true
		}
	}
	assert.True(t, foundShow, "empty list help must still point at arm show")
}

func TestListExternalRefFiltersLiveNames_REQ_HOSTREF_S1_T9(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithStoryAndTask(t)

	appendExternalRefOp(t, repo, "task-01", listHostURLA, "engineering", "")
	appendExternalRefOp(t, repo, "task-01", listHostURLB, "support", "")
	plantVerifiedTask(t, repo, "task-02", "src/other.go")
	appendExternalRefOp(t, repo, "task-02", listHostURLB, "engineering", "")
	plantVerifiedTask(t, repo, "task-03", "src/withdrawn.go")
	appendExternalRefOp(t, repo, "task-03", listHostURLA, "support", "")
	appendExternalRefOp(t, repo, "task-03", "", "", listHostURLA)

	decoded := decodeListEnvelope(t, runListJSON(t, repo, "--external-ref", listHostURLA))
	var issues []output.ListIssue
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	require.Len(t, issues, 1)
	assert.Equal(t, "task-01", issues[0].ID)

	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	assert.Equal(t, 1, count)

	rawIn := "https://GitHub.com/Scullxbones/Armature/issues/01?utm=1#frag"
	canonical, err := externalref.ParseGitHubIssueURL(rawIn)
	require.NoError(t, err)
	assert.Equal(t, listHostURLA, canonical.URL())

	decodedRaw := decodeListEnvelope(t, runListJSON(t, repo, "--external-ref", rawIn))
	require.NoError(t, json.Unmarshal(decodedRaw["issues"], &issues))
	require.Len(t, issues, 1)
	assert.Equal(t, "task-01", issues[0].ID)

	decodedB := decodeListEnvelope(t, runListJSON(t, repo, "--external-ref", listHostURLB))
	require.NoError(t, json.Unmarshal(decodedB["issues"], &issues))
	ids := make([]string, 0, len(issues))
	for _, row := range issues {
		ids = append(ids, row.ID)
	}
	assert.ElementsMatch(t, []string{"task-01", "task-02"}, ids)

	empty := decodeListEnvelope(t, runListJSON(t, repo, "--external-ref", "https://github.com/scullxbones/armature/issues/999"))
	require.NoError(t, json.Unmarshal(empty["count"], &count))
	assert.Equal(t, 0, count)

	_, err = runTrls(t, repo, "list", "--external-ref", "https://linear.app/team/issue/ABC-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, externalref.ErrInvalidGitHubIssueURL)

	human, err := runTrls(t, repo, "--format", "human", "list", "--external-ref", listHostURLA)
	require.NoError(t, err)
	assert.Contains(t, human, "task-01")
	assert.Contains(t, human, listHostURLA)
	assert.Contains(t, human, "role=engineering")
	assert.Contains(t, human, listHostURLB)
	assert.Contains(t, human, "current")
	assert.NotContains(t, human, "task-02")
	assert.NotContains(t, human, "task-03")
	assert.NotContains(t, human, `"body"`)
	assert.NotContains(t, strings.ToLower(human), "plan text")
}

func TestFormatHostRefLinesMarksCurrent_REQ_HOSTREF_S1_T9(t *testing.T) {
	t.Parallel()
	refs := []materialize.HostRef{
		{URL: listHostURLA, Role: "engineering"},
		{URL: listHostURLB, Role: "support"},
		{URL: listHostURLA, Withdrawn: true},
		{URL: listHostURLA, Role: "engineering"},
	}
	lines := formatHostRefLines(refs)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], listHostURLB)
	assert.Contains(t, lines[0], "role=support")
	assert.NotContains(t, lines[0], "current")
	assert.Contains(t, lines[1], listHostURLA)
	assert.Contains(t, lines[1], "role=engineering")
	assert.Contains(t, lines[1], "current")
}
