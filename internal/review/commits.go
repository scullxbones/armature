package review

import (
	"fmt"
	"regexp"

	"github.com/scullxbones/armature/internal/adapters"
)

// ReviewCommits discovers and returns all delivery commits for an issue
// across all conventional-commit type prefixes (feat, fix, refactor, test,
// docs, chore, etc.).
//
// Commits are matched by their message prefix: `TYPE(ISSUE-ID):` where TYPE
// is any lowercase alphabetic string (e.g., feat, fix, refactor, test, docs, chore).
// This replaces the coordinator skill's feat-only grep pseudocode which silently
// dropped other commit types.
func ReviewCommits(git *adapters.Client, issueID string, branch string) ([]adapters.LogEntry, error) {
	if branch == "" {
		branch = "HEAD"
	}

	// Get all commits on the requested branch (e.g. a task or story branch,
	// not necessarily whatever happens to be checked out).
	entries, err := git.LogBranch(branch, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	// Filter commits by the conventional-commit pattern
	// Pattern: ^(feat|fix|refactor|test|docs|style|polish)\(ISSUE-ID\)!?:
	// Restricted to the commit types enumerated by docs/conventions.md — an
	// unrestricted ^[a-z]+ would also match a bogus type like
	// "oops(ISSUE-ID): ..." (same overly-permissive bug fixed in
	// internal/deliverygate/gate.go's CommitReferenceCheck).
	pattern := regexp.MustCompile(`^(feat|fix|refactor|test|docs|style|polish)\(` + regexp.QuoteMeta(issueID) + `\)!?:`)

	// Initialize as an empty (non-nil) slice so the no-match case marshals to
	// JSON "[]" rather than "null" for agent consumers.
	results := []adapters.LogEntry{}
	for _, entry := range entries {
		// LogBranch's format string produces a single-line subject, so no
		// further newline-splitting is needed here.
		if pattern.MatchString(entry.Subject) {
			results = append(results, entry)
		}
	}

	// Don't reverse; return in git log order (newest first)
	return results, nil
}
