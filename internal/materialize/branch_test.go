package materialize

import "testing"

// TestDeriveBranchName covers each issue-type -> branch-prefix mapping,
// including the empty-string fallback for types without a worktree. This
// closes a mutation-coverage gap: the function previously had no dedicated
// test, leaving its string-concatenation mutants unexercised.
func TestDeriveBranchName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		issueType string
		issueID   string
		want      string
	}{
		{"bug", "task-01", "fix/task-01"},
		{"feature", "task-02", "feat/task-02"},
		{"story", "task-03", "feat/task-03"},
		{"task", "task-04", "task/task-04"},
		{"epic", "task-05", ""},
		{"unknown-type", "task-06", ""},
	}
	for _, tc := range cases {
		got := DeriveBranchName(tc.issueType, tc.issueID)
		if got != tc.want {
			t.Errorf("DeriveBranchName(%q, %q) = %q, want %q", tc.issueType, tc.issueID, got, tc.want)
		}
	}
}
