package externalref

import (
	"errors"
	"testing"
)

func TestParseGitHubIssueURL_REQ_HOSTREF_S1_T3(t *testing.T) {
	t.Parallel()

	const canonical = "https://github.com/scullxbones/armature/issues/12"
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "happy path", raw: "https://github.com/scullxbones/armature/issues/12", want: canonical, ok: true},
		{name: "minimum issue number", raw: "https://github.com/scullxbones/armature/issues/1", want: "https://github.com/scullxbones/armature/issues/1", ok: true},
		{name: "issue number ten", raw: "https://github.com/scullxbones/armature/issues/10", want: "https://github.com/scullxbones/armature/issues/10", ok: true},
		{
			name: "issue number nineteen",
			raw:  "https://github.com/scullxbones/armature/issues/19",
			want: "https://github.com/scullxbones/armature/issues/19",
			ok:   true,
		},
		{name: "owner and repo case folding", raw: "https://github.com/ScullxBones/Armature/issues/12", want: canonical, ok: true},
		{name: "host case folding", raw: "https://GitHub.com/scullxbones/armature/issues/12", want: canonical, ok: true},
		{name: "query dropped", raw: "https://github.com/scullxbones/armature/issues/12?utm=1", want: canonical, ok: true},
		{name: "fragment dropped", raw: "https://github.com/scullxbones/armature/issues/12#discussion", want: canonical, ok: true},
		{name: "query and fragment dropped", raw: "https://github.com/ScullxBones/Armature/issues/12?x=1#note", want: canonical, ok: true},
		{
			name: "repo name containing pull",
			raw:  "https://github.com/scullxbones/my-pull/issues/12",
			want: "https://github.com/scullxbones/my-pull/issues/12",
			ok:   true,
		},
		{name: "trailing slash", raw: "https://github.com/scullxbones/armature/issues/12/"},
		{name: "extra path segment", raw: "https://github.com/scullxbones/armature/issues/12/comments"},
		{name: "pull path", raw: "https://github.com/scullxbones/armature/pull/12"},
		{name: "pull owner", raw: "https://github.com/pull/armature/issues/12"},
		{name: "pull repo", raw: "https://github.com/scullxbones/pull/issues/12"},
		{name: "pull repo case", raw: "https://github.com/scullxbones/Pull/issues/12"},
		{name: "issues segment case", raw: "https://github.com/scullxbones/armature/Issues/12"},
		{name: "leading zeros", raw: "https://github.com/scullxbones/armature/issues/01"},
		{name: "zero", raw: "https://github.com/scullxbones/armature/issues/0"},
		{name: "non digit", raw: "https://github.com/scullxbones/armature/issues/12a"},
		{name: "signed number", raw: "https://github.com/scullxbones/armature/issues/+1"},
		{name: "empty owner", raw: "https://github.com//armature/issues/12"},
		{name: "empty repo", raw: "https://github.com/scullxbones//issues/12"},
		{name: "missing number", raw: "https://github.com/scullxbones/armature/issues"},
		{name: "empty number", raw: "https://github.com/scullxbones/armature/issues/"},
		{name: "http", raw: "http://github.com/scullxbones/armature/issues/12"},
		{name: "www host", raw: "https://www.github.com/scullxbones/armature/issues/12"},
		{name: "api host", raw: "https://api.github.com/scullxbones/armature/issues/12"},
		{name: "enterprise host", raw: "https://github.example.com/scullxbones/armature/issues/12"},
		{name: "missing scheme", raw: "github.com/scullxbones/armature/issues/12"},
		{name: "scheme relative", raw: "//github.com/scullxbones/armature/issues/12"},
		{name: "userinfo", raw: "https://alice@github.com/scullxbones/armature/issues/12"},
		{name: "port", raw: "https://github.com:443/scullxbones/armature/issues/12"},
		{name: "invalid escape", raw: "https://github.com/%zz/armature/issues/12"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.ok {
				requireIssue(t, tc.raw, tc.want)
				return
			}
			err := requireNotIssue(t, tc.raw)
			if err.Error() != "not a github.com issue URL" {
				t.Fatalf("ParseGitHubIssueURL(%q) error string = %q", tc.raw, err.Error())
			}
		})
	}

	t.Run("equivalent urls", func(t *testing.T) {
		t.Parallel()
		folded := requireIssue(t, "https://github.com/ScullxBones/Armature/issues/12?x=1", canonical)
		plain := requireIssue(t, "https://github.com/scullxbones/armature/issues/12#note", canonical)
		if folded.URL() != plain.URL() {
			t.Fatalf("URL() = %q and %q, want one canonical value", folded.URL(), plain.URL())
		}
	})
}

func TestParseLinearURLErrors_REQ_HOSTREF_S1_T3(t *testing.T) {
	t.Parallel()

	const linear = "https://linear.app/team/issue/ABC-123"
	const malformed = "https://github.com/scullxbones/armature/issues/01"
	linearErr := requireNotIssue(t, linear)
	malformedErr := requireNotIssue(t, malformed)
	if linearErr.Error() != malformedErr.Error() || linearErr.Error() != "not a github.com issue URL" {
		t.Fatalf("error strings linear %q github %q", linearErr.Error(), malformedErr.Error())
	}
}

func requireIssue(t *testing.T, raw, want string) GitHubIssue {
	t.Helper()
	got, err := ParseGitHubIssueURL(raw)
	if err != nil {
		t.Fatalf("ParseGitHubIssueURL(%q) err = %v", raw, err)
	}
	if got.URL() != want {
		t.Fatalf("ParseGitHubIssueURL(%q) URL = %q, want %q", raw, got.URL(), want)
	}
	again, err := ParseGitHubIssueURL(got.URL())
	if err != nil {
		t.Fatalf("ParseGitHubIssueURL(%q) reparse err = %v", got.URL(), err)
	}
	if again.URL() != want {
		t.Fatalf("reparse URL = %q, want %q", again.URL(), want)
	}
	return got
}

func requireNotIssue(t *testing.T, raw string) error {
	t.Helper()
	got, err := ParseGitHubIssueURL(raw)
	var target NotGitHubIssueError
	if !errors.As(err, &target) {
		t.Fatalf("ParseGitHubIssueURL(%q) err = %v, want NotGitHubIssueError", raw, err)
	}
	if got.URL() != "" {
		t.Fatalf("ParseGitHubIssueURL(%q) URL = %q, want empty", raw, got.URL())
	}
	return err
}
