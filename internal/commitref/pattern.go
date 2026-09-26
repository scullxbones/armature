// Package commitref holds the shared conventional-commit / merge-commit
// reference patterns used to decide whether a commit "counts" for an issue.
package commitref

import (
	"regexp"
	"strings"
)

// CommitTypes enumerates the conventional-commit types in docs/conventions.md.
var CommitTypes = []string{"feat", "fix", "refactor", "test", "docs", "style", "polish"}

// TypedCommitPattern returns a regex matching the conventional-commit
// reference form `type(ISSUE-ID): description` or `type(ISSUE-ID)!: description`,
// where type is restricted to CommitTypes.
func TypedCommitPattern(issueID string) *regexp.Regexp {
	quoted := make([]string, len(CommitTypes))
	for i, typ := range CommitTypes {
		quoted[i] = regexp.QuoteMeta(typ)
	}
	return regexp.MustCompile(
		`^(` + strings.Join(quoted, "|") + `)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`,
	)
}

// MergeCommitPattern returns a regex matching the documented merge-commit
// reference form `merge: ISSUE-ID description` (see docs/conventions.md).
func MergeCommitPattern(issueID string) *regexp.Regexp {
	return regexp.MustCompile(`^merge:[ \t]+` + regexp.QuoteMeta(issueID) + `[ \t]+\S`)
}

func IsValidReference(subject string, parentCount int, issueID string) bool {
	if TypedCommitPattern(issueID).MatchString(subject) {
		return true
	}
	return parentCount >= 2 && MergeCommitPattern(issueID).MatchString(subject)
}
