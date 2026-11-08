// Package externalref parses github.com issue URLs into GitHubIssue values.
package externalref

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidGitHubIssueURL = errors.New("invalid GitHub issue URL")

type GitHubIssue struct {
	canonical string
}

func (g GitHubIssue) URL() string { return g.canonical }

func ParseGitHubIssueURL(raw string) (GitHubIssue, error) {
	canonical, ok := canonicalGitHubIssueURL(raw)
	if !ok {
		return GitHubIssue{}, ErrInvalidGitHubIssueURL
	}
	return GitHubIssue{canonical: canonical}, nil
}

func canonicalGitHubIssueURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return "", false
	}
	if u.User != nil {
		return "", false
	}
	if !strings.EqualFold(u.Hostname(), "github.com") || u.Port() != "" {
		return "", false
	}
	path := u.Path
	if strings.Contains(path, "/pull/") {
		return "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[0] != "" || parts[3] != "issues" {
		return "", false
	}
	owner := strings.ToLower(parts[1])
	repo := strings.ToLower(parts[2])
	if !validGitHubOwner(owner) || !validGitHubRepo(repo) {
		return "", false
	}
	n, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil || n == 0 {
		return "", false
	}
	return "https://github.com/" + owner + "/" + repo + "/issues/" + strconv.FormatUint(n, 10), true
}

func validGitHubOwner(s string) bool {
	if len(s) < 1 || len(s) > 39 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validGitHubRepo(s string) bool {
	if s == "." || s == ".." || len(s) < 1 || len(s) > 100 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}
