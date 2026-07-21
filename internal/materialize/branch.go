package materialize

// DeriveBranchName determines the worktree branch name for an issue based on
// its type. Returns an empty string for types that do not receive a
// worktree (e.g., epic). Shared by claim (creating worktrees), merge
// (tearing them down), and doctor (diagnosing missing worktrees), so all
// three agree on the same type -> branch-prefix mapping.
func DeriveBranchName(issueType, issueID string) string {
	switch issueType {
	case "bug":
		return "fix/" + issueID
	case "feature":
		return "feat/" + issueID
	case "story":
		return "feat/" + issueID
	case "task":
		return "task/" + issueID
	default:
		// epic and unknown types do not have worktrees.
		return ""
	}
}
