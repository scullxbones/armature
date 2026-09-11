package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeShowEnvelope(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON value, got %q", stdout)
	require.True(t, strings.HasPrefix(raw, "{"), "envelope must be an object, not a bare issue or array")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")
	return decoded
}

func decodeShowIssues(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	decoded := decodeShowEnvelope(t, stdout)
	var issues []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	assert.Equal(t, len(issues), count, "count must equal issues length")
	return issues
}

func decodeShowIssue(t *testing.T, stdout string) map[string]any {
	t.Helper()
	issues := decodeShowIssues(t, stdout)
	require.Len(t, issues, 1, "single-issue show must have count 1")
	return issues[0]
}

func TestShowOmitsTombstonedNotes(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--id", "note-task", "--title", "Note task", "--type", "task")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "note", "--issue", "note-task", "--msg", "visible note")
	require.NoError(t, err)

	out2, err := runTrls(t, repo, "note", "--issue", "note-task", "--msg", "deleted note")
	require.NoError(t, err)
	var noteResult map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out2)), &noteResult))
	deletedID, _ := noteResult["note_id"].(string) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	require.NotEmpty(t, deletedID)

	_, err = runTrls(t, repo, "note", "delete", "--issue", "note-task", "--note-id", deletedID)
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "note-task")
	require.NoError(t, err)
	showResult := decodeShowIssue(t, out)
	notes, _ := showResult["notes"].([]any) //nolint:errcheck // panic on failed type assertion is acceptable in tests
	assert.Len(t, notes, 1, "deleted note should be hidden from show output")
	if len(notes) > 0 {
		assert.Equal(t, "visible note", notes[0])
	}
}

// TestShow_BlockedBy verifies that arm show displays blocked_by and blocks lists
// when they are non-empty, in both human-readable and JSON formats.
func TestShow_BlockedBy(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
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
		out, err := runTrls(t, repo, "show", "--format", "human", "blk-2")
		require.NoError(t, err)
		assert.Contains(t, out, "BlockedBy:", "blk-2 should show BlockedBy field")
		assert.Contains(t, out, "blk-1", "blk-2 should list blk-1 as its blocker")
	})

	t.Run("human-readable shows Blocks", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "--format", "human", "blk-1")
		require.NoError(t, err)
		assert.Contains(t, out, "Blocks:", "blk-1 should show Blocks field")
		assert.Contains(t, out, "blk-2", "blk-1 should list blk-2 as what it blocks")
	})

	t.Run("JSON output includes blocked_by", func(t *testing.T) {
		out, err := runTrls(t, repo, "show", "--format", "json", "blk-2")
		require.NoError(t, err)
		result := decodeShowIssue(t, out)
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
		result := decodeShowIssue(t, out)
		blocks, ok := result["blocks"]
		require.True(t, ok, "JSON output should contain blocks key")
		blocksList, ok := blocks.([]any)
		require.True(t, ok, "blocks should be an array")
		assert.Len(t, blocksList, 1)
		assert.Equal(t, "blk-2", blocksList[0])
	})

	t.Run("omits BlockedBy when empty", func(t *testing.T) {
		// blk-1 is not blocked by anything
		out, err := runTrls(t, repo, "show", "--format", "human", "blk-1")
		require.NoError(t, err)
		assert.NotContains(t, out, "BlockedBy:", "blk-1 has no blockers and should not show BlockedBy")
	})

	t.Run("omits Blocks when empty", func(t *testing.T) {
		// blk-2 does not block anything
		out, err := runTrls(t, repo, "show", "--format", "human", "blk-2")
		require.NoError(t, err)
		assert.NotContains(t, out, "Blocks:", "blk-2 blocks nothing and should not show Blocks")
	})
}

// TestShow_BlockedBy_MultiJSON verifies that the multi-issue JSON array path
// also includes blocked_by and blocks fields.
func TestShow_BlockedBy_MultiJSON(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
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

	results := decodeShowIssues(t, out)
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

// TestShow_JSON_IncludesPriorityField verifies that the priority field is present in
// JSON output when set. This is a non-regression test: the move from the old inline
// showJSON struct to output.IssueJSON added the priority field to the JSON schema.
func TestShow_JSON_IncludesPriorityField(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Create a task with an explicit priority
	_, err = runTrls(t, repo, "create", "--id", "pri-task", "--title", "Priority task", "--type", "task", "--priority", "high")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "pri-task")
	require.NoError(t, err)

	result := decodeShowIssue(t, out)
	priority, ok := result["priority"]
	require.True(t, ok, "JSON output from arm show must include the priority field")
	assert.Equal(t, "high", priority, "priority field must reflect the value set at create time")
}

func TestShowDisplaysRunningSpend_REQ_TOPTIER_S11_T2(t *testing.T) {
	repo := initCostFixture(t)

	out, err := runTrls(t, repo, "show", "--format", "human", "STORY-COST")
	require.NoError(t, err)
	assert.Contains(t, out, "Spend-to-date:")
	assert.Contains(t, out, "$")
	assert.Regexp(t, `Spend-to-date: \$[0-9]+\.[0-9]+ \([0-9]+ in / [0-9]+ out\)`, out)
	// Story rollup includes descendant TASK-COST-A (1M in @ $3) and TASK-COST-B (haiku).
	assert.NotContains(t, out, "$0.000000", "story spend-to-date must include recorded descendant tokens")

	fieldOut, err := runTrls(t, repo, "show", "--field", "status", "STORY-COST")
	require.NoError(t, err)
	assert.NotContains(t, fieldOut, "Spend-to-date:", "--field must stay a scalar extractor")

	jsonOut, err := runTrls(t, repo, "show", "--format", "json", "STORY-COST")
	require.NoError(t, err)
	result := decodeShowIssue(t, jsonOut)
	assert.Equal(t, "STORY-COST", result["id"])
}

func TestShowSkipsSpendLoadForJSONAndField(t *testing.T) {
	repo := initCostFixture(t)
	ctx := getTestContext(t, repo)
	require.NoError(t, os.WriteFile(filepath.Join(ctx.IssuesDir, stats.DefaultRatesFile), []byte("{"), 0o600))

	_, stderr, err := runTrlsWithStderr(t, repo, "show", "--format", "json", "STORY-COST")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "spend-to-date unavailable", "JSON show must not parse ops for spend")

	_, stderr, err = runTrlsWithStderr(t, repo, "show", "--format", "agent", "STORY-COST")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "spend-to-date unavailable", "agent show must not parse ops for spend")

	_, stderr, err = runTrlsWithStderr(t, repo, "show", "--field", "status", "STORY-COST")
	require.NoError(t, err)
	assert.NotContains(t, stderr, "spend-to-date unavailable", "--field must not parse ops for spend")

	_, stderr, err = runTrlsWithStderr(t, repo, "show", "--format", "human", "STORY-COST")
	require.NoError(t, err)
	assert.Contains(t, stderr, "spend-to-date unavailable", "human show still loads spend and surfaces rate-table errors")
}

func TestShowHonoursStructuredFormat_REQ_AOC_S2_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	jsonOut, err := runTrls(t, repo, "show", "--format", "json", "task-01")
	require.NoError(t, err)
	agentOut, err := runTrls(t, repo, "show", "--format", "agent", "task-01")
	require.NoError(t, err)

	jsonEnv := decodeShowEnvelope(t, jsonOut)
	agentEnv := decodeShowEnvelope(t, agentOut)
	assert.Equal(t, jsonEnv["count"], agentEnv["count"])
	assert.Equal(t, jsonEnv["issues"], agentEnv["issues"])
	assert.Equal(t, jsonEnv["help"], agentEnv["help"])

	issue := decodeShowIssue(t, jsonOut)
	assert.Equal(t, "task-01", issue["id"])
	assert.Equal(t, "task", issue["type"])
	assert.Equal(t, "open", issue["status"])
	assert.NotEmpty(t, issue["title"])

	humanOut, err := runTrls(t, repo, "show", "--format", "human", "task-01")
	require.NoError(t, err)
	assert.Contains(t, humanOut, "ID:")
	assert.False(t, json.Valid([]byte(strings.TrimSpace(humanOut))), "human show stays RenderIssue prose")

	_, err = runTrls(t, repo, "show", "--format", "json", "does-not-exist")
	require.Error(t, err, "a missing issue is an error, not an empty envelope")
}

func TestShowTruncatesOutcomeWithSizeHint_REQ_AOC_S2_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	longOutcome := strings.Repeat("completed with a detailed outcome body. ", 40)
	require.Greater(t, len(longOutcome), showLargeFieldLimit)

	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", longOutcome, "--force")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "task-01")
	require.NoError(t, err)
	issue := decodeShowIssue(t, out)
	got, ok := issue["outcome"].(string)
	require.True(t, ok)
	assert.Less(t, len(got), len(longOutcome), "large outcome must be truncated by default")
	assert.Equal(t, longOutcome[:len(got)], got)

	decoded := decodeShowEnvelope(t, out)
	require.Contains(t, decoded, "truncated")
	var hints []showTruncation
	require.NoError(t, json.Unmarshal(decoded["truncated"], &hints))
	require.NotEmpty(t, hints)
	found := false
	for _, h := range hints {
		if h.Field == "outcome" {
			found = true
			assert.Equal(t, len(got), h.ShownBytes)
			assert.Equal(t, len(longOutcome), h.TotalBytes)
		}
	}
	assert.True(t, found, "truncated adjunct must name outcome")

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], strconv.Itoa(len(longOutcome)))
	assert.Contains(t, help[0], "--full")
}

func TestShowFullFlagReturnsCompleteField_REQ_AOC_S2_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-01", "--worktree")
	require.NoError(t, err)

	longOutcome := strings.Repeat("completed with a detailed outcome body. ", 40)
	require.Greater(t, len(longOutcome), showLargeFieldLimit)

	_, err = runTrls(t, repo, "transition", "task-01", "--to", "done",
		"--skip-delivery-gate", "--outcome", longOutcome, "--force")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "--full", "task-01")
	require.NoError(t, err)
	issue := decodeShowIssue(t, out)
	assert.Equal(t, longOutcome, issue["outcome"])

	decoded := decodeShowEnvelope(t, out)
	_, hasTrunc := decoded["truncated"]
	assert.False(t, hasTrunc, "--full must not attach a truncated adjunct")

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.NotContains(t, strings.Join(help, "\n"), "--full", "--full is offered only when truncation is used")
}

func TestCoordinatorShowRecoveryReadsEnvelope_REQ_AOC_S2_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "note", "--issue", "task-01", "--msg", "unrelated note")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "note", "--issue", "task-01", "--msg", "a.2 cycle 2/3 remediator returned yellow")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "show", "--format", "json", "task-01")
	require.NoError(t, err)
	decoded := decodeShowEnvelope(t, out)
	_, hasTopNotes := decoded["notes"]
	assert.False(t, hasTopNotes, "cycle notes live on the issue row, not the envelope")

	issue := decodeShowIssue(t, out)
	notes, ok := issue["notes"].([]any)
	require.True(t, ok)
	cycleRe := regexp.MustCompile(`a\.2 cycle (\d+)/3`)
	maxCycle := 0
	for _, n := range notes {
		msg, _ := n.(string)
		m := cycleRe.FindStringSubmatch(msg)
		if len(m) != 2 {
			continue
		}
		nCycle, convErr := strconv.Atoi(m[1])
		require.NoError(t, convErr)
		if nCycle > maxCycle {
			maxCycle = nCycle
		}
	}
	assert.Equal(t, 2, maxCycle, "remedia cycle must be recovered from .issues[0].notes")

	skillPath := filepath.Join("..", "..", "internal", "skillsembed", "skills", "armature-coordinator", "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	body := string(skill)
	assert.Contains(t, body, ".issues[0].notes")
	assert.NotContains(t, body, "[.notes // []")
}

func TestShowFieldExtractionStaysScalar_REQ_AOC_S2_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, err := runTrls(t, repo, "show", "--format", "json", "--field", "status", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "open\n", out)
	assert.NotContains(t, out, "{")
	assert.NotContains(t, out, "issues")

	agentOut, err := runTrls(t, repo, "show", "--format", "agent", "--field", "id,title", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "task-01\nTest task\n", agentOut)

	humanOut, err := runTrls(t, repo, "show", "--field", "status,title", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "open\nTest task\n", humanOut)
}
