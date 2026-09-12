package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Equal(t, listShowHelp, help[0])
}

func TestListOmitsOutcome_REQ_AOC_S2_T2(t *testing.T) {
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
	repo := setupRepoWithStoryAndTask(t)

	ungrouped := runListJSON(t, repo)
	grouped := runListJSON(t, repo, "--group")
	assert.NotEqual(t, ungrouped, grouped, "--group must change structured output")

	plain := decodeListEnvelope(t, ungrouped)
	_, hasGroups := plain["groups"]
	assert.False(t, hasGroups, "ungrouped list must not carry a groups adjunct")

	withGroup := decodeListEnvelope(t, grouped)
	require.Contains(t, withGroup, "groups")

	var issues []listEntry
	require.NoError(t, json.Unmarshal(withGroup["issues"], &issues))
	require.NotEmpty(t, issues)

	var groups []listGroup
	require.NoError(t, json.Unmarshal(withGroup["groups"], &groups))
	require.NotEmpty(t, groups)

	seen := map[string]bool{}
	var groupedIDs []string
	for i, g := range groups {
		assert.NotEmpty(t, g.Status)
		assert.NotEmpty(t, g.IDs)
		if i > 0 {
			assert.LessOrEqual(t, output.ListStatusRank(groups[i-1].Status), output.ListStatusRank(g.Status))
		}
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
	repo := setupRepoWithStoryAndTask(t)

	decoded := decodeListEnvelope(t, runListJSON(t, repo))
	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	var issues []listEntry
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	assert.Equal(t, len(issues), count, "count must equal payload length")
	assert.Equal(t, 3, count, "count is the true unfiltered total, not a cap")

	emptyOut := runListJSON(t, repo, "--status", "cancelled")
	empty := decodeListEnvelope(t, emptyOut)
	require.NoError(t, json.Unmarshal(empty["count"], &count))
	assert.Equal(t, 0, count)
	var emptyIssues []listEntry
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
