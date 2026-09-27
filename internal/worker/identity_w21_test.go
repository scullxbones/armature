package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidWorkerIDRejectedNamedError_REQ_CLAIMORD_W21(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"ABC", "con", strings.Repeat("a", 65), "../x"} {
		err := ValidateWorkerID(id)
		require.Error(t, err, id)
		assert.ErrorIs(t, err, claim.ErrWorkerIDInvalid)
		assert.Equal(t, "WORKER-ID-INVALID", err.Error())
	}
}

func TestInvalidLogSlotRejectedNamedError_REQ_CLAIMORD_W21(t *testing.T) {
	t.Parallel()
	err := ValidateLogSlot("COM1")
	require.Error(t, err)
	assert.ErrorIs(t, err, claim.ErrLogSlotInvalid)
	_, err = ResolveIdentity(IdentityInput{RepoPath: initTempRepo(t), FlagWorkerID: "okid", EnvLogSlot: "NOPE"})
	require.Error(t, err)
	assert.ErrorIs(t, err, claim.ErrLogSlotInvalid)
	assert.Equal(t, "LOG-SLOT-INVALID", err.Error())
}

func TestEnvUsedWhenNoWorktreeId_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	repo := initTempRepo(t)
	require.NoError(t, adapters.New(repo).SetGitConfig(gitConfigKey, "clone-baked-id"))
	ident, err := ResolveIdentity(IdentityInput{
		RepoPath:         repo,
		EnvWorkerID:      "env-worker-id",
		CloneGitConfigID: "clone-baked-id",
	})
	require.NoError(t, err)
	assert.Equal(t, "env-worker-id", ident.ID)
	assert.Equal(t, "env", ident.Source)
}

func TestInheritedEnvDoesNotOverrideWorktreeId_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	repo := initTempRepo(t)
	ident, err := Init(InitWorkerInput{RepoPath: repo, ID: "wt-child-id"})
	require.NoError(t, err)
	assert.Equal(t, "wt-child-id", ident.ID)
	got, err := ResolveIdentity(IdentityInput{
		RepoPath:    repo,
		EnvWorkerID: "coord",
	})
	require.NoError(t, err)
	assert.Equal(t, "wt-child-id", got.ID)
	assert.Equal(t, "worktree-config", got.Source)
	assert.NotContains(t, got.LogPath, "coord")
}

func TestTwoWorktreesDistinctIdsWithoutEnv_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	wt := t.TempDir()
	gittest.Git(t, repo, "worktree", "add", wt, "HEAD")
	a, err := Init(InitWorkerInput{RepoPath: repo})
	require.NoError(t, err)
	b, err := Init(InitWorkerInput{RepoPath: wt})
	require.NoError(t, err)
	assert.NotEqual(t, a.ID, b.ID)
	assert.NotEqual(t, a.LogPath, b.LogPath)
}

func TestWorkerInitDoesNotCopyCloneConfigIntoNewWorktree_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	require.NoError(t, adapters.New(repo).SetGitConfig(gitConfigKey, "baked-clone-id"))
	wt := t.TempDir()
	gittest.Git(t, repo, "worktree", "add", wt, "HEAD")
	ident, err := Init(InitWorkerInput{RepoPath: wt})
	require.NoError(t, err)
	assert.NotEqual(t, "baked-clone-id", ident.ID)
}

func TestWorkerInitRefusesPublishedDuplicateId_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "branch", "-M", "_armature")
	opsDir := filepath.Join(repo, "ops")
	require.NoError(t, os.MkdirAll(opsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(opsDir, "taken-id.log"), []byte("{}\n"), 0o644))
	gittest.Git(t, repo, "add", "ops/taken-id.log")
	gittest.Git(t, repo, "commit", "-m", "publish log")
	gittest.Git(t, repo, "push", "-u", "origin", "HEAD:refs/heads/_armature")
	_, err := Init(InitWorkerInput{RepoPath: repo, ID: "taken-id"})
	require.Error(t, err)
	assert.ErrorIs(t, err, claim.ErrWorkerIDInUse)
	assert.Equal(t, "WORKER-ID-IN-USE", err.Error())
}

func TestLogSlotCollisionDetected_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	repo := initTempRepo(t)
	unlock, err := LockLogID(repo, "same-id")
	require.NoError(t, err)
	defer unlock()
	_, err = LockLogID(repo, "same-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, claim.ErrLogSlotCollision)
	assert.Equal(t, "LOG-SLOT-COLLISION", err.Error())
}

func TestWorkerInitEnablesWorktreeConfigAndMovesCoreKeys_REQ_CLAIMORD_W21(t *testing.T) { //nolint:paralleltest
	repo := initTempRepo(t)
	gc := adapters.New(repo)
	require.NoError(t, gc.SetGitConfig("core.worktree", repo))
	require.NoError(t, EnableWorktreeConfig(repo))
	ext, err := gc.ReadGitConfig("extensions.worktreeConfig")
	require.NoError(t, err)
	assert.Equal(t, "true", ext)
	moved, err := gc.ReadGitConfigWorktree("core.worktree")
	require.NoError(t, err)
	assert.Equal(t, repo, moved)
	_, err = gc.ReadGitConfig("core.worktree")
	assert.Error(t, err, "core.worktree must leave shared config")
}

func TestFlagBeatsWorktreeConfig(t *testing.T) { //nolint:paralleltest
	repo := initTempRepo(t)
	_, err := Init(InitWorkerInput{RepoPath: repo, ID: "wt-id"})
	require.NoError(t, err)
	ident, err := ResolveIdentity(IdentityInput{RepoPath: repo, FlagWorkerID: "flag-id", EnvWorkerID: "env-id"})
	require.NoError(t, err)
	assert.Equal(t, "flag-id", ident.ID)
	assert.Equal(t, "flag", ident.Source)
}
