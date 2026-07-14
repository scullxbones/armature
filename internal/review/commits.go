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
func ReviewCommits(git *adapters.Client, issueID string) ([]adapters.LogEntry, error) {
	// Get all commits on the current branch
	entries, err := git.LogBranch("HEAD", 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	// Filter commits by the conventional-commit pattern
	// Pattern: ^[a-z]+\(ISSUE-ID\):
	// This matches any lowercase type (feat, fix, refactor, etc.) followed by
	// the issue ID in parentheses and a colon.
	pattern := regexp.MustCompile(`^[a-z]+\(` + regexp.QuoteMeta(issueID) + `\):`)

	var results []adapters.LogEntry
	for _, entry := range entries {
		// Match against the first line of the subject (in case of multiline messages)
		firstLine := entry.Subject
		if idx := len(firstLine); idx > 0 {
			// Find the newline if present
			for i := 0; i < len(firstLine); i++ {
				if firstLine[i] == '\n' {
					firstLine = firstLine[:i]
					break
				}
			}
		}

		if pattern.MatchString(firstLine) {
			results = append(results, entry)
		}
	}

	// Don't reverse; return in git log order (newest first)
	return results, nil
}
