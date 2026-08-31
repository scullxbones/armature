package e2eharness_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/e2eharness"
	"github.com/scullxbones/armature/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArtifactPipelineUsesCLI_REQ_TOPTIER_S3_T3 drives the production artifact
// boundaries end to end: strict plan parsing by dag apply, materialized context
// assembly by render-context, and strict assessment/bundle parsing by review record.
func TestArtifactPipelineUsesCLI_REQ_TOPTIER_S3_T3(t *testing.T) {
	t.Parallel()

	h := e2eharness.New(t, buildArmBinary(t))
	out, err := h.RunArm("bootstrap", "--repo", h.WorkDir)
	require.NoError(t, err, "bootstrap failed: %s", out)
	out, err = h.RunArm("worker-init", "--repo", h.WorkDir)
	require.NoError(t, err, "worker-init failed: %s", out)

	planPath := filepath.Join(h.TempDir, "pipeline-plan.json")
	plan := map[string]any{
		"version": 1,
		"title":   "CLI artifact pipeline",
		"issues": []map[string]any{{
			"id": "PIPE-001", "title": "artifact pipeline", "type": "task",
			"source":     "src-e2e",
			"dod":        "CLI artifacts round-trip under strict decoding",
			"scope":      "pipeline.go",
			"acceptance": []map[string]any{{"type": "test_passes"}},
		}},
	}
	planJSON, err := json.Marshal(plan)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(planPath, planJSON, 0o600))

	out, err = h.RunArm("dag", "apply", "--repo", h.WorkDir, "--plan", planPath)
	require.NoError(t, err, "dag apply failed: %s", out)
	out, err = h.RunArm("dag", "transition", "--repo", h.WorkDir, "--issue", "PIPE-001")
	require.NoError(t, err, "dag transition failed: %s", out)

	out, err = h.RunArm("render-context", "--repo", h.WorkDir, "--issue", "PIPE-001", "--raw")
	require.NoError(t, err, "render-context failed: %s", out)
	assert.Contains(t, out, "PIPE-001")
	assert.Contains(t, out, "CLI artifacts round-trip under strict decoding")

	gitRunInDir(t, h.WorkDir, "commit", "--allow-empty", "-m", "test: pipeline base")
	base := gitRevision(t, h.WorkDir)
	require.NoError(t, os.WriteFile(filepath.Join(h.WorkDir, "pipeline.go"), []byte("package pipeline\n"), 0o600))
	gitRunInDir(t, h.WorkDir, "add", "pipeline.go")
	gitRunInDir(t, h.WorkDir, "commit", "-m", "test: add pipeline delivery")
	head := gitRevision(t, h.WorkDir)

	bundlePath := filepath.Join(h.TempDir, "bundle.json")
	out, err = h.RunArm("review", "prepare", "--repo", h.WorkDir, "--issue", "PIPE-001", "--base", base, "--head", head, "--output", bundlePath)
	require.NoError(t, err, "review prepare failed: %s", out)
	bundleJSON, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	bundle, err := review.DecodeReviewBundle(bundleJSON)
	require.NoError(t, err, "prepared bundle must satisfy strict decoding")
	assert.Equal(t, "PIPE-001", bundle.Issue.ID)

	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results: []review.CriterionResult{{
			ID:        "definition_of_done",
			Status:    review.Satisfied,
			Rationale: "The real CLI artifact boundaries completed successfully.",
			Citations: []review.Citation{{Path: "pipeline.go", Line: 1}},
		}, {
			ID:        "acceptance[0]",
			Status:    review.Satisfied,
			Rationale: "The declared test-passes criterion was met by the pipeline.",
			Citations: []review.Citation{{Path: "pipeline.go", Line: 1}},
		}},
	}
	assessmentPath := filepath.Join(h.TempDir, "assessment.json")
	assessmentJSON, err := json.Marshal(assessment)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(assessmentPath, assessmentJSON, 0o600))

	out, err = h.RunArm("review", "record", "--repo", h.WorkDir, "--issue", "PIPE-001", "--assessment", assessmentPath, "--bundle", bundlePath)
	require.NoError(t, err, "review record failed: %s", out)
	assert.True(t, strings.Contains(out, "recorded"), "record must report the durable assessment: %s", out)
}
