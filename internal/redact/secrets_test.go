package redact_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/redact"
	"github.com/stretchr/testify/assert"
)

func TestSecrets_RedactsURLUserinfo(t *testing.T) {
	t.Parallel()

	secret := "ghp_fakeSecretTokenForTest1234567890"
	in := "fatal: unable to access 'https://x-access-token:" + secret + "@github.com/org/repo.git/': 403"
	out := redact.Secrets(in)

	assert.NotContains(t, out, secret)
	assert.Contains(t, out, "https://***@github.com/org/repo.git/")
	assert.Contains(t, out, "403")
}

func TestSecrets_LeavesUncredentialedTextUnchanged(t *testing.T) {
	t.Parallel()

	in := "git fetch origin _armature: connection refused"
	assert.Equal(t, in, redact.Secrets(in))
}

func TestSecrets_RedactsGitHubTokenPrefixes(t *testing.T) {
	t.Parallel()

	secret := "github_pat_" + "11AAAAAAA0123456789abcdefghijklmnopqrstuv"
	in := "auth failed for token " + secret
	out := redact.Secrets(in)

	assert.NotContains(t, out, secret)
	assert.Contains(t, out, "github_pat_***")
}

func TestSecrets_RedactsURLQueryValues(t *testing.T) {
	t.Parallel()

	secret := "SUPER" + "SECRET"
	cases := []struct {
		name string
		in   string
	}{
		{name: "sig", in: "fatal: unable to access 'https://host/repo.git?sig=" + secret + "': 403"},
		{name: "token", in: "fatal: unable to access 'https://host/repo.git?token=" + secret + "': 403"},
		{name: "access_token", in: "fatal: unable to access 'https://host/repo.git?access_token=" + secret + "': 403"},
		{
			name: "presigned",
			in: "fatal: unable to access 'https://bucket.s3.amazonaws.com/repo.git" +
				"?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=" + secret +
				"&X-Amz-Signature=" + secret + "': 403",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := redact.Secrets(tc.in)
			assert.NotContains(t, out, secret)
			assert.Contains(t, out, "https://")
			assert.Contains(t, out, "403")
		})
	}
}

func TestSecrets_RedactsCredentialLookingPathSegments(t *testing.T) {
	t.Parallel()

	ghSecret := "ghp_fakeSecretTokenForTest1234567890"
	jwtSecret := "eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiJ0ZXN0In0." + "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{
			name:   "github-token-path",
			in:     "fatal: unable to access 'https://host/" + ghSecret + "/org/repo.git/': 403",
			secret: ghSecret,
		},
		{
			name:   "jwt-path",
			in:     "fatal: unable to access 'https://host/t/" + jwtSecret + "/repo.git/': 403",
			secret: jwtSecret,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := redact.Secrets(tc.in)
			assert.NotContains(t, out, tc.secret)
			assert.Contains(t, out, "https://host")
			assert.Contains(t, out, "403")
		})
	}
}
