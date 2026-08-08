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
	assert.Equal(t, "team/task-01", items[0].IssueID)

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

	items := []Meta{{Path: "/one", IssueID: "task-01"}}
	found, ok := FindByIssue(items, "task-01")
	require.True(t, ok)
	assert.Equal(t, "/one", found.Path)
	_, ok = FindByIssue(items, "missing")
	assert.False(t, ok)
}
