package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShow_BlockedBy verifies that arm show displays blocked_by and blocks lists
// when they are non-empty, in both human-readable and JSON formats.
func TestShow_BlockedBy(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create three tasks: t1 blocks t2, t2 is blocked_by t1, t3 is independent
	_, err = runTrls(t, repo, "create", "--id", "blk-1", "--title", "Blocker task", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "blk-2", "--title", "Blocked task", "--type", "task")
	require.NoError(t, err)

	// Link: blk-2 is blocked_by blk-1 (engine processes "blocked_by" rel on source,
	// and also sets blk-1.Blocks = [blk-2] as the symmetric side).
	_, err = runTrls(t, repo, "link", "--source", "blk-2", "--dep", "blk-1", "--rel", "blocked_by")
	require.NoError(t, err)

	t.Run("human-readable shows BlockedBy", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "blk-2")
		require.NoError(t, err)
		assert.Contains(t, out, "BlockedBy:", "blk-2 should show BlockedBy field")
		assert.Contains(t, out, "blk-1", "blk-2 should list blk-1 as its blocker")
	})

	t.Run("human-readable shows Blocks", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "blk-1")
		require.NoError(t, err)
		assert.Contains(t, out, "Blocks:", "blk-1 should show Blocks field")
		assert.Contains(t, out, "blk-2", "blk-1 should list blk-2 as what it blocks")
	})

	t.Run("JSON output includes blocked_by", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "--format", "json", "blk-2")
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
		blockedBy, ok := result["blocked_by"]
		require.True(t, ok, "JSON output should contain blocked_by key")
		blockedByList, ok := blockedBy.([]any)
		require.True(t, ok, "blocked_by should be an array")
		assert.Len(t, blockedByList, 1)
		assert.Equal(t, "blk-1", blockedByList[0])
	})

	t.Run("JSON output includes blocks", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "--format", "json", "blk-1")
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
		blocks, ok := result["blocks"]
		require.True(t, ok, "JSON output should contain blocks key")
		blocksList, ok := blocks.([]any)
		require.True(t, ok, "blocks should be an array")
		assert.Len(t, blocksList, 1)
		assert.Equal(t, "blk-2", blocksList[0])
	})

	t.Run("omits BlockedBy when empty", func(t *testing.T) {
		// blk-1 is not blocked by anything
		out, err := runTrls(t, repo, "show", "blk-1")
		require.NoError(t, err)
		assert.NotContains(t, out, "BlockedBy:", "blk-1 has no blockers and should not show BlockedBy")
	})

	t.Run("omits Blocks when empty", func(t *testing.T) {
		// blk-2 does not block anything
		out, err := runTrls(t, repo, "show", "blk-2")
		require.NoError(t, err)
		assert.NotContains(t, out, "Blocks:", "blk-2 blocks nothing and should not show Blocks")
	})
}

// TestShow_BlockedBy_MultiJSON verifies that the multi-issue JSON array path
// also includes blocked_by and blocks fields.
func TestShow_BlockedBy_MultiJSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "mblk-1", "--title", "Multi Blocker", "--type", "task")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "mblk-2", "--title", "Multi Blocked", "--type", "task")
	require.NoError(t, err)
	// Link: mblk-2 is blocked_by mblk-1 — engine sets symmetric Blocks on mblk-1.
	_, err = runTrls(t, repo, "link", "--source", "mblk-2", "--dep", "mblk-1", "--rel", "blocked_by")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "mblk-1", "mblk-2")
	require.NoError(t, err)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &results))
	require.Len(t, results, 2)

	// Find mblk-1 and mblk-2 entries
	var entry1, entry2 map[string]any
	for _, r := range results {
		switch r["id"] {
		case "mblk-1":
			entry1 = r
		case "mblk-2":
			entry2 = r
		}
	}
	require.NotNil(t, entry1, "mblk-1 must be in results")
	require.NotNil(t, entry2, "mblk-2 must be in results")

	blocksList, ok := entry1["blocks"].([]any)
	require.True(t, ok, "mblk-1 blocks field should be an array")
	assert.Equal(t, []any{"mblk-2"}, blocksList)

	blockedByList, ok := entry2["blocked_by"].([]any)
	require.True(t, ok, "mblk-2 blocked_by field should be an array")
	assert.Equal(t, []any{"mblk-1"}, blockedByList)
}
