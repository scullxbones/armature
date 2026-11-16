package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromotionReadyCLI_FreshPass_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	recordAssessmentForTask01(t, repo, base, tip)
	plantCIGateEvidence(t, repo, tip)

	out, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip)
	require.NoError(t, err)
	assert.Contains(t, out, "promotion-ready: ok")
}

func TestPromotionReadyCLI_EmptyContractFails_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	plantCIGateEvidence(t, repo, tip)
	plantEmptyContractTask(t, repo, "task-empty")

	_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-empty", "--base", base, "--head", tip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty contract")
}

func TestPromotionReadyCLI_UnknownScopeFails_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	plantCIGateEvidence(t, repo, tip)

	_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-nope", "--base", base, "--head", tip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scope")
}

func TestPromotionReadyCLI_MissingCIGateEvidenceFails_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	recordAssessmentForTask01(t, repo, base, tip)

	_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", tip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gate evidence")
}

func TestPromotionReadyCLI_StaleHeadAssessmentFails_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	recordAssessmentForTask01(t, repo, base, tip)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "extra.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "extra.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): push after assessment")
	newTip := gitRevParse(t, repo)
	plantCIGateEvidence(t, repo, newTip)

	_, err := runTrls(t, repo, "promotion-ready", "--issue", "task-01", "--base", base, "--head", newTip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assessment")
}

func TestPromotionReadyCLI_SharedDiffOneAssessmentSeveralLeaves_REQ_480(t *testing.T) {
	t.Parallel()
	repo, base, tip := promotionReadyFixture(t)
	plantVerifiedTask(t, repo, "task-02", "shared.go")
	_, err := runTrls(t, repo, "amend", "--issue", "task-02", "--acceptance", testAcceptance, "--dod", "task-02 done")
	require.NoError(t, err)
	recordAssessmentForTask01(t, repo, base, tip)
	plantCIGateEvidence(t, repo, tip)

	out, err := runTrls(t, repo,
		"promotion-ready",
		"--issue", "task-01",
		"--issue", "task-02",
		"--base", base,
		"--head", tip,
	)
	require.NoError(t, err)
	assert.Contains(t, out, "promotion-ready: ok")
}

func TestGateRunMarksCIUnderGitHubActions_REQ_480(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{"full": {"true"}})
	t.Setenv("GITHUB_ACTIONS", "true")

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.CI, "GITHUB_ACTIONS must mark gate evidence as CI")
}

func promotionReadyFixture(t *testing.T) (repo, base, tip string) {
	t.Helper()
	repo = setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	// Fixture create omits acceptance; R4 requires a non-empty contract.
	_, err = runTrls(t, repo, "amend", "--issue", "task-01", "--acceptance", testAcceptance)
	require.NoError(t, err)

	run(t, repo, "git", "commit", "--allow-empty", "-m", "feat(task-01): base")
	base = gitRevParse(t, repo)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "impl.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "impl.go")
	run(t, repo, "git", "commit", "-m", "feat(task-01): tip")
	tip = gitRevParse(t, repo)
	return repo, base, tip
}

func plantCIGateEvidence(t *testing.T, repo, head string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendGateEvidence(logPath, workerID, ops.GateEvidence{
		Profile: "full",
		Command: []string{"true"},
		HeadSHA: head,
		Start:   1,
		End:     2,
		Exit:    0,
		CI:      true,
	}))
}

func plantEmptyContractTask(t *testing.T, repo, id string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  id,
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload: ops.Payload{
			Title:            id,
			NodeType:         "task",
			Scope:            []string{"impl.go"},
			DefinitionOfDone: "",
			Acceptance:       json.RawMessage(`[]`),
			Confidence:       "verified",
		},
	}))
}

func recordAssessmentForTask01(t *testing.T, repo, base, tip string) {
	t.Helper()
	const issueID = "task-01"
	bundlePath := filepath.Join(repo, issueID+"-bundle.json")
	_, err := runTrls(t, repo, "review", "prepare", "--issue", issueID, "--base", base, "--head", tip, "--output", bundlePath)
	require.NoError(t, err)
	bundleData, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	var bundle review.ReviewBundle
	require.NoError(t, json.Unmarshal(bundleData, &bundle))

	citation := []review.Citation{review.FileCitation("impl.go", 1, 0)}
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{
			{
				ID:        "definition_of_done",
				Status:    review.Satisfied,
				Rationale: "complete",
				Citations: citation,
			},
			{
				ID:        "acceptance[0]",
				Status:    review.Satisfied,
				Rationale: "complete",
				Citations: citation,
			},
		},
	}
	path := filepath.Join(repo, issueID+"-assessment.json")
	raw, err := json.MarshalIndent(&assessment, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	_, err = runTrls(t, repo, "review", "record", "--issue", issueID, "--assessment", path, "--bundle", bundlePath)
	require.NoError(t, err)
}
