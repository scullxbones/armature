// Package commitref holds the shared conventional-commit / merge-commit
// reference patterns used to decide whether a commit "counts" for an issue.
// It is deliberately dependency-free (only stdlib) so both
// internal/deliverygate (the pass/fail delivery gate) and internal/review
// (discovery for `arm review commits`) can depend on it without an import
// cycle between those two packages.
package commitref

import (
	"regexp"
	"strings"
)

// CommitTypes enumerates the conventional-commit types documented by
// docs/conventions.md.
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

// IsValidReference reports whether subject is a valid commit reference for
// issueID, either as the typed form (TypedCommitPattern) or as the
// merge-commit form (MergeCommitPattern) on a genuine merge commit (2+
// parents, per parentCount). Regex alone can't distinguish a real merge
// commit from an ordinary single-parent commit whose author merely wrote a
// subject that looks like the merge form, so the parent-count requirement is
// mandatory for the merge form.
//
// Takes subject/parentCount rather than adapters.LogEntry so this
// dependency-free package doesn't need to import internal/adapters.
func IsValidReference(subject string, parentCount int, issueID string) bool {
	if TypedCommitPattern(issueID).MatchString(subject) {
		return true
	}
	return parentCount >= 2 && MergeCommitPattern(issueID).MatchString(subject)
}
