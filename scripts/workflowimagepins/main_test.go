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
		"shell-key.yml",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := checkYAMLFixture(base, name, true)
			require.NoError(t, err)
			assert.Greater(t, n, 0, name)
		})
	}
	for _, name := range []string{"dockerfile-from", "from-platform", "from-scratch", "from-stage"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := checkDirFixture(filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned-dirs", name), true)
			require.NoError(t, err)
			assert.Greater(t, n, 0)
		})
	}
}

func TestFROMScratchAndStagePass_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	for _, name := range []string{"from-scratch", "from-stage", "from-platform"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n, err := checkDirFixture(filepath.Join(root, "scripts/testdata/workflow-image-pins/pinned-dirs", name), false)
			require.NoError(t, err)
			assert.Equal(t, 0, n)
		})
	}
}

func TestTokenFindingsReportAll_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	hits := tokenFindings("docker run a\npodman run b\nnerdctl run c\n", "f", "run")
	require.Len(t, hits, 3)
}

func TestManifestRequired_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "scripts"), 0o755))
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
