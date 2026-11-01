package worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestReplaceProvenGitCopiedID_Table_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	readErr := errors.New("main id unreadable")
	cases := []struct {
		name     string
		existing string
		mainID   string
		mainErr  error
		want     bool
	}{
		{name: "a_main_empty_keep", existing: "explicit-wt", mainID: "", mainErr: nil, want: false},
		{name: "b_main_differs_keep", existing: "explicit-wt", mainID: "main-worker", mainErr: nil, want: false},
		{name: "c_main_equals_replace", existing: "main-worker", mainID: "main-worker", mainErr: nil, want: true},
		{name: "d_main_read_error_keep", existing: "explicit-wt", mainID: "", mainErr: readErr, want: false},
		{name: "empty_existing_never_replace_via_helper", existing: "", mainID: "main-worker", mainErr: nil, want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, replaceProvenGitCopiedID(tc.existing, tc.mainID, tc.mainErr))
		})
	}
}

// Table via real git worktrees — runs on host Git (CI 2.43+) and on the
// check-git-2-34 job (exact 2.34.0). Cases (a)–(d) from the seed keep/replace class.
func TestSeedWorktreeIdentity_KeepUnlessProvenCopy_Table_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()

	t.Run("a_main_no_id_explicit_wt_kept", func(t *testing.T) {
		t.Parallel()
		fx := gittest.InitWithOrigin(t)
		repo := fx.Dir
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
		wt := filepath.Join(t.TempDir(), "explicit-wt")
		gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")
		_, err := Init(InitWorkerInput{RepoPath: wt, ID: "explicit-wt"})
		require.NoError(t, err)
		// Main never received armature.worker-id in worktree config.
		_, mainErr := adapters.New(repo).ReadGitConfigWorktree(gitConfigKey)
		require.Error(t, mainErr, "fixture: main must have no worktree worker id")

		require.NoError(t, SeedWorktreeIdentity(wt, "override-worker"))
		got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
		require.NoError(t, err)
		assert.Equal(t, "explicit-wt", got, "(a) main no id + explicit wt id must be kept")
	})

	t.Run("b_main_differs_kept", func(t *testing.T) {
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

		require.NoError(t, SeedWorktreeIdentity(wt, "override-worker"))
		got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
		require.NoError(t, err)
		assert.Equal(t, "other-worker", got, "(b) main differs → keep explicit dest id")
	})

	t.Run("c_main_equals_real_copy_replaced", func(t *testing.T) {
		t.Parallel()
		fx := gittest.InitWithOrigin(t)
		repo := fx.Dir
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
		_, err := Init(InitWorkerInput{RepoPath: repo, ID: "main-worker"})
		require.NoError(t, err)
		wt := filepath.Join(t.TempDir(), "copied-wt")
		gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")
		// Git 2.43+ copies; Git 2.34 leaves empty — simulate the copy.
		if v, readErr := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey); readErr != nil || strings.TrimSpace(v) == "" {
			require.NoError(t, adapters.New(wt).SetGitConfigWorktree(gitConfigKey, "main-worker"))
		} else {
			require.Equal(t, "main-worker", strings.TrimSpace(v))
		}

		require.NoError(t, SeedWorktreeIdentity(wt, "override-worker"))
		got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
		require.NoError(t, err)
		assert.Equal(t, "override-worker", got, "(c) proven copy must yield to effective claim owner")
	})

	t.Run("d_main_id_read_errors_kept", func(t *testing.T) {
		t.Parallel()
		fx := gittest.InitWithOrigin(t)
		repo := fx.Dir
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
		wt := filepath.Join(t.TempDir(), "kept-wt")
		gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")
		_, err := Init(InitWorkerInput{RepoPath: wt, ID: "kept-worker"})
		require.NoError(t, err)
		// Break main worktree config.worktree readability after dest has an id.
		mainCfg := filepath.Join(repo, ".git", "config.worktree")
		require.NoError(t, adapters.New(repo).SetGitConfigWorktree(gitConfigKey, "main-temp"))
		require.NoError(t, os.Chmod(mainCfg, 0o000))
		t.Cleanup(func() { _ = os.Chmod(mainCfg, 0o600) })

		require.NoError(t, SeedWorktreeIdentity(wt, "override-worker"))
		got, err := adapters.New(wt).ReadGitConfigWorktree(gitConfigKey)
		require.NoError(t, err)
		assert.Equal(t, "kept-worker", got, "(d) main id read error → keep existing dest id")
	})
}

func TestMainWorktreePath_UsesPorcelainFirstEntry_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, repo, "worktree", "add", "--detach", wt, "HEAD")

	mainFromRepo, err := mainWorktreePath(repo)
	require.NoError(t, err)
	mainFromWT, err := mainWorktreePath(wt)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(mainFromRepo), filepath.Clean(mainFromWT),
		"linked and main must agree on porcelain first entry")
	assert.Equal(t, filepath.Clean(repo), filepath.Clean(mainFromRepo))
}
