package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runInventoryGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func TestParsePorcelainBlocks_ExcludesPrunableAndPreservesDetached_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	items := parsePorcelainBlocks("worktree /repo\nbranch refs/heads/main\n\n" +
		"worktree /repo/.worktrees/deleted\nbranch refs/heads/task/deleted\nprunable gitdir missing\n\n" +
		"worktree /repo/.worktrees/rebasing\ndetached\n")

	assert.Len(t, items, 3)
	assert.True(t, items[1].prunable)
	assert.Equal(t, "detached", items[2].branch)
}

func TestResolveGitDirHandlesMainLinkedAndInvalidWorktrees(t *testing.T) {
	t.Parallel()
	main := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(main, ".git"), 0755))
	got, err := ResolveGitDir(main)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(main, ".git"), got)

	linked := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: ../git-dir\n"), 0644))
	got, err = ResolveGitDir(linked)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(linked, "../git-dir"), got)

	malformed := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(malformed, ".git"), []byte("not a gitdir\n"), 0644))
	_, err = ResolveGitDir(malformed)
	assert.Error(t, err)

	_, err = ResolveGitDir(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

func TestReadBindingPrefersCurrentAndFallsBackToLegacy(t *testing.T) {
	t.Parallel()
	current := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(current, "armature-issue-id"), []byte("current-01\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(current, "armature-task-id"), []byte("legacy-01\n"), 0644))
	got, err := ReadBinding(current)
	require.NoError(t, err)
	assert.Equal(t, "current-01", got)

	legacy := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "armature-task-id"), []byte("legacy-01\n"), 0644))
	got, err = ReadBinding(legacy)
	require.NoError(t, err)
	assert.Equal(t, "legacy-01", got)

	missing, err := ReadBinding(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestListManagedUsesCanonicalRootAndMarkerIdentity(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	runInventoryGit(t, repo, "init", "-q")
	runInventoryGit(t, repo, "config", "user.email", "test@example.com")
	runInventoryGit(t, repo, "config", "user.name", "Test")
	runInventoryGit(t, repo, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0644))
	runInventoryGit(t, repo, "add", "README.md")
	runInventoryGit(t, repo, "commit", "-q", "-m", "initial")
	managedPath := filepath.Join(repo, ".worktrees", "team", "task-01")
	require.NoError(t, os.MkdirAll(filepath.Dir(managedPath), 0755))
	runInventoryGit(t, repo, "worktree", "add", "-b", "task/team-task-01", managedPath)
	gitDir, err := ResolveGitDir(managedPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "armature-issue-id"), []byte("team/task-01"), 0644))

	items, err := ListManaged(repo)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, managedPath, items[0].Path)
	assert.Equal(t, "team/task-01", items[0].Binding)

	_, err = ListManaged(filepath.Join(t.TempDir(), "not-a-repo"))
	assert.Error(t, err)
}

func TestRootMembershipAndIssueLookupUseBoundaries(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), ".worktrees")
	assert.True(t, IsUnderRoot(root, root))
	assert.True(t, IsUnderRoot(filepath.Join(root, "task-01"), root))
	assert.False(t, IsUnderRoot(root+"-old", root))
	assert.False(t, IsUnderRoot(filepath.Join(filepath.Dir(root), "other"), root))
}

func TestCanonicalRoot_RelativeRepoPathIsAbsolute(t *testing.T) {
	t.Parallel()
	root := CanonicalRoot(".")
	assert.True(t, filepath.IsAbs(root), "canonical root must be absolute, got %q", root)
}

// TestSelectByIssue_ResolutionTriState_REQ_LNGHZN_S5_T6 verifies that selection
// reports THREE outcomes, not two. A single bool cannot distinguish "nothing to
// act on" from "more than one candidate and no way to choose", and callers that
// collapse the two skip a fail-closed gate instead of refusing (I5/I6).
func TestSelectByIssue_ResolutionTriState_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	// No bound entry -> NotFound. There is genuinely nothing to act on; a
	// caller may legitimately fall through to a read-only gate lookup.
	_, res := SelectByIssue([]Meta{{Path: "/other", Binding: "other"}}, "task-01", "/repo/.worktrees/task-01")
	assert.Equal(t, NotFound, res)

	// Exactly one bound entry -> Bound, regardless of recorded path.
	single := []Meta{{Path: "/repo/.worktrees/task-01", Binding: "task-01"}}
	got, res := SelectByIssue(single, "task-01", "")
	require.Equal(t, Bound, res)
	assert.Equal(t, "/repo/.worktrees/task-01", got.Path)

	// Two bound entries + recorded path -> the recorded-path entry wins.
	dup := []Meta{
		{Path: "/legacy/explicit", Binding: "task-01"},
		{Path: "/repo/.worktrees/task-01", Binding: "task-01"},
	}
	got, res = SelectByIssue(dup, "task-01", "/repo/.worktrees/task-01")
	require.Equal(t, Bound, res)
	assert.Equal(t, "/repo/.worktrees/task-01", got.Path)

	// Two bound entries + empty recorded path -> Ambiguous, NOT NotFound. This
	// is the distinction the old bool erased: the caller must refuse, not treat
	// it as "no worktree" and continue.
	_, res = SelectByIssue(dup, "task-01", "")
	assert.Equal(t, Ambiguous, res)

	// Two bound entries + recorded path matching neither -> Ambiguous.
	_, res = SelectByIssue(dup, "task-01", "/somewhere/else")
	assert.Equal(t, Ambiguous, res)
}

// TestAnyBound_ExistenceIsOverInclusive_REQ_LNGHZN_S5_T6 pins the difference
// between the two questions. Existence asks "is any worktree bound to this
// issue?" and answers YES where selection refuses, because finding one PREVENTS
// a destructive act (doctor releasing a live claim) rather than causing one.
func TestAnyBound_ExistenceIsOverInclusive_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	dup := []Meta{
		{Path: "/legacy/explicit", Binding: "task-01"},
		{Path: "/repo/.worktrees/task-01", Binding: "task-01"},
	}

	// The defining case: selection is Ambiguous here, existence is true. A live
	// claim owning two bound worktrees must never be reported as having none.
	_, res := SelectByIssue(dup, "task-01", "")
	require.Equal(t, Ambiguous, res)
	assert.True(t, AnyBound(dup, "task-01", ""))

	// No bound entry -> false.
	assert.False(t, AnyBound([]Meta{{Path: "/other", Binding: "other"}}, "task-01", ""))

	// One bound entry, no recorded path -> true.
	single := []Meta{{Path: "/legacy/explicit", Binding: "task-01"}}
	assert.True(t, AnyBound(single, "task-01", ""))

	// A recorded path may drift after `git worktree move`; the binding is still
	// live evidence and must suppress destructive claim release.
	assert.True(t, AnyBound(dup, "task-01", "/repo/.worktrees/task-01"))
	assert.True(t, AnyBound(single, "task-01", "/repo/.worktrees/task-01"))
}

// TestHasPrunableRegistration_DetectsExactPath_REQ_LNGHZN_S5 verifies a managed
// worktree whose directory was deleted leaves a prunable registration that
// HasPrunableRegistration detects at its exact path (and that a live worktree is
// not reported as prunable).
func TestHasPrunableRegistration_DetectsExactPath_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	runInventoryGit(t, repo, "init", "-q")
	runInventoryGit(t, repo, "config", "user.email", "test@example.com")
	runInventoryGit(t, repo, "config", "user.name", "Test")
	runInventoryGit(t, repo, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0644))
	runInventoryGit(t, repo, "add", "README.md")
	runInventoryGit(t, repo, "commit", "-q", "-m", "initial")

	wtPath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(filepath.Dir(wtPath), 0755))
	runInventoryGit(t, repo, "worktree", "add", "-b", "task/task-01", wtPath)

	// Live worktree: not prunable.
	got, err := HasPrunableRegistration(repo, wtPath)
	require.NoError(t, err)
	assert.False(t, got)

	// Delete the directory out from under git: registration survives, prunable.
	require.NoError(t, os.RemoveAll(wtPath))
	got, err = HasPrunableRegistration(repo, wtPath)
	require.NoError(t, err)
	assert.True(t, got)

	// A different, unregistered path is never reported prunable.
	got, err = HasPrunableRegistration(repo, filepath.Join(repo, ".worktrees", "task-02"))
	require.NoError(t, err)
	assert.False(t, got)
}
