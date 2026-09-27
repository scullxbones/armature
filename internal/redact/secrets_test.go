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
