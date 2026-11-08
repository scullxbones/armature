package externalref

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubIssueURL_REQ_HOSTREF_S1_T3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "canonical github.com issues URL",
			raw:  "https://github.com/octocat/hello-world/issues/1",
			want: "https://github.com/octocat/hello-world/issues/1",
		},
		{
			name: "lowercase owner and repo",
			raw:  "https://github.com/OctoCat/Hello-World/issues/42",
			want: "https://github.com/octocat/hello-world/issues/42",
		},
		{
			name: "lowercase host and scheme",
			raw:  "HTTPS://GitHub.COM/octocat/hello-world/issues/7",
			want: "https://github.com/octocat/hello-world/issues/7",
		},
		{
			name: "drop query",
			raw:  "https://github.com/octocat/hello-world/issues/3?foo=bar&baz=1",
			want: "https://github.com/octocat/hello-world/issues/3",
		},
		{
			name: "drop fragment",
			raw:  "https://github.com/octocat/hello-world/issues/3#discussion",
			want: "https://github.com/octocat/hello-world/issues/3",
		},
		{
			name: "drop query and fragment",
			raw:  "https://github.com/octocat/hello-world/issues/9?x=1#y",
			want: "https://github.com/octocat/hello-world/issues/9",
		},
		{
			name: "store n without leading zeros",
			raw:  "https://github.com/octocat/hello-world/issues/01",
			want: "https://github.com/octocat/hello-world/issues/1",
		},
		{
			name: "repo with dots underscores and hyphens",
			raw:  "https://github.com/octocat/hello.world_repo-name/issues/12",
			want: "https://github.com/octocat/hello.world_repo-name/issues/12",
		},
		{
			name: "empty query still drops",
			raw:  "https://github.com/octocat/hello-world/issues/4?",
			want: "https://github.com/octocat/hello-world/issues/4",
		},
		{
			name: "hyphenated owner",
			raw:  "https://github.com/cool-org/hello-world/issues/8",
			want: "https://github.com/cool-org/hello-world/issues/8",
		},
		{
			name: "repo named .github",
			raw:  "https://github.com/octocat/.github/issues/5",
			want: "https://github.com/octocat/.github/issues/5",
		},
		{
			name: "owner at 39 characters",
			raw:  "https://github.com/" + strings.Repeat("a", 39) + "/hello-world/issues/6",
			want: "https://github.com/" + strings.Repeat("a", 39) + "/hello-world/issues/6",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseGitHubIssueURL(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.URL())
			assertStableCanonicalIssueURL(t, got)
		})
	}

	t.Run("zero value is not a parsed issue", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", GitHubIssue{}.URL())
	})
}

func TestParseLinearURLErrors_REQ_HOSTREF_S1_T3(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "linear issue URL", raw: "https://linear.app/team/issue/ENG-123"},
		{name: "unknown host gitlab", raw: "https://gitlab.com/octocat/hello-world/issues/1"},
		{name: "github enterprise host", raw: "https://github.example.com/octocat/hello-world/issues/1"},
		{name: "www.github.com", raw: "https://www.github.com/octocat/hello-world/issues/1"},
		{name: "http scheme", raw: "http://github.com/octocat/hello-world/issues/1"},
		{name: "missing scheme", raw: "github.com/octocat/hello-world/issues/1"},
		{name: "trailing slash", raw: "https://github.com/octocat/hello-world/issues/1/"},
		{name: "extra path segment", raw: "https://github.com/octocat/hello-world/issues/1/extra"},
		{name: "pull request path", raw: "https://github.com/octocat/hello-world/pull/1"},
		{name: "pull in owner repo path", raw: "https://github.com/octocat/pull/issues/1"},
		{name: "missing issue number", raw: "https://github.com/octocat/hello-world/issues/"},
		{name: "issues spelled Issues", raw: "https://github.com/octocat/hello-world/Issues/1"},
		{name: "singular issue", raw: "https://github.com/octocat/hello-world/issue/1"},
		{name: "issue number zero", raw: "https://github.com/octocat/hello-world/issues/0"},
		{name: "non numeric issue number", raw: "https://github.com/octocat/hello-world/issues/n"},
		{name: "userinfo", raw: "https://git@github.com/octocat/hello-world/issues/1"},
		{name: "empty owner", raw: "https://github.com//hello-world/issues/1"},
		{name: "empty repo", raw: "https://github.com/octocat//issues/1"},
		{name: "invalid percent escape", raw: "https://github.com/%zz"},
		{name: "explicit port", raw: "https://github.com:443/octocat/hello-world/issues/1"},
		{name: "empty", raw: ""},
		{name: "malformed", raw: "not a url"},
		{name: "encoded question mark in repo", raw: "https://github.com/octocat/repo%3Fsecret/issues/1"},
		{name: "encoded question mark in owner", raw: "https://github.com/octo%3Fcat/hello-world/issues/1"},
		{name: "encoded hash in repo", raw: "https://github.com/octocat/repo%23secret/issues/1"},
		{name: "encoded hash in owner", raw: "https://github.com/octo%23cat/hello-world/issues/1"},
		{name: "encoded null in repo", raw: "https://github.com/octocat/repo%00secret/issues/1"},
		{name: "encoded newline in repo", raw: "https://github.com/octocat/repo%0asecret/issues/1"},
		{name: "encoded carriage return in repo", raw: "https://github.com/octocat/repo%0dsecret/issues/1"},
		{name: "encoded tab in repo", raw: "https://github.com/octocat/repo%09secret/issues/1"},
		{name: "dot owner", raw: "https://github.com/./hello-world/issues/1"},
		{name: "dot dot owner", raw: "https://github.com/../hello-world/issues/1"},
		{name: "dot repo", raw: "https://github.com/octocat/./issues/1"},
		{name: "dot dot repo", raw: "https://github.com/octocat/../issues/1"},
		{name: "encoded dot owner", raw: "https://github.com/%2e/hello-world/issues/1"},
		{name: "encoded dot dot repo", raw: "https://github.com/octocat/%2e%2e/issues/1"},
		{name: "owner ending with hyphen", raw: "https://github.com/octocat-/hello-world/issues/1"},
		{name: "owner starting with hyphen", raw: "https://github.com/-octocat/hello-world/issues/1"},
		{name: "owner longer than 39 characters", raw: "https://github.com/" + strings.Repeat("a", 40) + "/hello-world/issues/1"},
		{name: "repo longer than 100 characters", raw: "https://github.com/octocat/" + strings.Repeat("a", 101) + "/issues/1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseGitHubIssueURL(tc.raw)
			require.Equal(t, ErrInvalidGitHubIssueURL, err)
			assert.Equal(t, "", got.URL())
		})
	}
}

func assertStableCanonicalIssueURL(t *testing.T, got GitHubIssue) {
	t.Helper()
	canonical := got.URL()
	u, err := url.Parse(canonical)
	require.NoError(t, err)
	assert.Equal(t, "https", u.Scheme)
	assert.Equal(t, "github.com", u.Host)
	assert.Empty(t, u.RawQuery)
	assert.Empty(t, u.Fragment)
	assert.Nil(t, u.User)
	parts := strings.Split(u.Path, "/")
	require.Len(t, parts, 5)
	assert.Equal(t, "", parts[0])
	assert.Equal(t, "issues", parts[3])
	again, err := ParseGitHubIssueURL(canonical)
	require.NoError(t, err)
	assert.Equal(t, canonical, again.URL())
}
