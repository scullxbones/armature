package materialize

// DeriveBranchName determines the worktree branch name for an issue based on
// its type. Returns an empty string for types that do not receive a
// worktree (e.g., epic).
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
		return ""
	}
}
