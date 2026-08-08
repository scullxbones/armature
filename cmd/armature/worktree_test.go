package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worktreeReconcileFixture is a repo wired with real claim/transition ops so the
// worktree commands exercise the true production read path (snapshot store ->
// materialize -> Reconcile). Hand-editing state JSON is useless here: the
// snapshot store re-materializes from the op log on every load, so the classes
// below are driven entirely by ops.
type worktreeReconcileFixture struct {
	repo       string
	boundPath  string // .worktrees/task-bound  (claimed, live)   -> bound
	orphanPath string // .worktrees/task-orphan (on disk, no claim) -> orphan
	ghostPath  string // .worktrees/task-ghost  (claimed, removed) -> ghost
	gcPath     string // .worktrees/task-gc     (merged, on disk)  -> gc_ready / removed
}

// setupWorktreeReconcileFixture builds a repo with one worktree in each
// reconcile class using real arm commands.
func setupWorktreeReconcileFixture(t *testing.T) worktreeReconcileFixture {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Distinct scopes so concurrent claims never trip scope-overlap handling.
	create := func(id, scope string) {
		_, cerr := runTrls(t, repo, "create", "--id", id, "--title", id, "--type", "task", "--scope", scope)
		require.NoError(t, cerr)
	}
	create("task-bound", "a.go")
	create("task-orphan", "b.go")
	create("task-ghost", "c.go")
	create("task-gc", "d.go")

	// task-bound: live claim with a managed worktree -> bound.
	_, err = runTrls(t, repo, "claim", "task-bound", "--worktree")
	require.NoError(t, err)

	// task-ghost: live claim, then delete the worktree off disk without arm.
	// The claim op still records its WorktreePath, so reconcile sees a live
	// claim whose worktree vanished -> ghost.
	_, err = runTrls(t, repo, "claim", "task-ghost", "--worktree")
	require.NoError(t, err)
	ghostPath := filepath.Join(repo, ".worktrees", "task-ghost")
	run(t, repo, "git", "worktree", "remove", "--force", ghostPath)

	// task-gc: live claim (records WorktreePath), then transition to merged
	// while the worktree is still on disk -> gc_ready.
	_, err = runTrls(t, repo, "claim", "task-gc", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-gc", "--to", "merged", "--force")
	require.NoError(t, err)

	// task-orphan: a managed worktree on disk that no issue claims -> orphan.
	orphanPath := filepath.Join(repo, ".worktrees", "task-orphan")
	run(t, repo, "git", "worktree", "add", orphanPath, "-b", "task/task-orphan")

	// Materialize so on-disk state reflects the ops (belt and suspenders; the
	// snapshot store also materializes on load).
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	return worktreeReconcileFixture{
		repo:       repo,
		boundPath:  filepath.Join(repo, ".worktrees", "task-bound"),
		orphanPath: orphanPath,
		ghostPath:  ghostPath,
		gcPath:     filepath.Join(repo, ".worktrees", "task-gc"),
	}
}

// listJSON runs `worktree list --format json` and decodes it into string slices.
func listJSON(t *testing.T, repo string) map[string][]string {
	t.Helper()
	out, err := runTrls(t, repo, "worktree", "list", "--format", "json")
	require.NoError(t, err)

	var raw map[string][]string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &raw))
	return raw
}

// TestWorktreeListClassifiesEachClass_REQ_LNGHZN_S5_T2 asserts real reconciliation
// outcomes: the claimed worktree is bound, the unclaimed one is an orphan, the
// merged one is gc_ready, and the vanished claim is a ghost. These assertions
// FAIL against the pre-F1 code, which read from a non-existent issues dir and so
// classified nothing (every list came back empty).
func TestWorktreeListClassifiesEachClass_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	res := listJSON(t, fx.repo)

	assert.Contains(t, res["bound"], "task-bound", "claimed live worktree must be bound")
	assert.Contains(t, res["orphans"], "task-orphan", "unclaimed managed worktree must be an orphan")
	assert.Contains(t, res["gc_ready"], "task-gc", "merged worktree still on disk must be gc_ready")
	assert.Contains(t, res["ghosts"], "task-ghost", "claimed-but-removed worktree must be a ghost")

	// Cross-class: a bound worktree is never simultaneously an orphan/gc target.
	assert.NotContains(t, res["orphans"], "task-bound")
	assert.NotContains(t, res["gc_ready"], "task-bound")
}

// TestWorktreeGCRemovesMergedWorktree_REQ_LNGHZN_S5_T2 asserts gc actually removes
// the merged worktree from disk and reports it as removed. This FAILS against the
// pre-F1 code, where gc found zero issues and removed nothing.
func TestWorktreeGCRemovesMergedWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	// Precondition: the merged worktree exists on disk.
	_, err := os.Stat(fx.gcPath)
	require.NoError(t, err, "gc worktree must exist before gc")

	out, err := runTrls(t, fx.repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)

	var res struct {
		Removed []string `json:"removed"`
		Skipped []string `json:"skipped"`
		Failed  []string `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.Contains(t, res.Removed, "task-gc", "merged worktree must be reported removed")
	assert.NotContains(t, res.Skipped, "task-gc")
	assert.Empty(t, res.Failed)

	// The worktree is actually gone from disk.
	_, statErr := os.Stat(fx.gcPath)
	assert.True(t, os.IsNotExist(statErr), "gc must remove the worktree directory from disk")
}

// TestWorktreeGCDryRunKeepsWorktree_REQ_LNGHZN_S5_T2 verifies --dry-run reports the
// merged worktree as a removal candidate but leaves it on disk.
func TestWorktreeGCDryRunKeepsWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	out, err := runTrls(t, fx.repo, "worktree", "gc", "--dry-run", "--format", "json")
	require.NoError(t, err)

	var res struct {
		DryRun      bool     `json:"dry_run"`
		WouldRemove []string `json:"would_remove"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.True(t, res.DryRun)
	assert.Contains(t, res.WouldRemove, "task-gc")

	// Still on disk after a dry run.
	_, statErr := os.Stat(fx.gcPath)
	assert.NoError(t, statErr, "dry-run must not remove the worktree")
}

// TestWorktreeListHumanFormat_REQ_LNGHZN_S5_T2 verifies the human format renders
// the class headings and the classified issue IDs.
func TestWorktreeListHumanFormat_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	out, err := runTrls(t, fx.repo, "worktree", "list", "--format", "human")
	require.NoError(t, err)

	assert.Contains(t, out, "BOUND WORKTREES:")
	assert.Contains(t, out, "task-bound")
	assert.Contains(t, out, "task-orphan")
	assert.Contains(t, out, "task-gc")
}

// TestReadManagedWorktrees_GitFailureIsError_REQ_LNGHZN_S5_T2 verifies that a
// `git worktree list` failure is surfaced as a non-nil error rather than
// swallowed into an empty inventory. An empty list from a transient git failure
// would mislabel every live claim as a ghost and make gc a silent no-op, so the
// commands must fail closed instead.
func TestReadManagedWorktrees_GitFailureIsError_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	// A plain temp dir is not a git repository, so `git -C <dir> worktree list`
	// exits non-zero.
	nonRepo := t.TempDir()

	worktrees, err := readManagedWorktrees(nonRepo)
	require.Error(t, err, "a git worktree list failure must return an error, not empty success")
	assert.Nil(t, worktrees, "no inventory should be returned on git failure")
}

// TestIsManaged_PrefixMatch_REQ_LNGHZN_S5_T2 verifies isManaged uses a
// path-prefix test rooted at <repo>/.worktrees/, not a naive substring match.
func TestIsManaged_PrefixMatch_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	managed := filepath.Join(repo, ".worktrees", "task-01")
	assert.True(t, isManaged(repo, managed), "path under repo/.worktrees must be managed")

	// A sibling directory that merely contains the substring ".worktrees" but is
	// not under this repo's managed root must NOT be classified as managed.
	notManaged := filepath.Join(t.TempDir(), ".worktrees-backup", "task-01")
	assert.False(t, isManaged(repo, notManaged), "unrelated path must not be managed")

	// The managed root itself (no trailing child) is not a managed worktree.
	assert.False(t, isManaged(repo, filepath.Join(repo, ".worktrees")), "bare .worktrees dir is not a worktree")
}

// TestManagedWorktreeRoot_RelativeRepoPath_REQ_LNGHZN_S5_T2 pins the fix for the
// reconcile no-op that survived F1: in production ctx.RepoPath defaults to "."
// (cwd) when --repo is not passed, while `git worktree list` emits ABSOLUTE
// paths. A relative managed root would never prefix-match those, so every
// managed worktree was misclassified as not-managed and reconcile came back
// empty. The root must be absolute even for a relative repoPath.
func TestManagedWorktreeRoot_RelativeRepoPath_REQ_LNGHZN_S5_T2(t *testing.T) {
	root := managedWorktreeRoot(".")
	assert.True(t, filepath.IsAbs(root), "managed root must be absolute, got %q", root)

	// An absolute worktree path under the cwd's .worktrees must match even when
	// the repo path was given relatively.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	assert.True(t, isManaged(".", filepath.Join(cwd, ".worktrees", "task-01")),
		"absolute worktree under cwd must be managed when repoPath is \".\"")
}

// TestGoWorkMitigationApplied_REQ_LNGHZN_S5_T3 drives `arm claim --worktree`
// end-to-end and asserts the worktree mitigation ran with the correct effect:
// with no go.work in the main tree, the mitigation is a no-op — it neither
// creates a go.work in the main tree nor a bare go.work in the worktree (the
// latter would break `go build ./...` inside the worktree).
func TestGoWorkMitigationApplied_REQ_LNGHZN_S5_T3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-mit", "--title", "Mit task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-mit", "--worktree")
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(repo, "go.work"))
	assert.True(t, os.IsNotExist(statErr), "no go.work should be created in the main tree")

	wt := filepath.Join(repo, ".worktrees", "task-mit")
	_, statErr = os.Stat(filepath.Join(wt, "go.work"))
	assert.True(t, os.IsNotExist(statErr), "no go.work should be created in the worktree")
}
