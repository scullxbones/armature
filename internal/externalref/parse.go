// Package externalref parses a github.com issue URL into a canonical value.
// The host ticket stays the approval record. The URL only joins a name to that ticket.
package externalref

import (
	"net/url"
	"strings"
)

const githubHost = "github.com"

type GitHubIssue struct {
	canonical string
}

func (g GitHubIssue) URL() string { return g.canonical }

// NotGitHubIssueError is the only parse error. A Linear URL, an unknown host,
// and a malformed GitHub URL all return it.
type NotGitHubIssueError struct{}

func (NotGitHubIssueError) Error() string {
	return "not a github.com issue URL"
}

func ParseGitHubIssueURL(raw string) (GitHubIssue, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), githubHost) {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	if parsed.User != nil || parsed.Port() != "" {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	// Fold case before the spelling check. The stored path is lowercase, so a
	// repo named Pull must not become a canonical URL that contains /pull/.
	// Issue numbers and pull numbers can collide. This parser does not call the API.
	if strings.Contains(strings.ToLower(parsed.Path), "/pull/") {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(segments) != 4 || segments[2] != "issues" || segments[0] == "" || segments[1] == "" {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	if !canonicalIssueNumber(segments[3]) {
		return GitHubIssue{}, NotGitHubIssueError{}
	}
	owner := strings.ToLower(segments[0])
	repo := strings.ToLower(segments[1])
	canonical := "https://" + githubHost + "/" + owner + "/" + repo + "/issues/" + segments[3]
	return GitHubIssue{canonical: canonical}, nil
}

func canonicalIssueNumber(s string) bool {
	if s == "" || s[0] == '0' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
