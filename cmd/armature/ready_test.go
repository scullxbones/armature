package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runReadyJSON(t *testing.T, repo string, extra ...string) (string, string) {
	t.Helper()
	args := append([]string{"ready", "--format", "json"}, extra...)
	out, errOut, err := runTrlsWithStderr(t, repo, args...)
	require.NoError(t, err)
	return out, errOut
}

func decodeReadyEnvelope(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	raw := strings.TrimSpace(stdout)
	require.True(t, json.Valid([]byte(raw)), "stdout must be one JSON value, got %q", stdout)
	require.True(t, strings.HasPrefix(raw, "{"), "envelope must be an object, not a bare array or null")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, "issues")
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")
	return decoded
}

// TestFilterExpiredClaimsByAssignedWorker_UsesAssignedWorkerNotClaimedBy proves
// that an expired claim on an issue assigned to worker-a but claimed by
// worker-b shows up under --assigned-to worker-a (not worker-b), matching the
// AssignedWorker-based filtering ready.FilterByAssignedTo uses for the main
// ready list.
func TestFilterExpiredClaimsByAssignedWorker_UsesAssignedWorkerNotClaimedBy(t *testing.T) {
	t.Parallel()

	expiredClaims := []ready.ExpiredClaimEntry{
		{Issue: "task-01", ClaimedBy: "worker-b"},
		{Issue: "task-02", ClaimedBy: "worker-a"},
	}
	issues := map[string]*materialize.Issue{
		"task-01": {AssignedWorker: "worker-a", ClaimedBy: "worker-b"},
		"task-02": {AssignedWorker: "worker-c", ClaimedBy: "worker-a"},
	}

	got := filterExpiredClaimsByAssignedWorker(expiredClaims, issues, "worker-a")

	if len(got) != 1 || got[0].Issue != "task-01" {
		t.Fatalf("expected only task-01 (assigned to worker-a, regardless of claimant), got %+v", got)
	}
}

func TestFilterExpiredClaimsByAssignedWorker_MissingIssueExcluded(t *testing.T) {
	t.Parallel()

	expiredClaims := []ready.ExpiredClaimEntry{
		{Issue: "task-missing", ClaimedBy: "worker-a"},
	}
	issues := map[string]*materialize.Issue{}

	got := filterExpiredClaimsByAssignedWorker(expiredClaims, issues, "worker-a")

	if len(got) != 0 {
		t.Fatalf("expected no entries for an issue with no materialized state, got %+v", got)
	}
}

func TestReadyEmitsEnvelopeOnStdout_REQ_AOC_S2_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	out, errOut := runReadyJSON(t, repo)
	decoded := decodeReadyEnvelope(t, out)

	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	assert.Equal(t, len(issues), count)
	require.NotEmpty(t, issues)

	found := false
	for _, row := range issues {
		assert.NotEmpty(t, row.ID)
		assert.NotEmpty(t, row.Type)
		assert.NotEmpty(t, row.Status)
		assert.NotEmpty(t, row.Title)
		if row.ID == "task-01" {
			found = true
			assert.Equal(t, "task", row.Type)
			assert.Equal(t, ops.StatusOpen, row.Status)
		}
	}
	assert.True(t, found, "verified task-01 must appear in issues")

	var rawIssues []map[string]any
	require.NoError(t, json.Unmarshal(decoded["issues"], &rawIssues))
	for _, row := range rawIssues {
		_, hasOutcome := row["outcome"]
		assert.False(t, hasOutcome, "ready row must not carry outcome")
	}

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Equal(t, readyClaimHelp, help[0])
	foundShow := false
	for _, h := range help {
		if strings.Contains(h, "arm show") {
			foundShow = true
		}
	}
	assert.True(t, foundShow, "ready help must point at arm show")
	assert.NotContains(t, decoded, "waves")

	trimmedErr := strings.TrimSpace(errOut)
	if trimmedErr != "" {
		assert.False(t, strings.HasPrefix(trimmedErr, "["), "structured stderr must not be a results array")
	}

	wavesOut, _ := runReadyJSON(t, repo, "--waves")
	wavesDecoded := decodeReadyEnvelope(t, wavesOut)
	require.Contains(t, wavesDecoded, "waves")
	var waveIDs [][]string
	require.NoError(t, json.Unmarshal(wavesDecoded["waves"], &waveIDs))
	require.NotEmpty(t, waveIDs)
	var waveCount int
	require.NoError(t, json.Unmarshal(wavesDecoded["count"], &waveCount))
	var waveIssues []readyIssueRow
	require.NoError(t, json.Unmarshal(wavesDecoded["issues"], &waveIssues))
	assert.Equal(t, len(waveIssues), waveCount)
	flat := 0
	for _, wave := range waveIDs {
		flat += len(wave)
	}
	assert.Equal(t, waveCount, flat, "waves adjunct names ids; rows stay in issues")

	var waveHelp []string
	require.NoError(t, json.Unmarshal(wavesDecoded["help"], &waveHelp))
	require.NotEmpty(t, waveHelp)
	assert.Equal(t, readyWavesHelp, waveHelp[0])
}

func TestReadyExpiredClaimsInEnvelopeNotStderr_REQ_AOC_S2_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	opsDir := filepath.Join(repo, ".armature", "ops")
	logPath := filepath.Join(opsDir, "expired-worker.log")
	staleClaimTime := time.Now().Unix() - 7200
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type: ops.OpClaim, TargetID: "task-01", Timestamp: staleClaimTime,
		WorkerID: "expired-worker", Payload: ops.Payload{TTL: 1},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	out, errOut := runReadyJSON(t, repo)
	decoded := decodeReadyEnvelope(t, out)

	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	for _, row := range issues {
		assert.NotEqual(t, "task-01", row.ID, "claimed+expired issue must not appear in issues")
	}

	require.Contains(t, decoded, "expired_claims")
	var expired []map[string]any
	require.NoError(t, json.Unmarshal(decoded["expired_claims"], &expired))
	require.Len(t, expired, 1)
	assert.Equal(t, "task-01", expired[0]["id"])
	assert.Equal(t, "expired-worker", expired[0]["claimed_by"])
	assert.Equal(t, ops.StatusClaimed, expired[0]["status"])

	trimmedErr := strings.TrimSpace(errOut)
	assert.False(t, strings.HasPrefix(trimmedErr, "["), "expired claims must not be a stderr JSON array, got %q", errOut)
	if trimmedErr != "" {
		var stderrArr []map[string]any
		err := json.Unmarshal([]byte(trimmedErr), &stderrArr)
		assert.Error(t, err, "structured stderr must not decode as expired-claims JSON")
	}

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], "expired_claims")
	assert.NotContains(t, help[0], "blockers")
	assert.Contains(t, help, readyExpiredHelp)
}

func TestReadyEmptyStateIsDefinitive_REQ_AOC_S2_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, errOut := runReadyJSON(t, repo)
	raw := strings.TrimSpace(out)
	assert.NotEqual(t, "[]", raw)
	assert.NotEqual(t, "null", raw)

	decoded := decodeReadyEnvelope(t, out)
	var count int
	require.NoError(t, json.Unmarshal(decoded["count"], &count))
	assert.Equal(t, 0, count)
	assert.Equal(t, "[]", strings.TrimSpace(string(decoded["issues"])))
	var issues []readyIssueRow
	require.NoError(t, json.Unmarshal(decoded["issues"], &issues))
	assert.Empty(t, issues)

	var help []string
	require.NoError(t, json.Unmarshal(decoded["help"], &help))
	require.NotEmpty(t, help)
	assert.Contains(t, help[0], "ready")
	foundShow := false
	for _, h := range help {
		if strings.Contains(h, "arm show") {
			foundShow = true
		}
	}
	assert.True(t, foundShow, "empty ready help must still point at arm show")

	trimmedErr := strings.TrimSpace(errOut)
	if trimmedErr != "" {
		assert.False(t, strings.HasPrefix(trimmedErr, "["), "empty ready must not dump a stderr array")
	}

	wavesOut, _ := runReadyJSON(t, repo, "--waves")
	wavesDecoded := decodeReadyEnvelope(t, wavesOut)
	require.NoError(t, json.Unmarshal(wavesDecoded["count"], &count))
	assert.Equal(t, 0, count)
	require.Contains(t, wavesDecoded, "waves")
	var waveIDs [][]string
	require.NoError(t, json.Unmarshal(wavesDecoded["waves"], &waveIDs))
	assert.Empty(t, waveIDs)
}

func TestReadyHelpEmptyReasonNamesFilterAndExpired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		n          int
		waves      bool
		expiredN   int
		parent     string
		assignedTo string
		wantFirst  string
		wantAll    []string
	}{
		{
			name:      "unfiltered empty blames blockers",
			wantFirst: readyEmptyHelp,
			wantAll:   []string{readyEmptyHelp, readyShowHelp},
		},
		{
			name:      "parent filter names --parent",
			parent:    "STORY-01",
			wantFirst: "no issues match --parent STORY-01",
			wantAll:   []string{"no issues match --parent STORY-01", readyShowHelp},
		},
		{
			name:       "assigned-to filter names --assigned-to",
			assignedTo: "worker-a",
			wantFirst:  "no issues match --assigned-to worker-a",
			wantAll:    []string{"no issues match --assigned-to worker-a", readyShowHelp},
		},
		{
			name:       "both filters name both flags",
			parent:     "STORY-01",
			assignedTo: "worker-a",
			wantFirst:  "no issues match --parent STORY-01 and --assigned-to worker-a",
			wantAll:    []string{"no issues match --parent STORY-01 and --assigned-to worker-a", readyShowHelp},
		},
		{
			name:      "expired-only empty names expired_claims not blockers",
			expiredN:  1,
			wantFirst: readyEmptyExpiredHelp,
			wantAll:   []string{readyEmptyExpiredHelp, readyShowHelp, readyExpiredHelp},
		},
		{
			name:      "parent filter plus expired names filter and keeps expired adjunct help",
			parent:    "STORY-01",
			expiredN:  1,
			wantFirst: "no issues match --parent STORY-01",
			wantAll:   []string{"no issues match --parent STORY-01", readyShowHelp, readyExpiredHelp},
		},
		{
			name:      "non-empty ignores filters",
			n:         2,
			parent:    "STORY-01",
			wantFirst: readyClaimHelp,
			wantAll:   []string{readyClaimHelp, readyShowHelp},
		},
		{
			name:      "waves non-empty keeps wave help",
			n:         2,
			waves:     true,
			wantFirst: readyWavesHelp,
			wantAll:   []string{readyWavesHelp, readyShowHelp},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := readyHelp(tc.n, tc.waves, tc.expiredN, tc.parent, tc.assignedTo)
			assert.Equal(t, tc.wantFirst, got[0])
			assert.Equal(t, tc.wantAll, got)
			if tc.n == 0 && (tc.parent != "" || tc.assignedTo != "" || tc.expiredN > 0) {
				assert.NotContains(t, got[0], "blockers", "empty help must not blame blockers when a filter or expired claim is the context")
			}
		})
	}
}

func TestReadyEmptyHelpNamesActiveFilter_REQ_AOC_S2_T1(t *testing.T) {
	repo := setupRepoWithTask(t)

	parentOut, _ := runReadyJSON(t, repo, "--parent", "nonexistent-parent")
	parentDecoded := decodeReadyEnvelope(t, parentOut)
	var count int
	require.NoError(t, json.Unmarshal(parentDecoded["count"], &count))
	assert.Equal(t, 0, count)
	var parentHelp []string
	require.NoError(t, json.Unmarshal(parentDecoded["help"], &parentHelp))
	require.NotEmpty(t, parentHelp)
	assert.Contains(t, parentHelp[0], "--parent")
	assert.Contains(t, parentHelp[0], "nonexistent-parent")
	assert.NotContains(t, parentHelp[0], "blockers")

	assignedOut, _ := runReadyJSON(t, repo, "--assigned-to", "worker-nobody")
	assignedDecoded := decodeReadyEnvelope(t, assignedOut)
	require.NoError(t, json.Unmarshal(assignedDecoded["count"], &count))
	assert.Equal(t, 0, count)
	var assignedHelp []string
	require.NoError(t, json.Unmarshal(assignedDecoded["help"], &assignedHelp))
	require.NotEmpty(t, assignedHelp)
	assert.Contains(t, assignedHelp[0], "--assigned-to")
	assert.Contains(t, assignedHelp[0], "worker-nobody")
	assert.NotContains(t, assignedHelp[0], "blockers")

	bothOut, _ := runReadyJSON(t, repo, "--parent", "nonexistent-parent", "--assigned-to", "worker-nobody")
	bothDecoded := decodeReadyEnvelope(t, bothOut)
	var bothHelp []string
	require.NoError(t, json.Unmarshal(bothDecoded["help"], &bothHelp))
	require.NotEmpty(t, bothHelp)
	assert.Contains(t, bothHelp[0], "--parent")
	assert.Contains(t, bothHelp[0], "--assigned-to")
	assert.NotContains(t, bothHelp[0], "blockers")
}
