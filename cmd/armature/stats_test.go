package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/scullxbones/armature/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsCostCLI_REQ_TOPTIER_S11_T2(t *testing.T) {
	repo := initCostFixture(t)

	out, err := runTrls(t, repo, "stats", "--cost", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "STORY-COST")
	assert.Contains(t, out, "wave-")
	assert.Contains(t, out, "$")
	assert.Contains(t, out, "TASK-COST-A")

	jsonOut, err := runTrls(t, repo, "stats", "--cost", "--format", "json")
	require.NoError(t, err)
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &report))
	stories, ok := report["stories"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, stories)
	waves, ok := report["waves"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, waves)
}

func TestStatsCostExcludesMismatchedWorkerOps(t *testing.T) {
	repo := initCostFixture(t)
	ctx := getTestContext(t, repo)
	ghostPath := filepath.Join(ctx.IssuesDir, "ops", "worker-ghost.log")
	require.NoError(t, appendOp(ctx, ghostPath, ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "TASK-COST-A",
		Timestamp: nowEpoch(),
		WorkerID:  "someone-else",
		Payload: ops.Payload{
			To:           ops.StatusDone,
			Outcome:      "tokens from a filename that does not own this worker_id",
			InputTokens:  1_000_000_000,
			OutputTokens: 1_000_000_000,
		},
	}))

	jsonOut, err := runTrls(t, repo, "stats", "--cost", "--format", "json")
	require.NoError(t, err)
	var report struct {
		Stories []struct {
			ID  string  `json:"id"`
			USD float64 `json:"usd"`
		} `json:"stories"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &report))
	require.Len(t, report.Stories, 1)
	assert.InDelta(t, 4.0, report.Stories[0].USD, 1e-9,
		"spend must match materialized ops and ignore filename/worker_id mismatches")
}

func TestStatsCostDedupesIdempotentAssessments(t *testing.T) {
	repo := initCostFixture(t)
	ctx := getTestContext(t, repo)
	ghostPath := filepath.Join(ctx.IssuesDir, "ops", "worker-ghost.log")
	attJSON, err := json.Marshal(review.AssessmentAttestation{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-cost",
		ContractFingerprint: "cf",
		DeliveryFingerprint: "df",
		BaseSHA:             "aa",
		HeadSHA:             "bb",
		ModelIdentity:       "claude-haiku-4-5",
		InputTokens:         500_000,
		OutputTokens:        100_000,
		Rating:              review.Green,
		ResultFingerprint:   "fp-cost-b",
	})
	require.NoError(t, err)
	require.NoError(t, appendOp(ctx, ghostPath, ops.Op{
		Type:      ops.OpAssessmentAttested,
		TargetID:  "TASK-COST-B",
		Timestamp: nowEpoch(),
		WorkerID:  "worker-ghost",
		Payload:   ops.Payload{Assessment: attJSON},
	}))

	jsonOut, err := runTrls(t, repo, "stats", "--cost", "--format", "json")
	require.NoError(t, err)
	var report struct {
		Stories []struct {
			ID           string  `json:"id"`
			USD          float64 `json:"usd"`
			InputTokens  int     `json:"input_tokens"`
			OutputTokens int     `json:"output_tokens"`
		} `json:"stories"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &report))
	require.Len(t, report.Stories, 1)
	assert.InDelta(t, 4.0, report.Stories[0].USD, 1e-9,
		"idempotent assessment attestations must be billed once")
	assert.Equal(t, 1_500_000, report.Stories[0].InputTokens)
	assert.Equal(t, 100_000, report.Stories[0].OutputTokens)
}

func TestStatsWithoutCostFlagHints(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "stats")
	require.NoError(t, err)
	assert.Contains(t, out, "--cost")

	emptyCost, err := runTrls(t, repo, "stats", "--cost", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, emptyCost, "(none)")
}

func initCostFixture(t *testing.T) string {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "create", "--type", "story", "--title", "Cost story", "--id", "STORY-COST")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Cost task A", "--id", "TASK-COST-A",
		"--parent", "STORY-COST", "--scope", "pkg/cost_a.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--title", "Cost task B", "--id", "TASK-COST-B",
		"--parent", "STORY-COST", "--scope", "pkg/cost_b.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	appendTokenOps(t, repo)
	return repo
}

func appendTokenOps(t *testing.T, repo string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, err := worker.GetWorkerID(repo)
	require.NoError(t, err)
	require.NotEmpty(t, workerID)
	logPath := opsLogPath(ctx.IssuesDir, workerID)

	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type:      ops.OpTransition,
		TargetID:  "TASK-COST-A",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			To:           ops.StatusDone,
			Outcome:      "recorded worker tokens for cost view",
			InputTokens:  1_000_000,
			OutputTokens: 0,
		},
	}))

	attJSON, err := json.Marshal(review.AssessmentAttestation{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            "bundle-cost",
		ContractFingerprint: "cf",
		DeliveryFingerprint: "df",
		BaseSHA:             "aa",
		HeadSHA:             "bb",
		ModelIdentity:       "claude-haiku-4-5",
		InputTokens:         500_000,
		OutputTokens:        100_000,
		Rating:              review.Green,
		ResultFingerprint:   "fp-cost-b",
	})
	require.NoError(t, err)
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type:      ops.OpAssessmentAttested,
		TargetID:  "TASK-COST-B",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{Assessment: attJSON},
	}))
}
