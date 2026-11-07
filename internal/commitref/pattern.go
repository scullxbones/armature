// Package commitref decides whether a commit subject counts for an issue.
// Callers use IsValidReference.
// scripts/check-commit-subjects.sh duplicates subjectHasIssueScope for any-id
// subjects; keep those two rules aligned.
package commitref

import (
	"regexp"
	"strings"
)

func commitTypes() []string {
	return []string{"feat", "fix", "refactor", "test", "docs", "style", "polish", "ci"}
}

var (
	squashPRSuffix = regexp.MustCompile(` \(#\d+\)$`)
	anyTypedScope  = regexp.MustCompile(`^(` + typeAlternation() + `)\([^)]+\)!?:[ \t]+\S`)
	anyMergeScope  = regexp.MustCompile(`^merge:[ \t]+\S+[ \t]+\S`)
)

func typeAlternation() string {
	types := commitTypes()
	quoted := make([]string, len(types))
	for i, typ := range types {
		quoted[i] = regexp.QuoteMeta(typ)
	}
	return strings.Join(quoted, "|")
}

func typedCommitPattern(issueID string) *regexp.Regexp {
	return regexp.MustCompile(
		`^(` + typeAlternation() + `)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`,
	)
}

func mergeCommitPattern(issueID string) *regexp.Regexp {
	return regexp.MustCompile(`^merge:[ \t]+` + regexp.QuoteMeta(issueID) + `[ \t]+\S`)
}

func scopeMatches(subject string, parentCount int, typed, merge *regexp.Regexp) bool {
	if typed.MatchString(subject) {
		return true
	}
	return parentCount >= 2 && merge.MatchString(subject)
}

// IsValidReference reports whether subject is a conventional commit or a
// two-parent merge commit for issueID. Issue-id metacharacters are literal.
func IsValidReference(subject string, parentCount int, issueID string) bool {
	return scopeMatches(subject, parentCount, typedCommitPattern(issueID), mergeCommitPattern(issueID))
}

func subjectHasIssueScope(subject string, parentCount int) bool {
	subject = squashPRSuffix.ReplaceAllString(subject, "")
	return scopeMatches(subject, parentCount, anyTypedScope, anyMergeScope)
}
