package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")
	run("commit", "--allow-empty", "-m", "init")
	return dir
}

func TestResolveContext_RequiresOpsWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}

func TestResolveContext_UsesOpsWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	runGit := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "armature.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, issuesDir, ctx.IssuesDir)
	assert.Equal(t, repo, ctx.RepoPath)
	assert.Equal(t, worktreePath, ctx.WorktreePath)
}

func TestResolveLayout_SurvivesUnreadableConfig(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".armature")
	require.NoError(t, os.MkdirAll(worktreePath, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(worktreePath, "config.json"),
		[]byte(`{"project_type":"go","mystery_knob":1}`),
		0o600,
	))
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "config", "armature.ops-worktree-path", worktreePath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git config: %s", out)

	_, err = ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery_knob")

	layout, err := ResolveLayout(repo)
	require.NoError(t, err)
	assert.Equal(t, repo, layout.RepoPath)
	assert.Equal(t, worktreePath, layout.WorktreePath)
	assert.Equal(t, worktreePath, layout.IssuesDir)
}

func TestResolveContext_DualBranchWithStrayConfigJSONStillDetectedAsUnmigrated_REQ_LNGHZN_S1(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	require.NoError(t, WriteConfig(filepath.Join(worktreePath, "config.json"), cfg))

	runGit := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	runGit("config", "armature.ops-worktree-path", worktreePath)

	ctx, err := ResolveContext(repo)
	require.NoError(t, err)
	assert.Equal(t, issuesDir, ctx.IssuesDir,
		"nested .armature/ must win over a stray root config.json when detecting dual-branch layout")
	assert.True(t, DetectUnmigratedLayout(ctx.WorktreePath, ctx.IssuesDir),
		"a stray config.json at .arm/ root must not suppress the unmigrated-layout refusal")
}

func TestResolveContext_ErrorWhenOpsWorktreePathNotSet_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}

func TestContextStateDir(t *testing.T) {
	t.Parallel()
	ctx := &Context{
		StateDir: "/tmp/armature-state",
	}
	assert.Equal(t, "/tmp/armature-state", ctx.StateDir)
}

func TestResolveContext_GitWorktreeResolvedToParent_REQ_SB_T5(t *testing.T) {
	t.Parallel()
	parentRepo := initTestRepo(t)

	worktreeCheckout := filepath.Join(parentRepo, "worktree-checkout")
	require.NoError(t, os.MkdirAll(worktreeCheckout, 0755))

	gitdirPath := filepath.Join(parentRepo, ".git", "worktrees", "test-wt")
	require.NoError(t, os.MkdirAll(gitdirPath, 0755))
	gitFileContent := fmt.Sprintf("gitdir: %s\n", gitdirPath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeCheckout, ".git"), []byte(gitFileContent), 0644))

	opsWorktree := filepath.Join(parentRepo, ".arm")
	opsIssuesDir := filepath.Join(opsWorktree, ".armature")
	require.NoError(t, os.MkdirAll(opsIssuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(opsIssuesDir, "config.json"), DefaultConfig("go")))

	cmd := exec.CommandContext(context.Background(), "git", "-C", parentRepo, "config", "armature.ops-worktree-path", opsWorktree)
	require.NoError(t, cmd.Run())

	ctx, err := ResolveContext(worktreeCheckout)
	require.NoError(t, err)
	assert.Equal(t, parentRepo, ctx.RepoPath)
	assert.Equal(t, opsIssuesDir, ctx.IssuesDir)
	assert.Equal(t, opsWorktree, ctx.WorktreePath)
}

func TestResolveLayout_NonexistentRepoIsNotMissingLayout_REQ_AOC_S2_T5(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "no-such-repo")
	_, err := ResolveLayout(missing)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "armature.ops-worktree-path must be set",
		"inaccessible GitConfig failures must not be classified as a missing layout")
}
