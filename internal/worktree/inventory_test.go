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

func TestCanonicalRoot_MissingThroughSymlink_REQ_ARCHIMP_S20(t *testing.T) {
	t.Parallel()
	realRepo := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "repo-link")
	require.NoError(t, os.Symlink(realRepo, link))

	got := CanonicalRoot(link)
	want := NormalizePathAllowingMissing(filepath.Join(realRepo, ".worktrees"))
	assert.Equal(t, want, got)
	assert.Equal(t, CanonicalRoot(realRepo), got)
}

func TestCanonicalPath_MissingThroughSymlinkAgreesWithRoot_REQ_ARCHIMP_S20(t *testing.T) {
	t.Parallel()
	realRepo := t.TempDir()
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "repo-link")
	require.NoError(t, os.Symlink(realRepo, link))

	const issueID = "ISSUE-01"
	root := CanonicalRoot(link)
	path := CanonicalPath(link, issueID)
	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)
	t.Logf("symlink repo=%s real=%s CanonicalRoot=%s CanonicalPath=%s Rel=%s", link, realRepo, root, path, rel)
	assert.Equal(t, issueID, rel)
	assert.Equal(t, CanonicalPath(realRepo, issueID), path)
}

func TestSelectByIssue_ResolutionTriState_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	_, res := SelectByIssue([]Meta{{Path: "/other", Binding: "other"}}, "task-01", "/repo/.worktrees/task-01")
	assert.Equal(t, NotFound, res)

	single := []Meta{{Path: "/repo/.worktrees/task-01", Binding: "task-01"}}
	got, res := SelectByIssue(single, "task-01", "")
	require.Equal(t, Bound, res)
	assert.Equal(t, "/repo/.worktrees/task-01", got.Path)

	dup := []Meta{
		{Path: "/legacy/explicit", Binding: "task-01"},
		{Path: "/repo/.worktrees/task-01", Binding: "task-01"},
	}
	got, res = SelectByIssue(dup, "task-01", "/repo/.worktrees/task-01")
	require.Equal(t, Bound, res)
	assert.Equal(t, "/repo/.worktrees/task-01", got.Path)

	_, res = SelectByIssue(dup, "task-01", "")
	assert.Equal(t, Ambiguous, res)

	_, res = SelectByIssue(dup, "task-01", "/somewhere/else")
	assert.Equal(t, Ambiguous, res)
}

func TestLocateBinding_ExistenceIsOverInclusive_REQ_LNGHZN_S5_T6(t *testing.T) {
	t.Parallel()

	dup := []Meta{
		{Path: "/legacy/explicit", Binding: "task-01"},
		{Path: "/repo/.worktrees/task-01", Binding: "task-01"},
	}

	_, res := SelectByIssue(dup, "task-01", "")
	require.Equal(t, Ambiguous, res)
	loc, _ := LocateBinding(dup, "task-01", "")
	assert.Equal(t, BindingAmbiguous, loc)

	loc, _ = LocateBinding([]Meta{{Path: "/other", Binding: "other"}}, "task-01", "")
	assert.Equal(t, BindingNone, loc)

	single := []Meta{{Path: "/legacy/explicit", Binding: "task-01"}}
	loc, _ = LocateBinding(single, "task-01", "")
	assert.Equal(t, BindingAtRecordedPath, loc)

	loc, _ = LocateBinding(dup, "task-01", "/repo/.worktrees/task-01")
	assert.NotEqual(t, BindingNone, loc)
	loc, _ = LocateBinding(single, "task-01", "/repo/.worktrees/task-01")
	assert.Equal(t, BindingElsewhere, loc)
}

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

	got, err := HasPrunableRegistration(repo, wtPath)
	require.NoError(t, err)
	assert.False(t, got)

	require.NoError(t, os.RemoveAll(wtPath))
	got, err = HasPrunableRegistration(repo, wtPath)
	require.NoError(t, err)
	assert.True(t, got)

	got, err = HasPrunableRegistration(repo, filepath.Join(repo, ".worktrees", "task-02"))
	require.NoError(t, err)
	assert.False(t, got)
}

func TestResolveGitDir_AbsolutePointerMayLeaveWorktree_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	linked := t.TempDir()
	target := filepath.Join(t.TempDir(), "linked.git")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+target+"\n"), 0o644))
	got, err := ResolveGitDir(linked)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

func TestReadBinding_MissingGitDirIsUnbound_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	got, err := ReadBinding(filepath.Join(t.TempDir(), "missing-git-dir"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestResolveGitDir_RejectsEscapingGitEntry_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	wt := filepath.Join(parent, "wt")
	require.NoError(t, os.Mkdir(wt, 0o755))
	outside := filepath.Join(parent, "outside.git")
	require.NoError(t, os.WriteFile(outside, []byte("gitdir: /tmp/evil\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(wt, ".git")))
	_, err := ResolveGitDir(wt)
	require.Error(t, err)

	secret := filepath.Join(parent, "secret")
	require.NoError(t, os.WriteFile(secret, []byte("nope\n"), 0o644))
	_, err = readFileInRoot(wt, filepath.Join("..", "secret"))
	require.Error(t, err)
}

func TestReadBinding_RejectsEscapingBinding_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	gitDir := filepath.Join(parent, "git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	outside := filepath.Join(parent, "stolen")
	require.NoError(t, os.WriteFile(outside, []byte("stolen-id\n"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(gitDir, "armature-issue-id")))
	_, err := ReadBinding(gitDir)
	require.Error(t, err)

	_, err = readFileInRoot(gitDir, filepath.Join("..", "stolen"))
	require.Error(t, err)
}
