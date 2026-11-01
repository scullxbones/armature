package adapters_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitVersionOutput_Boundary_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()

	tooOld, err := adapters.ParseGitVersionOutput("git version 2.33.0")
	require.NoError(t, err)
	assert.Equal(t, adapters.Version{Major: 2, Minor: 33, Patch: 0}, tooOld)
	assert.False(t, tooOld.AtLeast(adapters.MinGitVersion()))
	require.Error(t, adapters.CheckMinGitVersion(tooOld))
	assert.Contains(t, adapters.CheckMinGitVersion(tooOld).Error(), "2.33.0")
	assert.Contains(t, adapters.CheckMinGitVersion(tooOld).Error(), "2.34.0")

	tooOldPatch, err := adapters.ParseGitVersionOutput("git version 2.33.1")
	require.NoError(t, err)
	assert.False(t, tooOldPatch.AtLeast(adapters.MinGitVersion()))
	require.Error(t, adapters.CheckMinGitVersion(tooOldPatch))

	minOK, err := adapters.ParseGitVersionOutput("git version 2.34.0")
	require.NoError(t, err)
	assert.Equal(t, adapters.Version{Major: 2, Minor: 34, Patch: 0}, minOK)
	assert.False(t, minOK.PreRelease)
	assert.True(t, minOK.AtLeast(adapters.MinGitVersion()))
	require.NoError(t, adapters.CheckMinGitVersion(minOK))

	apple, err := adapters.ParseGitVersionOutput("git version 2.43.0 (Apple Git-155)\n")
	require.NoError(t, err)
	assert.Equal(t, adapters.Version{Major: 2, Minor: 43, Patch: 0}, apple)
	assert.False(t, apple.PreRelease)
	assert.True(t, apple.AtLeast(adapters.MinGitVersion()))

	windows, err := adapters.ParseGitVersionOutput("git version 2.34.0.windows.1")
	require.NoError(t, err)
	assert.Equal(t, adapters.Version{Major: 2, Minor: 34, Patch: 0}, windows)
	assert.False(t, windows.PreRelease, "windows dotted suffix is not a prerelease")
	require.NoError(t, adapters.CheckMinGitVersion(windows))

	remergeFloor := adapters.Version{Major: 2, Minor: 36, Patch: 0}
	assert.False(t, minOK.AtLeast(remergeFloor), "2.34 must not claim remerge (≥2.36)")
	assert.True(t, adapters.Version{Major: 2, Minor: 36, Patch: 0}.AtLeast(remergeFloor))
}

func TestParseGitVersionOutput_PrereleaseBelowRelease_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()

	rc, err := adapters.ParseGitVersionOutput("git version 2.34.0-rc1")
	require.NoError(t, err)
	assert.Equal(t, 2, rc.Major)
	assert.Equal(t, 34, rc.Minor)
	assert.Equal(t, 0, rc.Patch)
	assert.True(t, rc.PreRelease)
	assert.Equal(t, "2.34.0-pre", rc.String())
	assert.False(t, rc.AtLeast(adapters.MinGitVersion()), "2.34.0-rc1 must be below 2.34.0")
	require.Error(t, adapters.CheckMinGitVersion(rc))
	assert.Contains(t, adapters.CheckMinGitVersion(rc).Error(), "2.34.0-pre")
	assert.Contains(t, adapters.CheckMinGitVersion(rc).Error(), "2.34.0")

	rcDot, err := adapters.ParseGitVersionOutput("git version 2.34.0-rc.1")
	require.NoError(t, err)
	assert.True(t, rcDot.PreRelease)
	assert.False(t, rcDot.AtLeast(adapters.MinGitVersion()))

	// A newer prerelease than the floor still clears the floor.
	newerRC, err := adapters.ParseGitVersionOutput("git version 2.35.0-rc1")
	require.NoError(t, err)
	assert.True(t, newerRC.PreRelease)
	assert.True(t, newerRC.AtLeast(adapters.MinGitVersion()), "2.35.0-rc1 is above 2.34.0 release")
}

func TestParseGitVersionOutput_RejectsGarbage_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "version 2.34.0", "git version", "git version x.y.z", "git version 2"} {
		_, err := adapters.ParseGitVersionOutput(in)
		require.Error(t, err, "input %q", in)
	}
}

func TestRequireMinGit_RealBinary_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	have, err := adapters.DetectGitVersion(nil)
	require.NoError(t, err)
	require.True(t, have.AtLeast(adapters.MinGitVersion()),
		"host git %s must meet armature minimum %s (CI and developer hosts)", have, adapters.MinGitVersion())
	require.NoError(t, adapters.RequireMinGit(nil))
	require.NoError(t, adapters.CheckMinGitVersion(have))
}
