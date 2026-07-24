package harnesspolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScopeMatrix_BindingStateXToolXPath_REQ_TOPTIER_S5_T1 is a comprehensive
// conformance matrix that verifies every combination of:
//   - Binding state (bound active, bound inactive, unbound)
//   - Tool class (Edit, Bash, etc.)
//   - Path type (in scope, out of scope, outside worktree)
//
// The matrix drives policy decisions: allow, block, or pass-through.
// This is the data-centric test suite for scope policy validation.
func TestScopeMatrix_BindingStateXToolXPath_REQ_TOPTIER_S5_T1(t *testing.T) {
	type testCase struct {
		name        string
		scope       []string
		paths       []string
		expectAllow bool
		expectMsg   string
	}

	// Test cases covering the binding state x tool x path matrix
	tests := []testCase{
		// ========== In-Scope Paths (Tool: Edit, Binding: Bound Active) ==========
		{
			name:        "matrix_bound_edit_in_scope_single_file_allow",
			scope:       []string{"internal/"},
			paths:       []string{"internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_bound_edit_in_scope_multiple_files_allow",
			scope:       []string{"internal/harnesshook/"},
			paths:       []string{"internal/harnesshook/evaluator.go", "internal/harnesshook/binding.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_bound_edit_in_scope_exact_file_match_allow",
			scope:       []string{"internal/foo.go"},
			paths:       []string{"internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},

		// ========== Out-of-Scope Paths (Tool: Edit, Binding: Bound Active) ==========
		{
			name:        "matrix_bound_edit_out_of_scope_single_file_block",
			scope:       []string{"internal/"},
			paths:       []string{"cmd/main.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "matrix_bound_edit_out_of_scope_sibling_dir_block",
			scope:       []string{"internal/"},
			paths:       []string{"pkg/util.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "matrix_bound_edit_out_of_scope_parent_dir_block",
			scope:       []string{"internal/orchestrate/"},
			paths:       []string{"internal/harnesshook/hook.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Outside Worktree Paths (Tool: Edit, Binding: Bound Active) ==========
		{
			name:        "matrix_bound_edit_outside_worktree_traversal_block",
			scope:       []string{"internal/"},
			paths:       []string{"../parent-repo/file.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "matrix_bound_edit_outside_worktree_sibling_repo_block",
			scope:       []string{"internal/"},
			paths:       []string{"../../other-repo/file.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Root Scope (.) - Universal Allow ==========
		{
			name:        "matrix_bound_edit_root_scope_any_file_allow",
			scope:       []string{"."},
			paths:       []string{"any/path/to/file.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_bound_edit_root_scope_top_level_file_allow",
			scope:       []string{"."},
			paths:       []string{"README.md"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},

		// ========== Glob Patterns ==========
		{
			name:        "matrix_bound_edit_glob_single_star_allow",
			scope:       []string{"internal/*.go"},
			paths:       []string{"internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_bound_edit_glob_single_star_nested_block",
			scope:       []string{"internal/*.go"},
			paths:       []string{"internal/nested/foo.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "matrix_bound_edit_glob_double_star_nested_allow",
			scope:       []string{"internal/**/*.go"},
			paths:       []string{"internal/nested/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_bound_edit_glob_double_star_deeply_nested_allow",
			scope:       []string{"internal/**"},
			paths:       []string{"internal/a/b/c/file.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},

		// ========== Empty Scope (Unbound) ==========
		{
			name:        "matrix_unbound_empty_scope_any_path_block",
			scope:       []string{},
			paths:       []string{"internal/foo.go"},
			expectAllow: false,
			expectMsg:   "task has no declared scope",
		},
		{
			name:        "matrix_unbound_empty_scope_top_level_block",
			scope:       []string{},
			paths:       []string{"README.md"},
			expectAllow: false,
			expectMsg:   "task has no declared scope",
		},

		// ========== Multiple Scope Entries (OR logic) ==========
		{
			name:        "matrix_multiple_scopes_path_matches_first_allow",
			scope:       []string{"internal/", "cmd/"},
			paths:       []string{"internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_multiple_scopes_path_matches_second_allow",
			scope:       []string{"internal/", "cmd/"},
			paths:       []string{"cmd/main.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_multiple_scopes_path_matches_none_block",
			scope:       []string{"internal/", "cmd/"},
			paths:       []string{"pkg/util.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Multiple Paths: All Must Match (AND logic per path) ==========
		{
			name:        "matrix_multiple_paths_all_in_scope_allow",
			scope:       []string{"internal/"},
			paths:       []string{"internal/foo.go", "internal/bar.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_multiple_paths_one_out_of_scope_block",
			scope:       []string{"internal/"},
			paths:       []string{"internal/foo.go", "cmd/main.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "matrix_multiple_paths_all_out_of_scope_block",
			scope:       []string{"internal/"},
			paths:       []string{"cmd/main.go", "pkg/util.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Absolute Paths (normalized by root) ==========
		{
			name:        "matrix_absolute_path_in_scope_allow",
			scope:       []string{"internal/"},
			paths:       []string{"/workspace/repo/internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_absolute_path_out_of_scope_block",
			scope:       []string{"internal/"},
			paths:       []string{"/workspace/repo/cmd/main.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Path Cleaning (. and .. normalization) ==========
		{
			name:        "matrix_path_with_dot_segments_cleaned_allow",
			scope:       []string{"internal/"},
			paths:       []string{"./internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_path_with_parent_traversal_cleaned_allow",
			scope:       []string{"internal/"},
			paths:       []string{"internal/subdir/../foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_path_traversal_escape_cleaned_block",
			scope:       []string{"internal/"},
			paths:       []string{"internal/../../cmd/main.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Dogfood Bypass Cases ==========
		{
			name:        "dogfood_case_1_makefile_out_of_scope_block",
			scope:       []string{"internal/harnesshook/"},
			paths:       []string{"Makefile"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "dogfood_case_2_stray_binary_outside_scope_block",
			scope:       []string{"internal/"},
			paths:       []string{"stray_binary"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
		{
			name:        "dogfood_case_3_parent_repo_leak_block",
			scope:       []string{"internal/"},
			paths:       []string{"../main-repo/file.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},

		// ========== Edge Cases ==========
		{
			name:        "matrix_empty_path_in_scope_allow",
			scope:       []string{"internal/"},
			paths:       []string{},
			expectAllow: true,
			expectMsg:   "", // No paths means no violations
		},
		{
			name:        "matrix_dot_slash_prefix_normalized_allow",
			scope:       []string{"internal/"},
			paths:       []string{"./internal/foo.go"},
			expectAllow: true,
			expectMsg:   "within task scope",
		},
		{
			name:        "matrix_case_sensitive_path_matching",
			scope:       []string{"Internal/"},
			paths:       []string{"internal/foo.go"},
			expectAllow: false,
			expectMsg:   "outside task scope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := NewScopePolicyWithRoot(tc.scope, "/workspace/repo")

			result := policy.CheckPaths(tc.paths)

			if tc.expectAllow {
				require.True(t, result.Allowed, "expected allowed for %s", tc.name)
				if tc.expectMsg != "" {
					assert.Contains(t, result.Message(), tc.expectMsg)
				}
			} else {
				require.False(t, result.Allowed, "expected blocked for %s", tc.name)
				if tc.expectMsg != "" {
					assert.Contains(t, result.Message(), tc.expectMsg)
				}
				require.NotEmpty(t, result.Violations, "expected violations for %s", tc.name)
			}
		})
	}
}

// TestScopeMatrix_DoubleStarEdgeCases_REQ_TOPTIER_S5_T1 verifies edge cases
// for ** (doublestar) glob patterns.
func TestScopeMatrix_DoubleStarEdgeCases_REQ_TOPTIER_S5_T1(t *testing.T) {
	type testCase struct {
		name        string
		scope       []string
		paths       []string
		expectAllow bool
	}

	tests := []testCase{
		{
			name:        "doublestar_at_end_matches_any_depth",
			scope:       []string{"src/**"},
			paths:       []string{"src/foo/bar/baz.go"},
			expectAllow: true,
		},
		{
			name:        "doublestar_at_start_matches_any_depth",
			scope:       []string{"**/test.go"},
			paths:       []string{"a/b/c/test.go"},
			expectAllow: true,
		},
		{
			name:        "doublestar_in_middle_matches_skipped_segments",
			scope:       []string{"internal/**/api.go"},
			paths:       []string{"internal/v1/handlers/api.go"},
			expectAllow: true,
		},
		{
			name:        "doublestar_with_suffix_respects_suffix",
			scope:       []string{"src/**/*.go"},
			paths:       []string{"src/deep/nested/path/file.go"},
			expectAllow: true,
		},
		{
			name:        "doublestar_with_suffix_rejects_wrong_extension",
			scope:       []string{"src/**/*.go"},
			paths:       []string{"src/deep/nested/path/file.txt"},
			expectAllow: false,
		},
		{
			name:        "doublestar_zero_segments_allows_immediate_child",
			scope:       []string{"internal/**/api.go"},
			paths:       []string{"internal/api.go"},
			expectAllow: true,
		},
		{
			name:        "doublestar_sibling_prefix_no_match",
			scope:       []string{"internal/**"},
			paths:       []string{"internal-copy/foo.go"},
			expectAllow: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := NewScopePolicyWithRoot(tc.scope, "/workspace")

			result := policy.CheckPaths(tc.paths)

			if tc.expectAllow {
				assert.True(t, result.Allowed, "expected allowed for %s", tc.name)
			} else {
				assert.False(t, result.Allowed, "expected blocked for %s", tc.name)
			}
		})
	}
}

// TestScopeMatrix_ScopeViolationMessages_REQ_TOPTIER_S5_T1 verifies that
// violation messages are clear and include all necessary information.
func TestScopeMatrix_ScopeViolationMessages_REQ_TOPTIER_S5_T1(t *testing.T) {
	type testCase struct {
		name              string
		scope             []string
		paths             []string
		expectedViolation bool
		expectedInMsg     []string
	}

	tests := []testCase{
		{
			name:              "violation_message_includes_path",
			scope:             []string{"internal/"},
			paths:             []string{"cmd/main.go"},
			expectedViolation: true,
			expectedInMsg:     []string{"cmd/main.go", "outside task scope"},
		},
		{
			name:              "violation_message_includes_allowed_scope",
			scope:             []string{"internal/", "cmd/"},
			paths:             []string{"pkg/util.go"},
			expectedViolation: true,
			expectedInMsg:     []string{"pkg/util.go", "allowed scope", "internal/", "cmd/"},
		},
		{
			name:              "violation_message_multiple_violations",
			scope:             []string{"internal/"},
			paths:             []string{"cmd/main.go", "pkg/util.go"},
			expectedViolation: true,
			expectedInMsg:     []string{"cmd/main.go", "pkg/util.go"},
		},
		{
			name:              "empty_scope_violation_message",
			scope:             []string{},
			paths:             []string{"any/file.go"},
			expectedViolation: true,
			expectedInMsg:     []string{"task has no declared scope", "declare scope"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := NewScopePolicyWithRoot(tc.scope, "/workspace")

			result := policy.CheckPaths(tc.paths)

			if tc.expectedViolation {
				assert.False(t, result.Allowed)
				msg := result.Message()
				for _, expected := range tc.expectedInMsg {
					assert.Contains(t, msg, expected, "message should contain: %s", expected)
				}
			} else {
				assert.True(t, result.Allowed)
			}
		})
	}
}

// TestScopeMatrix_AbsoluteVsRelativePaths_REQ_TOPTIER_S5_T1 verifies that
// absolute and relative paths are normalized correctly with a provided root.
func TestScopeMatrix_AbsoluteVsRelativePaths_REQ_TOPTIER_S5_T1(t *testing.T) {
	const (
		repoRoot = "/workspace/armature"
	)

	type testCase struct {
		name        string
		scope       []string
		path        string
		root        string
		expectAllow bool
	}

	tests := []testCase{
		{
			name:        "relative_path_normalized_with_root_allow",
			scope:       []string{"internal/"},
			path:        "internal/foo.go",
			root:        repoRoot,
			expectAllow: true,
		},
		{
			name:        "absolute_path_normalized_with_root_allow",
			scope:       []string{"internal/"},
			path:        repoRoot + "/internal/foo.go",
			root:        repoRoot,
			expectAllow: true,
		},
		{
			name:        "absolute_path_outside_root_block",
			scope:       []string{"internal/"},
			path:        "/workspace/other/internal/foo.go",
			root:        repoRoot,
			expectAllow: false,
		},
		{
			name:        "relative_path_with_dot_slash_normalized",
			scope:       []string{"internal/"},
			path:        "./internal/foo.go",
			root:        repoRoot,
			expectAllow: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy := NewScopePolicyWithRoot(tc.scope, tc.root)

			result := policy.CheckPaths([]string{tc.path})

			if tc.expectAllow {
				assert.True(t, result.Allowed, "expected allowed for %s", tc.name)
			} else {
				assert.False(t, result.Allowed, "expected blocked for %s", tc.name)
			}
		})
	}
}
