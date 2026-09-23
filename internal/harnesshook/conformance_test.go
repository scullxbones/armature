package harnesshook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPolicyResolver struct {
	policy harnesspolicy.IssuePolicy
}

func (m *mockPolicyResolver) Resolve(taskID string) (harnesspolicy.IssuePolicy, error) {
	return m.policy, nil
}

func makeHookPayload(t *testing.T, toolName string, toolInput map[string]any) []byte {
	t.Helper()
	payload := map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       toolName,
		"tool_input":      toolInput,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}

func TestHookConformance_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	t.Run("BindingStates", TestConformanceMatrix_BindingStates_REQ_TOPTIER_S5_T1)
	t.Run("ToolClasses", TestConformanceMatrix_ToolClasses_REQ_TOPTIER_S5_T1)
	t.Run("PathTypes", TestConformanceMatrix_PathTypes_REQ_TOPTIER_S5_T1)
	t.Run("DogfoodBypassCases", TestConformanceMatrix_DogfoodBypassCases_REQ_TOPTIER_S5_T1)
	t.Run("EmptyScope", TestConformanceMatrix_EmptyScope_REQ_TOPTIER_S5_T1)
	t.Run("AbsolutePaths", TestConformanceMatrix_AbsolutePaths_REQ_TOPTIER_S5_T1)
	t.Run("NoPathPolicy", TestConformanceMatrix_NoPathPolicy_REQ_TOPTIER_S5_T1)
}

func TestConformanceMatrix_BindingStates_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		issueID     string
		scope       []string
		filePath    string
		setupRepo   func(root string) error
		expectAllow bool
		expectMsg   string
	}

	tests := []testCase{
		{
			name:    "bound_active_in_scope_file_allowed",
			issueID: "TASK-001",
			scope:   []string{"internal/"},
			setupRepo: func(root string) error {
				gitDir := filepath.Join(root, ".git")
				if err := os.MkdirAll(gitDir, 0o755); err != nil {
					return err
				}
				issueIDFile := filepath.Join(gitDir, "armature-issue-id")
				return os.WriteFile(issueIDFile, []byte("TASK-001"), 0o644)
			},
			filePath:    "internal/foo.go",
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:    "bound_active_out_of_scope_file_denied",
			issueID: "TASK-002",
			scope:   []string{"internal/"},
			setupRepo: func(root string) error {
				gitDir := filepath.Join(root, ".git")
				if err := os.MkdirAll(gitDir, 0o755); err != nil {
					return err
				}
				issueIDFile := filepath.Join(gitDir, "armature-issue-id")
				return os.WriteFile(issueIDFile, []byte("TASK-002"), 0o644)
			},
			filePath:    "cmd/main.go",
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:    "unbound_worktree_no_policy_denied",
			issueID: "",
			scope:   []string{},
			setupRepo: func(root string) error {
				gitDir := filepath.Join(root, ".git")
				return os.MkdirAll(gitDir, 0o755)
			},
			filePath:    "internal/foo.go",
			expectAllow: false,
			expectMsg:   "task has no declared scope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			require.NoError(t, tc.setupRepo(tmpDir))

			resolver := &mockPolicyResolver{
				policy: harnesspolicy.IssuePolicy{
					ID:    tc.issueID,
					Scope: tc.scope,
				},
			}

			hook := NewHook(resolver)

			input := makeHookPayload(t, "Edit", map[string]any{
				"file_path": tc.filePath,
			})

			result, err := hook.Evaluate(context.Background(), EvaluateInput{
				Input:    input,
				Binding:  tc.issueID,
				Platform: "claude",
				Root:     tmpDir,
			})

			require.NoError(t, err)
			if tc.expectAllow {
				assert.Equal(t, DecisionAllow, result.Decision.Action, "expected allow for %s", tc.name)
				assert.Contains(t, result.Decision.Message, tc.expectMsg)
			} else {
				assert.Equal(t, DecisionBlock, result.Decision.Action, "expected block for %s", tc.name)
				assert.Contains(t, result.Decision.Message, tc.expectMsg)
			}
		})
	}
}

func TestConformanceMatrix_ToolClasses_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		tool        string
		command     string
		filePath    string
		scope       []string
		expectAllow bool
		expectMsg   string
	}

	tests := []testCase{
		{
			name:        "edit_tool_in_scope_allowed",
			tool:        "Edit",
			filePath:    "internal/foo.go",
			scope:       []string{"internal/"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "edit_tool_out_of_scope_denied",
			tool:        "Edit",
			filePath:    "cmd/main.go",
			scope:       []string{"internal/"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "bash_git_commit_denied",
			tool:        "Bash",
			command:     "git commit -m 'test'",
			scope:       []string{"internal/"},
			expectAllow: false,
			expectMsg:   "Armature owns commits",
		},
		{
			name:        "bash_non_commit_command_allowed",
			tool:        "Bash",
			command:     "ls -la",
			scope:       []string{"internal/"},
			expectAllow: true,
			expectMsg:   "no path policy applies",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			require.NoError(t, os.MkdirAll(gitDir, 0o755))

			resolver := &mockPolicyResolver{
				policy: harnesspolicy.IssuePolicy{
					ID:    "TASK-001",
					Scope: tc.scope,
				},
			}

			hook := NewHook(resolver)

			toolInput := map[string]any{"command": tc.command}
			if tc.filePath != "" {
				toolInput["file_path"] = tc.filePath
			}

			input := makeHookPayload(t, tc.tool, toolInput)

			result, err := hook.Evaluate(context.Background(), EvaluateInput{
				Input:    input,
				Binding:  "TASK-001",
				Platform: "claude",
				Root:     tmpDir,
			})

			require.NoError(t, err)
			if tc.expectAllow {
				assert.Equal(t, DecisionAllow, result.Decision.Action, "expected allow for %s", tc.name)
			} else {
				assert.Equal(t, DecisionBlock, result.Decision.Action, "expected block for %s", tc.name)
			}
			assert.Contains(t, result.Decision.Message, tc.expectMsg)
		})
	}
}

func TestConformanceMatrix_PathTypes_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		path        string
		scope       []string
		expectAllow bool
		expectMsg   string
	}

	tests := []testCase{
		{
			name:        "in_scope_path_allowed",
			path:        "internal/harnesshook/evaluator.go",
			scope:       []string{"internal/"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "out_of_scope_path_denied",
			path:        "cmd/main.go",
			scope:       []string{"internal/"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "outside_worktree_path_denied",
			path:        "../parent-repo/file.go",
			scope:       []string{"internal/"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "root_scope_allows_everything",
			path:        "any/arbitrary/path.go",
			scope:       []string{"."},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			require.NoError(t, os.MkdirAll(gitDir, 0o755))

			resolver := &mockPolicyResolver{
				policy: harnesspolicy.IssuePolicy{
					ID:    "TASK-001",
					Scope: tc.scope,
				},
			}

			hook := NewHook(resolver)

			input := makeHookPayload(t, "Edit", map[string]any{
				"file_path": tc.path,
			})

			result, err := hook.Evaluate(context.Background(), EvaluateInput{
				Input:    input,
				Binding:  "TASK-001",
				Platform: "claude",
				Root:     tmpDir,
			})

			require.NoError(t, err)
			if tc.expectAllow {
				assert.Equal(t, DecisionAllow, result.Decision.Action, "expected allow for %s", tc.name)
			} else {
				assert.Equal(t, DecisionBlock, result.Decision.Action, "expected block for %s", tc.name)
			}
			assert.Contains(t, result.Decision.Message, tc.expectMsg)
		})
	}
}

func TestConformanceMatrix_DogfoodBypassCases_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name        string
		path        string
		scope       []string
		description string
		expectAllow bool
	}

	tests := []testCase{
		{
			name:        "bypass_case_1_out_of_scope_makefile_edit_denied",
			path:        "Makefile",
			scope:       []string{"internal/harnesshook/"},
			description: "Out-of-scope Makefile edit must be denied to prevent accidental build config changes",
			expectAllow: false,
		},
		{
			name:        "bypass_case_2_stray_binary_outside_scope_denied",
			path:        "stray_binary",
			scope:       []string{"internal/"},
			description: "Stray binary left outside declared scope must be denied to prevent artifact pollution",
			expectAllow: false,
		},
		{
			name:        "bypass_case_3_worktree_changes_leaking_to_main_denied",
			path:        "../main-repo/file.go",
			scope:       []string{"internal/"},
			description: "Worktree changes leaking into the main worktree must be denied to maintain isolation",
			expectAllow: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			require.NoError(t, os.MkdirAll(gitDir, 0o755))

			resolver := &mockPolicyResolver{
				policy: harnesspolicy.IssuePolicy{
					ID:    "TASK-001",
					Scope: tc.scope,
				},
			}

			hook := NewHook(resolver)

			input := makeHookPayload(t, "Edit", map[string]any{
				"file_path": tc.path,
			})

			result, err := hook.Evaluate(context.Background(), EvaluateInput{
				Input:    input,
				Binding:  "TASK-001",
				Platform: "claude",
				Root:     tmpDir,
			})

			require.NoError(t, err)
			if tc.expectAllow {
				assert.Equal(t, DecisionAllow, result.Decision.Action,
					"bypass case: %s - %s", tc.name, tc.description)
			} else {
				assert.Equal(t, DecisionBlock, result.Decision.Action,
					"bypass case: %s - %s", tc.name, tc.description)
				assert.Contains(t, result.Decision.Message, "outside task scope")
			}
		})
	}
}

func TestConformanceMatrix_EmptyScope_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	resolver := &mockPolicyResolver{
		policy: harnesspolicy.IssuePolicy{
			ID:    "TASK-UNSCOPED",
			Scope: []string{},
		},
	}

	hook := NewHook(resolver)

	input := makeHookPayload(t, "Edit", map[string]any{
		"file_path": "internal/foo.go",
	})

	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "TASK-UNSCOPED",
		Platform: "claude",
		Root:     tmpDir,
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, result.Decision.Action)
	assert.Contains(t, result.Decision.Message, "task has no declared scope")
}

func TestConformanceMatrix_AbsolutePaths_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	resolver := &mockPolicyResolver{
		policy: harnesspolicy.IssuePolicy{
			ID:    "TASK-001",
			Scope: []string{"internal/"},
		},
	}

	hook := NewHook(resolver)

	absInScopePath := filepath.Join(tmpDir, "internal", "foo.go")
	input := makeHookPayload(t, "Edit", map[string]any{
		"file_path": absInScopePath,
	})

	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "TASK-001",
		Platform: "claude",
		Root:     tmpDir,
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, result.Decision.Action,
		"absolute path within scope should be allowed")

	absOutOfScopePath := filepath.Join(tmpDir, "cmd", "main.go")
	input = makeHookPayload(t, "Edit", map[string]any{
		"file_path": absOutOfScopePath,
	})

	result, err = hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "TASK-001",
		Platform: "claude",
		Root:     tmpDir,
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionBlock, result.Decision.Action,
		"absolute path outside scope should be blocked")
}

func TestConformanceMatrix_NoPathPolicy_REQ_TOPTIER_S5_T1(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))

	resolver := &mockPolicyResolver{
		policy: harnesspolicy.IssuePolicy{
			ID:    "TASK-001",
			Scope: []string{"internal/"},
		},
	}

	hook := NewHook(resolver)

	input := makeHookPayload(t, "Bash", map[string]any{
		"command": "echo hello",
	})

	result, err := hook.Evaluate(context.Background(), EvaluateInput{
		Input:    input,
		Binding:  "TASK-001",
		Platform: "claude",
		Root:     tmpDir,
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionAllow, result.Decision.Action)
	assert.Contains(t, result.Decision.Message, "no path policy applies")
}
