package skilltranscript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoordinatorGoldenTranscript_REQ_TOPTIER_S1_T2(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping golden transcript test in short mode")
	}

	t.Run("coordinator wave dispatch sequence", func(t *testing.T) {
		repo := NewTestRepo(t)

		persistentTmpDir := t.TempDir()

		storyBranch := "feat/test-story"
		runCmd(repo.Path(), "checkout", "-b", storyBranch)

		storyID := repo.CreateStory(t, "Golden Transcript Story")
		taskID := repo.HarnessCreateVerifiedTask(t,
			storyID,
			"Implement golden transcript test",
			[]string{"internal/skilltranscript/golden_test.go"})

		t.Logf("Created story %s and task %s", storyID, taskID)

		t.Run("arm ready returns ready tasks", func(t *testing.T) {
			readyTasks := repo.Ready(t)
			if len(readyTasks) == 0 {
				t.Fatalf("expected at least one ready task, got none")
			}

			found := false
			for _, task := range readyTasks {
				taskMap, ok := task.(map[string]interface{})
				if !ok {
					continue
				}
				if id, ok := taskMap["id"].(string); ok && id == taskID {
					found = true
					break
				}
			}

			if !found {
				t.Logf("ready output: %v", readyTasks)
				t.Fatalf("task %s not found in ready list", taskID)
			}

			t.Logf("Successfully found ready task %s", taskID)
		})

		var worktreePath string
		t.Run("arm claim creates worktree", func(t *testing.T) {
			worktreePath = repo.Claim(t, taskID, 60)

			if _, err := os.Stat(worktreePath); err != nil {
				t.Fatalf("worktree not created at %s: %v", worktreePath, err)
			}

			gitFile := filepath.Join(worktreePath, ".git")
			gitContent, err := os.ReadFile(gitFile)
			if err != nil {
				t.Fatalf("failed to read .git file: %v", err)
			}

			// Parse gitdir path from .git file content
			// Format: "gitdir: /path/to/.git/worktrees/name"
			gitDirPath := strings.TrimPrefix(strings.TrimSpace(string(gitContent)), "gitdir: ")
			issueIDFile := filepath.Join(gitDirPath, "armature-issue-id")

			// #nosec G703 -- path derived from worktree .git file created by the test
			content, err := os.ReadFile(issueIDFile)
			if err != nil {
				t.Fatalf("failed to read armature-issue-id: %v", err)
			}

			if string(content) != taskID {
				t.Fatalf("armature-issue-id mismatch: expected %s, got %s", taskID, string(content))
			}

			t.Logf("Successfully claimed task %s with worktree at %s", taskID, worktreePath)
		})

		t.Run("arm render-context returns task specification", func(t *testing.T) {
			context := repo.RenderContext(t, taskID)

			expectedKeys := []string{"issue_id", "layers"}
			for _, key := range expectedKeys {
				if _, ok := context[key]; !ok {
					t.Fatalf("context missing expected key: %s", key)
				}
			}

			if id, ok := context["issue_id"].(string); !ok || id != taskID {
				t.Fatalf("context issue_id mismatch: expected %s, got %v", taskID, context["issue_id"])
			}

			if _, ok := context["layers"].([]interface{}); !ok {
				t.Fatalf("context layers is not an array: %T", context["layers"])
			}

			t.Logf("Successfully rendered context for task %s", taskID)
		})

		t.Run("arm transition marks task done", func(t *testing.T) {
			outcome := "Implemented golden transcript test for coordinator skill verification"
			repo.HarnessDriveTransition(t, taskID, "done", outcome)

			t.Logf("Successfully transitioned task %s to done", taskID)
		})

		var baseCommit, headCommit string
		t.Run("capture commit range for review", func(t *testing.T) {
			baseCommit = runCmd(repo.Path(), "rev-parse", "HEAD")

			testFilePath := filepath.Join(worktreePath, "test_output.txt")
			if err := os.WriteFile(testFilePath, []byte("Test output for golden transcript\n"), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			runCmd(worktreePath, "add", "test_output.txt")
			runCmd(worktreePath, "commit", "-m", fmt.Sprintf("feat(%s): add test output", taskID))

			headCommit = runCmd(worktreePath, "rev-parse", "HEAD")

			t.Logf("Base commit: %s, Head commit: %s", baseCommit, headCommit)
		})

		var bundleFile string
		t.Run("arm review prepare creates bundle", func(t *testing.T) {
			bundleFile = repo.ReviewPrepare(t, taskID, baseCommit, headCommit, persistentTmpDir)

			content, err := os.ReadFile(bundleFile)
			if err != nil {
				t.Fatalf("failed to read bundle file: %v", err)
			}

			var bundle map[string]interface{}
			if err := json.Unmarshal(content, &bundle); err != nil {
				t.Fatalf("bundle is not valid JSON: %v", err)
			}

			expectedBundleKeys := []string{"issue_id", "contract"}
			for _, key := range expectedBundleKeys {
				if _, ok := bundle[key]; !ok {
					t.Logf("Warning: bundle missing key %s (acceptable if optional)", key)
				}
			}

			t.Logf("Successfully prepared review bundle at %s", bundleFile)
		})

		t.Run("arm review record persists assessment", func(t *testing.T) {
			bundleContent, err := os.ReadFile(bundleFile)
			if err != nil {
				t.Fatalf("failed to read bundle file: %v", err)
			}

			var bundle map[string]interface{}
			if err := json.Unmarshal(bundleContent, &bundle); err != nil {
				t.Fatalf("failed to parse bundle: %v", err)
			}

			bundleID := bundle["bundle_id"]
			var contractFingerprint, deliveryFingerprint string
			if fingerprints, ok := bundle["fingerprints"].(map[string]interface{}); ok {
				if cf, ok := fingerprints["contract"].(string); ok {
					contractFingerprint = cf
				}
				if df, ok := fingerprints["delivery"].(string); ok {
					deliveryFingerprint = df
				}
			}

			assessment := map[string]interface{}{
				"schema_version":       1,
				"bundle_id":            bundleID,
				"contract_fingerprint": contractFingerprint,
				"delivery_fingerprint": deliveryFingerprint,
				"results": []map[string]interface{}{
					{
						"id":        "definition_of_done",
						"status":    "satisfied",
						"rationale": "Golden transcript test validation",
						"citations": []map[string]any{{"path": "test_output.txt", "line": 1}},
					},
					{
						"id":        "acceptance[0]",
						"status":    "satisfied",
						"rationale": "Acceptance criterion covered by golden transcript",
						"citations": []map[string]any{{"path": "test_output.txt", "line": 1}},
					},
				},
			}

			assessmentJSON, err := json.MarshalIndent(assessment, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal assessment: %v", err)
			}

			assessmentFile := filepath.Join(persistentTmpDir, "assessment.json")
			if err := os.WriteFile(assessmentFile, assessmentJSON, 0644); err != nil {
				t.Fatalf("failed to write assessment file: %v", err)
			}

			repo.ReviewRecord(t, taskID, assessmentFile, bundleFile)

			t.Logf("Successfully recorded assessment for task %s", taskID)
		})

		t.Run("verify final task state", func(t *testing.T) {
			context := repo.RenderContext(t, taskID)

			if _, ok := context["issue_id"]; !ok {
				t.Fatalf("failed to re-render context for completed task")
			}

			t.Logf("Task %s successfully completed the full coordinator workflow", taskID)
		})
	})
}

func TestCoordinatorCommandSurface_REQ_TOPTIER_S1_T2(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping command surface test in short mode")
	}

	repo := NewTestRepo(t)

	storyID := repo.CreateStory(t, "Command Surface Test Story")
	taskID := repo.HarnessCreateVerifiedTask(t, storyID, "Test Task", []string{"test.go"})

	t.Run("arm ready returns JSON array", func(t *testing.T) {
		readyTasks := repo.Ready(t)
		if readyTasks == nil {
			t.Fatalf("expected issues payload from arm ready, got nil")
		}
		t.Logf("arm ready returned issues payload of %d items", len(readyTasks))
	})

	t.Run("arm render-context returns JSON object with issue_id and layers", func(t *testing.T) {
		context := repo.RenderContext(t, taskID)

		if _, ok := context["issue_id"]; !ok {
			t.Errorf("issue_id missing from render-context output")
		}
		if _, ok := context["layers"]; !ok {
			t.Errorf("layers missing from render-context output")
		}

		t.Logf("arm render-context returned object with keys: %v", getMapKeys(context))
	})

	t.Run("arm claim with --worktree creates git worktree", func(t *testing.T) {
		worktreePath := repo.Claim(t, taskID, 120)

		gitFile := filepath.Join(worktreePath, ".git")
		if _, err := os.Stat(gitFile); err != nil {
			t.Errorf("worktree .git not found: %v", err)
		}

		gitContent, err := os.ReadFile(gitFile)
		if err != nil {
			t.Errorf("failed to read .git file: %v", err)
		}

		gitDirPath := strings.TrimPrefix(strings.TrimSpace(string(gitContent)), "gitdir: ")
		issueIDFile := filepath.Join(gitDirPath, "armature-issue-id")
		// #nosec G703 -- path derived from worktree .git file created by the test
		content, err := os.ReadFile(issueIDFile)
		if err != nil {
			t.Errorf("armature-issue-id file not found: %v", err)
		} else if string(content) != taskID {
			t.Errorf("armature-issue-id mismatch: expected %s, got %s", taskID, string(content))
		}

		t.Logf("arm claim created worktree with valid armature-issue-id binding")
	})
}

// TestE2EClaimAutoProvisionsWorktree_REQ_LNGHZN_S5_T5 verifies that the boolean
// --worktree flag auto-provisions a worktree at the canonical .worktrees/<issue-id>
// location (per ADR 0004).
func TestE2EClaimAutoProvisionsWorktree_REQ_LNGHZN_S5_T5(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repo := NewTestRepo(t)
	storyID := repo.CreateStory(t, "Worktree Auto-Provisioning Test Story")
	taskID := repo.HarnessCreateVerifiedTask(t, storyID, "Test auto-provisioning", []string{"test.go"})

	worktreePath := repo.Claim(t, taskID, 120)

	expectedPath := filepath.Join(repo.Path(), ".worktrees", taskID)
	if worktreePath != expectedPath {
		t.Fatalf("worktree path mismatch: expected %s, got %s", expectedPath, worktreePath)
	}

	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("worktree not created at expected location: %v", err)
	}

	gitFile := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitFile); err != nil {
		t.Fatalf("worktree .git file not found: %v", err)
	}

	gitContent, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf("failed to read .git file: %v", err)
	}

	// Parse gitdir path from .git file content
	// Format: "gitdir: /path/to/.git/worktrees/name"
	gitDirPath := strings.TrimPrefix(strings.TrimSpace(string(gitContent)), "gitdir: ")
	issueIDFile := filepath.Join(gitDirPath, "armature-issue-id")

	// #nosec G703 -- path derived from worktree .git file created by the test
	content, err := os.ReadFile(issueIDFile)
	if err != nil {
		t.Fatalf("failed to read armature-issue-id: %v", err)
	}

	if string(content) != taskID {
		t.Fatalf("armature-issue-id mismatch: expected %s, got %s", taskID, string(content))
	}

	t.Logf("Successfully verified worktree auto-provisioned at canonical location .worktrees/%s with valid binding", taskID)
}

func TestCoordinatorWavePlanningReference_REQ_TOPTIER_S1_T2(t *testing.T) {
	t.Parallel()

	referencePath := filepath.Join("..", "skillsembed", "skills", "armature-coordinator", "references", "parallel-dispatch.md")
	content, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read coordinator wave-planning reference: %v", err)
	}

	reference := string(content)
	if !strings.Contains(reference, "arm ready --waves --format json") {
		t.Error("coordinator reference must request JSON waves output")
	}
	if !strings.Contains(reference, `"issue": "STORY-S1-T1"`) {
		t.Error("coordinator reference must use the waves output issue field")
	}
	if strings.Contains(reference, `"id": "STORY-S1-T1"`) {
		t.Error("coordinator reference must not document a nonexistent waves id field")
	}
}

func getMapKeys(m map[string]interface{}) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
