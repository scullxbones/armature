package gittest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitWithOriginCreatesWorkingRepoAndBareOrigin(t *testing.T) {
	t.Parallel()
	fx := InitWithOrigin(t)

	head, err := os.ReadFile(filepath.Join(fx.Dir, ".git", "HEAD"))
	require.NoError(t, err)
	assert.Contains(t, string(head), "refs/heads/main")
	assert.DirExists(t, fx.Origin)

	origin := GitOutput(t, fx.Dir, "remote", "get-url", "origin")
	assert.Equal(t, fx.Origin, origin)

	bare := GitOutput(t, fx.Origin, "rev-parse", "--is-bare-repository")
	assert.Equal(t, "true", bare)
	originHead := GitOutput(t, fx.Origin, "symbolic-ref", "HEAD")
	assert.Equal(t, "refs/heads/main", originHead)

	assert.Equal(t, "0", GitOutput(t, fx.Origin, "config", "--local", "gc.auto"))
	assert.Equal(t, "false", GitOutput(t, fx.Origin, "config", "--local", "gc.autoDetach"))
	assert.Equal(t, "false", GitOutput(t, fx.Origin, "config", "--local", "receive.autogc"))
	assert.Equal(t, "false", GitOutput(t, fx.Dir, "config", "--local", "commit.gpgsign"))
}

func TestInitRepoHasNoOriginRemote(t *testing.T) {
	t.Parallel()
	repo := InitRepo(t)
	_, err := gitCombined(repo, "remote", "get-url", "origin")
	require.Error(t, err)
}

func TestInitRepoIgnoresInheritedOverrideEnv(t *testing.T) {
	other := InitRepo(t)
	marker := filepath.Join(other, "KEEP")
	require.NoError(t, os.WriteFile(marker, []byte("untouched"), 0o600))
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)

	repo := InitRepo(t)
	_, err := os.Stat(filepath.Join(repo, ".git"))
	require.NoError(t, err)
	keep, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(keep))
	if _, err := os.Stat(filepath.Join(other, ".git")); err != nil {
		t.Fatalf("victim repo must remain a repository: %v", err)
	}
}

func TestInitRepoDoesNotCreateInitialCommit(t *testing.T) {
	t.Parallel()
	repo := InitRepo(t)
	_, err := gitCombined(repo, "rev-parse", "--verify", "HEAD")
	require.Error(t, err)
}

func GitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitCombined(dir, args...)
	require.NoError(t, err, strings.TrimSpace(out))
	return strings.TrimSpace(out)
}
