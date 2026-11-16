package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLandProofPublishRef_REQ_481(t *testing.T) {
	t.Parallel()
	ref, err := landProofPublishRef(OverlayEnv(OSEnv(), map[string]string{
		"ARM_LAND_PROOF_REF": "refs/heads/_armature-ci-proof/43953957073",
	}))
	require.NoError(t, err)
	assert.Equal(t, "refs/heads/_armature-ci-proof/43953957073", ref)

	ref, err = landProofPublishRef(OverlayEnv(OSEnv(), map[string]string{
		"ARM_LAND_PROOF_REF": "",
	}))
	require.NoError(t, err)
	assert.Empty(t, ref)

	for _, bad := range []string{
		"refs/heads/_armature",
		"refs/heads/main",
		"refs/heads/_armature-ci-proof",
		"refs/heads/_armature-ci-proof/",
		"refs/heads/_armature-ci-proof/../main",
		"refs/heads/_armature-ci-proof/a/b",
		"refs/heads/_armature-ci-proof/-lead",
	} {
		_, err = landProofPublishRef(OverlayEnv(OSEnv(), map[string]string{
			"ARM_LAND_PROOF_REF": bad,
		}))
		require.Error(t, err, bad)
	}
}
