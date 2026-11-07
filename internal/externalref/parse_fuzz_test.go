package externalref

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func FuzzParseGitHubIssueURL_REQ_HOSTREF_S1_T3(f *testing.F) {
	for _, seed := range fuzzParseGitHubIssueURLSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got, err := ParseGitHubIssueURL(raw)
		if err != nil {
			if !errors.Is(err, ErrInvalidGitHubIssueURL) || got.URL() != "" {
				t.Fatalf("reject must be ErrInvalidGitHubIssueURL with empty URL, err=%v url=%q", err, got.URL())
			}
			return
		}
		owner, repo, n, ok := canonicalIssueParts(got.URL())
		if !ok {
			t.Fatalf("accepted URL failed shape: %q", got.URL())
		}
		again, err := ParseGitHubIssueURL(got.URL())
		if err != nil {
			t.Fatalf("canonical failed to reparse: %v url=%q", err, got.URL())
		}
		owner2, repo2, n2, ok := canonicalIssueParts(again.URL())
		if !ok || owner != owner2 || repo != repo2 || n != n2 || again.URL() != got.URL() {
			t.Fatalf("round-trip mismatch first=%q second=%q", got.URL(), again.URL())
		}
	})
}

func fuzzParseGitHubIssueURLSeeds() []string {
	return []string{
		"https://github.com/octocat/hello-world/issues/1",
		"https://github.com/octocat/repo%3Fsecret/issues/1",
		"https://github.com/octo%3Fcat/hello-world/issues/1",
		"https://github.com/octocat/repo%23secret/issues/1",
		"https://github.com/octo%23cat/hello-world/issues/1",
		"https://github.com/octocat/repo%00secret/issues/1",
		"https://github.com/octocat/repo%0asecret/issues/1",
		"https://github.com/octocat/repo%0dsecret/issues/1",
		"https://github.com/octocat/repo%09secret/issues/1",
		"https://github.com/./hello-world/issues/1",
		"https://github.com/../hello-world/issues/1",
		"https://github.com/octocat/./issues/1",
		"https://github.com/octocat/../issues/1",
		"https://github.com/%2e/hello-world/issues/1",
		"https://github.com/octocat/%2e%2e/issues/1",
		"https://linear.app/team/issue/ENG-123",
		"",
	}
}

func canonicalIssueParts(canonical string) (owner, repo, n string, ok bool) {
	u, err := url.Parse(canonical)
	if err != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", "", "", false
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.Port() != "" {
		return "", "", "", false
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 5 || parts[0] != "" || parts[3] != "issues" {
		return "", "", "", false
	}
	if parts[1] == "" || parts[2] == "" || parts[4] == "" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[4], true
}
