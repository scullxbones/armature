package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
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

func TestPushOpsProofRefLeavesOriginArmatureUntouched_REQ_481(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	run(t, fx.Dir, "git", "commit", "--allow-empty", "-m", "init")
	_, err := runTrls(t, fx.Dir, "bootstrap")
	require.NoError(t, err)

	env := map[string]string{"ARM_LAND_PROOF_REF": "refs/heads/_armature-ci-proof/99"}
	out, err := newArm(t, fx.Dir, env).Run("push-ops", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "refs/heads/_armature-ci-proof/99")
	assert.Contains(t, out, "origin/_armature unchanged")

	js, err := newArm(t, fx.Dir, env).Run("push-ops", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, js, `"branch":"refs/heads/_armature-ci-proof/99"`)

	missing := exec.CommandContext(t.Context(), "git", "-C", fx.Origin, "rev-parse", "--verify", "refs/heads/_armature")
	if err := missing.Run(); err == nil {
		t.Fatal("proof publish updated origin/_armature")
	}
	proof, err := exec.CommandContext(t.Context(), "git", "-C", fx.Origin, "rev-parse", "refs/heads/_armature-ci-proof/99").Output()
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(proof)))
}
