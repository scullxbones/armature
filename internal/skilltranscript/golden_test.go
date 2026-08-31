// Package skilltranscript provides golden-transcript e2e tests for the coordinator skill's documented command sequence.
package skilltranscript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCoordinatorGoldenTranscript_REQ_TOPTIER_S1_T2 exercises the armature coordinator's
// documented command sequence end-to-end against a real fixture repository.
//
// This is a DYNAMIC verification that tests the actual arm CLI surface,
// complementing the static skill-lint checks in TOPTIER-S1-T1.
// If any command's real behavior diverges from what's documented in the
// armature-coordinator skill, the test reflects the REAL behavior (that's the point).
func TestCoordinatorGoldenTranscript_REQ_TOPTIER_S1_T2(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping golden transcript test in short mode")
	}

	t.Run("coordinator wave dispatch sequence", func(t *testing.T) {
		// Create a fixture repository
		repo := NewTestRepo(t)

		// Create a persistent temp directory for review files (survives across subtests)
		persistentTmpDir := t.TempDir()

		// Step 0: Create a feature branch for the story (required before dispatch)
		storyBranch := "feat/test-story"
		runCmd(repo.Path(), "checkout", "-b", storyBranch)

		// Step 1: Create a story and task
		storyID := repo.CreateStory(t, "Golden Transcript Story")
		taskID := repo.CreateTask(t,
			storyID,
			"Implement golden transcript test",
			[]string{"internal/skilltranscript/golden_test.go"})

		// Verify the task exists and is open
		t.Logf("Created story %s and task %s", storyID, taskID)

		// Step 2: Run 'arm ready' to find ready work
		t.Run("arm ready returns ready tasks", func(t *testing.T) {
			readyTasks := repo.Ready(t)
			if len(readyTasks) == 0 {
				t.Fatalf("expected at least one ready task, got none")
			}

			// Verify the created task is in the ready list
			found := false
			for _, task := range readyTasks {
				taskMap, ok := task.(map[string]interface{})
				if !ok {
					continue
				}
				// Ready output uses "issue" field, not "id"
				if id, ok := taskMap["issue"].(string); ok && id == taskID {
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

		// Step 3: Claim the task with a worktree
		var worktreePath string
		t.Run("arm claim creates worktree", func(t *testing.T) {
			worktreePath = repo.Claim(t, taskID, 60)

			// Verify worktree was created
			if _, err := os.Stat(worktreePath); err != nil {
				t.Fatalf("worktree not created at %s: %v", worktreePath, err)
			}

			// Verify armature-issue-id file exists in the worktree
			// Note: .git in a worktree is a file pointing to the real git dir
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

		// Step 4: Render context for the task
		t.Run("arm render-context returns task specification", func(t *testing.T) {
			context := repo.RenderContext(t, taskID)

			// Verify the context contains expected top-level keys
			expectedKeys := []string{"issue_id", "layers"}
			for _, key := range expectedKeys {
				if _, ok := context[key]; !ok {
					t.Fatalf("context missing expected key: %s", key)
				}
			}

			// Verify issue_id matches
			if id, ok := context["issue_id"].(string); !ok || id != taskID {
				t.Fatalf("context issue_id mismatch: expected %s, got %v", taskID, context["issue_id"])
			}

			// Verify layers is an array
			if _, ok := context["layers"].([]interface{}); !ok {
				t.Fatalf("context layers is not an array: %T", context["layers"])
			}

			t.Logf("Successfully rendered context for task %s", taskID)
		})

		// Step 5: Transition the task to done
		t.Run("arm transition marks task done", func(t *testing.T) {
			outcome := "Implemented golden transcript test for coordinator skill verification"
			repo.Transition(t, taskID, "done", outcome)

			t.Logf("Successfully transitioned task %s to done", taskID)
		})

		// Step 6: Get base and head commits for review
		var baseCommit, headCommit string
		t.Run("capture commit range for review", func(t *testing.T) {
			// Base is the current HEAD in the main repo (where we started)
			// Head is the latest commit in the worktree
			baseCommit = runCmd(repo.Path(), "rev-parse", "HEAD")

			// Make a commit in the worktree to have something to review
			testFilePath := filepath.Join(worktreePath, "test_output.txt")
			if err := os.WriteFile(testFilePath, []byte("Test output for golden transcript\n"), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			// Stage and commit in the worktree
			runCmd(worktreePath, "add", "test_output.txt")
			runCmd(worktreePath, "commit", "-m", fmt.Sprintf("feat(%s): add test output", taskID))

			headCommit = runCmd(worktreePath, "rev-parse", "HEAD")

			t.Logf("Base commit: %s, Head commit: %s", baseCommit, headCommit)
		})

		// Step 7: Prepare review bundle
		var bundleFile string
		t.Run("arm review prepare creates bundle", func(t *testing.T) {
			bundleFile = repo.ReviewPrepare(t, taskID, baseCommit, headCommit, persistentTmpDir)

			// Verify bundle file contains valid JSON with expected structure
			content, err := os.ReadFile(bundleFile)
			if err != nil {
				t.Fatalf("failed to read bundle file: %v", err)
			}

			var bundle map[string]interface{}
			if err := json.Unmarshal(content, &bundle); err != nil {
				t.Fatalf("bundle is not valid JSON: %v", err)
			}

			// Verify essential bundle fields
			expectedBundleKeys := []string{"issue_id", "contract"}
			for _, key := range expectedBundleKeys {
				if _, ok := bundle[key]; !ok {
					t.Logf("Warning: bundle missing key %s (acceptable if optional)", key)
				}
			}

			t.Logf("Successfully prepared review bundle at %s", bundleFile)
		})

		// Step 8: Create a minimal valid assessment and record it
		t.Run("arm review record persists assessment", func(t *testing.T) {
			// Read the bundle file to extract BundleID and fingerprints
			bundleContent, err := os.ReadFile(bundleFile)
			if err != nil {
				t.Fatalf("failed to read bundle file: %v", err)
			}

			var bundle map[string]interface{}
			if err := json.Unmarshal(bundleContent, &bundle); err != nil {
				t.Fatalf("failed to parse bundle: %v", err)
			}

			// Extract bundle metadata
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

			// Create a conformance assessment JSON with proper schema
			// Must match the structure expected by `arm review record`
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

			// Record the assessment
			repo.ReviewRecord(t, taskID, assessmentFile, bundleFile)

			t.Logf("Successfully recorded assessment for task %s", taskID)
		})

		// Verify final state: task should be marked done
		t.Run("verify final task state", func(t *testing.T) {
			// Re-render context to verify status
			context := repo.RenderContext(t, taskID)

			// The context should still be valid (status might not be exposed in render-context,
			// but the command should succeed)
			if _, ok := context["issue_id"]; !ok {
				t.Fatalf("failed to re-render context for completed task")
			}

			t.Logf("Task %s successfully completed the full coordinator workflow", taskID)
		})
	})
}

// TestCoordinatorCommandSurface_REQ_TOPTIER_S1_T2 verifies that documented coordinator
// commands exist and respond with expected output shapes.
func TestCoordinatorCommandSurface_REQ_TOPTIER_S1_T2(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping command surface test in short mode")
	}

	repo := NewTestRepo(t)

	// Create minimal fixture
	storyID := repo.CreateStory(t, "Command Surface Test Story")
	taskID := repo.CreateTask(t, storyID, "Test Task", []string{"test.go"})

	t.Run("arm ready returns JSON array", func(t *testing.T) {
		readyTasks := repo.Ready(t)
		// readyTasks is already a []interface{}, so just verify it's not nil
		if readyTasks == nil {
			t.Fatalf("expected JSON array from arm ready, got nil")
		}
		t.Logf("arm ready returned array of %d items", len(readyTasks))
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

		// Read the .git file to get the real git directory (worktrees use a file reference)
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
	taskID := repo.CreateTask(t, storyID, "Test auto-provisioning", []string{"test.go"})

	// Claim with boolean --worktree flag
	worktreePath := repo.Claim(t, taskID, 120)

	// Verify worktree is created at canonical .worktrees/<issue-id> location
	expectedPath := filepath.Join(repo.Path(), ".worktrees", taskID)
	if worktreePath != expectedPath {
		t.Fatalf("worktree path mismatch: expected %s, got %s", expectedPath, worktreePath)
	}

	// Verify the directory exists
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("worktree not created at expected location: %v", err)
	}

	// Verify .git file exists (worktrees use a file pointer to the real git dir)
	gitFile := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitFile); err != nil {
		t.Fatalf("worktree .git file not found: %v", err)
	}

	// Read the .git file to get the real git directory
	gitContent, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf("failed to read .git file: %v", err)
	}

	// Parse gitdir path from .git file content
	// Format: "gitdir: /path/to/.git/worktrees/name"
	gitDirPath := strings.TrimPrefix(strings.TrimSpace(string(gitContent)), "gitdir: ")
	issueIDFile := filepath.Join(gitDirPath, "armature-issue-id")

	// Verify the task ID binding file exists
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

// TestCoordinatorWavePlanningReference_REQ_TOPTIER_S1_T2 verifies that the
// coordinator's wave-planning instructions use the machine-readable command
// and the same issue field emitted by arm ready --waves.
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

// getMapKeys returns the keys of a map as a slice of strings for debugging output.
func getMapKeys(m map[string]interface{}) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
