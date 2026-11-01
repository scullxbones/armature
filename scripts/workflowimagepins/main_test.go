package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelftestFixtures_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	require.NoError(t, runSelftest(root, ioDiscard{}, ioDiscard{}))
}

func TestPJReprosFail_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	base := filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned")
	for _, name := range []string{
		"cmd-sub.yml",
		"subshell.yml",
		"docker-context.yml",
		"compose-run.yml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := checkYAMLFixture(base, name, true)
			require.NoError(t, err)
			assert.Greater(t, n, 0, name)
		})
	}
	t.Run("dockerfile-from", func(t *testing.T) {
		t.Parallel()
		n, err := checkDirFixture(filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned-dirs/dockerfile-from"), true)
		require.NoError(t, err)
		assert.Greater(t, n, 0)
	})
}

func TestManifestRequired_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "scripts"), 0o755))
	// Empty manifest file present but no entries.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "scripts/container-images.manifest"), []byte("# none\n"), 0o644))
	_, err := checkRepo(tmp)
	require.Error(t, err)
}

func TestWrapperAllowlisted_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, seedFixtureRepo(tmp))
	findings, err := checkRepo(tmp)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
