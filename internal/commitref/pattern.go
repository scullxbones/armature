// Package commitref holds the shared conventional-commit / merge-commit
// reference patterns used to decide whether a commit "counts" for an issue.
// It is deliberately dependency-free (only stdlib) so both
// internal/deliverygate (the pass/fail delivery gate) and internal/review
// (discovery for `arm review commits`) can depend on it without an import
// cycle between those two packages.
package commitref

import "regexp"

// CommitTypes enumerates the conventional-commit types documented by
// docs/conventions.md. Shared by TypedCommitPattern (used by
// internal/deliverygate's CommitReferenceCheck) and
// internal/review.ReviewCommits so the two
// checks — "does this commit satisfy the delivery gate" and "does this
// commit show up in review discovery" — can't independently drift apart on
// which types/forms they recognize (the exact bug class that caused each of
// them to need a separate later fix for the same missing merge-commit
// form).
var CommitTypes = []string{"feat", "fix", "refactor", "test", "docs", "style", "polish"}

// TypedCommitPattern returns a regex matching the conventional-commit
// reference form `type(ISSUE-ID): description` or `type(ISSUE-ID)!: description`,
// where type is restricted to CommitTypes.
func TypedCommitPattern(issueID string) *regexp.Regexp {
	// CommitTypes entries are plain lowercase words with no regex
	// metacharacters, so joining them with "|" for alternation is safe
	// without per-entry quoting.
	return regexp.MustCompile(
		`^(` + joinTypes() + `)\(` + regexp.QuoteMeta(issueID) + `\)!?:[ \t]+\S`,
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
// mandatory for the merge form. This is the single shared decision point for
// "does this commit satisfy issueID" so internal/deliverygate's
// CommitReferenceCheck (pass/fail delivery gate) and internal/review's
// ReviewCommits (discovery) can't independently drift on which forms they
// accept — the exact bug class that let ReviewCommits accept a merge: ID
// subject on an ordinary single-parent commit after the multi-parent guard
// was added only to CommitReferenceCheck's loop.
//
// Takes subject/parentCount rather than adapters.LogEntry so this
// dependency-free package doesn't need to import internal/adapters.
func IsValidReference(subject string, parentCount int, issueID string) bool {
	if TypedCommitPattern(issueID).MatchString(subject) {
		return true
	}
	return parentCount >= 2 && MergeCommitPattern(issueID).MatchString(subject)
}

func joinTypes() string {
	out := ""
	for i, t := range CommitTypes {
		if i > 0 {
			out += "|"
		}
		out += t
	}
	return out
}
