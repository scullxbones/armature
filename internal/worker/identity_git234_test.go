package worker

import (
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedWorktreeIdentity_FillsMissingOnGitFloor_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")

	ident, err := Init(InitWorkerInput{RepoPath: repo, ID: "claim-worker"})
	require.NoError(t, err)
	assert.Equal(t, "claim-worker", ident.ID)

	wt := filepath.Join(t.TempDir(), "task-wt")
	// Detached add matches claim's provision path; do not rely on git copying
	// config.worktree (absent on Git 2.34, present on newer runners).
	gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")

	_, err = adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
	if err == nil {
		t.Log("host git copied config.worktree on worktree add; seed must still be idempotent")
	}

	require.NoError(t, SeedWorktreeIdentity(wt, "claim-worker"))
	got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
	require.NoError(t, err)
	assert.Equal(t, "claim-worker", got)

	require.NoError(t, SeedWorktreeIdentity(wt, "claim-worker"), "second seed must leave an existing id alone")
	got, err = adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
	require.NoError(t, err)
	assert.Equal(t, "claim-worker", got)
}

func TestSeedWorktreeIdentity_DoesNotOverwriteExplicitWorktreeId_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := Init(InitWorkerInput{RepoPath: repo, ID: "main-worker"})
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "other-wt")
	gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")
	_, err = Init(InitWorkerInput{RepoPath: wt, ID: "other-worker"})
	require.NoError(t, err)

	require.NoError(t, SeedWorktreeIdentity(wt, "main-worker"))
	got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
	require.NoError(t, err)
	assert.Equal(t, "other-worker", got, "explicit worktree id must win over seed")
}

func TestSeedWorktreeIdentity_UsesEffectiveClaimIDNotRepoConfig_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	// No Init on repo: claim with --worker-id / ARM_WORKER_ID when the checkout
	// has no configured id. Old seed (ResolveIdentity from fromRepo) failed;
	// effective claim id must still land on the task worktree.
	wt := filepath.Join(t.TempDir(), "override-wt")
	gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")

	require.NoError(t, SeedWorktreeIdentity(wt, "override-worker~slot-a"))
	got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
	require.NoError(t, err)
	assert.Equal(t, "override-worker", got, "effective claim id (slot stripped) must be seeded")
}
