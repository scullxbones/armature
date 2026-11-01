package main

import (
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

func TestPinnedRequiresNameDigest_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	assert.False(t, pinned(""))
	assert.False(t, pinned("alpine:3.22"))
	assert.False(t, pinned("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	assert.True(t, pinned("alpine:3.22@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
}

func TestShellForms_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"docker container run": `docker container run --rm alpine:3.22 echo hi`,
		"docker image pull":    `docker image pull alpine:3.22`,
		"sh -c nested":         `sh -c "docker run --rm alpine:3.22 echo hi"`,
		"global flags":         `docker --log-level debug run --rm alpine:3.22 echo hi`,
		"label then image":     `docker run --rm --label app=arm alpine:3.22 echo hi`,
		"mount not image":      `docker run --rm --mount type=bind,src=/tmp,dst=/tmp alpine:3.22 echo hi`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			imgs := imagesInShell(line)
			require.NotEmpty(t, imgs, line)
			assert.Equal(t, "alpine:3.22", imgs[0], imgs)
		})
	}
}

func TestCollectEmptyDockerUses_REQ_PORC_BASH32(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	path := filepath.Join(root, "scripts/testdata/workflow-image-pins/unpinned/uses-docker-empty.yml")
	refs, err := collectFile(path, "uses-docker-empty.yml")
	require.NoError(t, err)
	require.NotEmpty(t, refs)
	assert.Equal(t, "", refs[0].Ref)
	assert.False(t, pinned(refs[0].Ref))
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
