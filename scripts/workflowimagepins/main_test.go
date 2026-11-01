package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelftestFixtures_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	require.NoError(t, runSelftest(root, ioDiscard{}, ioDiscard{}))
}

func TestCollectForms_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	base := filepath.Join(root, "scripts", "testdata", "workflow-image-pins", "unpinned")
	cases := []struct {
		file string
		path string
	}{
		{"container-string.yml", "jobs.x.container"},
		{"container-image.yml", "jobs.x.container.image"},
		{"services-image.yml", "jobs.x.services.db.image"},
		{"uses-docker.yml", "jobs.x.steps[0].uses"},
		{"docker-run.yml", "jobs.x.steps[0].run"},
		{"docker-pull.yml", "jobs.x.steps[0].run"},
		{"docker-create.yml", "jobs.x.steps[0].run"},
		{"podman-run.yml", "jobs.x.steps[0].run"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			refs, err := collectFile(filepath.Join(base, tc.file), "unpinned/"+tc.file)
			require.NoError(t, err)
			require.NotEmpty(t, refs)
			found := false
			for _, r := range refs {
				if r.Path == tc.path {
					found = true
					require.False(t, pinned(r.Ref), "expected unpinned ref")
				}
			}
			require.True(t, found, "missing path %s in %#v", tc.path, refs)
		})
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
