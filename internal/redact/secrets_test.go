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
	assert.Contains(t, out, "https://github.com/***")
	assert.NotContains(t, out, "x-access-token:")
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

func TestSecrets_RedactsJWTOutsideURL(t *testing.T) {
	t.Parallel()

	secret := "eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiJ0ZXN0In0." + "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	in := "Authentication failed: bearer " + secret
	out := redact.Secrets(in)

	assert.NotContains(t, out, secret)
	assert.Contains(t, out, "Authentication failed")
	assert.Contains(t, out, "***")
}

func TestSecrets_RedactsURLQueryValues(t *testing.T) {
	t.Parallel()

	secret := "SUPER" + "SECRET"
	cases := []struct {
		name string
		in   string
		host string
	}{
		{name: "sig", in: "fatal: unable to access 'https://host/repo.git?sig=" + secret + "': 403", host: "host"},
		{name: "token", in: "fatal: unable to access 'https://host/repo.git?token=" + secret + "': 403", host: "host"},
		{name: "access_token", in: "fatal: unable to access 'https://host/repo.git?access_token=" + secret + "': 403", host: "host"},
		{
			name: "presigned",
			in: "fatal: unable to access 'https://bucket.s3.amazonaws.com/repo.git" +
				"?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=" + secret +
				"&X-Amz-Signature=" + secret + "': 403",
			host: "bucket.s3.amazonaws.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := redact.Secrets(tc.in)
			assert.NotContains(t, out, secret)
			assert.Contains(t, out, "https://"+tc.host+"/***")
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
			assert.Contains(t, out, "https://host/***")
			assert.Contains(t, out, "403")
		})
	}
}

func TestSecrets_RedactsOpaquePathAndScpRemotes(t *testing.T) {
	t.Parallel()

	opaque := "OPAQUE" + "SECRET123"
	cases := []struct {
		name   string
		in     string
		keep   string
		secret string
	}{
		{
			name:   "opaque-https-path",
			in:     "fatal: unable to access 'https://127.0.0.1:1/signed/" + opaque + "/repo.git': Could not resolve host",
			keep:   "https://127.0.0.1:1/***",
			secret: opaque,
		},
		{
			name:   "scp-style",
			in:     "fatal: Authentication failed\nCould not read from remote 'git@github.com:org/" + opaque + "/repo.git'",
			keep:   "github.com:***",
			secret: opaque,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := redact.Secrets(tc.in)
			assert.NotContains(t, out, tc.secret)
			assert.Contains(t, out, tc.keep)
			if tc.name == "opaque-https-path" {
				assert.Contains(t, out, "Could not resolve host")
			}
			if tc.name == "scp-style" {
				assert.Contains(t, out, "Authentication failed")
			}
		})
	}
}
