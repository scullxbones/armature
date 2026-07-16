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

	// Without ops-worktree-path set, ResolveContext should error
	_, err := ResolveContext(repo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "armature.ops-worktree-path")
}

func TestResolveContext_UsesOpsWorktree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	// Create ops worktree with .armature/ inside
	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	// Set git config to point to ops worktree
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

// TestResolveContext_DualBranchWithStrayConfigJSONStillDetectedAsUnmigrated_REQ_LNGHZN_S1
// guards against resolveIssuesDir's layout heuristic being fooled by a stray
// config.json sitting at the .arm/ worktree root alongside the real nested
// .armature/ directory. A collapsed worktree never has that nested
// subdirectory by construction, so its presence must take priority over a
// config.json probe when deciding whether the layout is dual-branch/unmigrated.
func TestResolveContext_DualBranchWithStrayConfigJSONStillDetectedAsUnmigrated_REQ_LNGHZN_S1(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)

	worktreePath := filepath.Join(repo, ".arm")
	issuesDir := filepath.Join(worktreePath, ".armature")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))
	cfg := DefaultConfig("go")
	require.NoError(t, WriteConfig(filepath.Join(issuesDir, "config.json"), cfg))

	// Stray config.json at the .arm/ worktree root itself (not the nested one),
	// which should NOT be mistaken for an already-collapsed layout.
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

	// Do NOT set ops-worktree-path
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
	// Create a "parent" repo that will be the actual git repo
	parentRepo := initTestRepo(t)

	// Create a worktree checkout directory (simulates git worktree add)
	worktreeCheckout := filepath.Join(parentRepo, "worktree-checkout")
	require.NoError(t, os.MkdirAll(worktreeCheckout, 0755))

	// In a git worktree, .git is a FILE (not directory) containing "gitdir: <path>"
	// The gitdir typically points to .git/worktrees/<name> in the parent repo
	gitdirPath := filepath.Join(parentRepo, ".git", "worktrees", "test-wt")
	require.NoError(t, os.MkdirAll(gitdirPath, 0755))
	gitFileContent := fmt.Sprintf("gitdir: %s\n", gitdirPath)
	require.NoError(t, os.WriteFile(filepath.Join(worktreeCheckout, ".git"), []byte(gitFileContent), 0644))

	// Create ops worktree in parent repo
	opsWorktree := filepath.Join(parentRepo, ".arm")
	opsIssuesDir := filepath.Join(opsWorktree, ".armature")
	require.NoError(t, os.MkdirAll(opsIssuesDir, 0755))
	require.NoError(t, WriteConfig(filepath.Join(opsIssuesDir, "config.json"), DefaultConfig("go")))

	// Set git config in parent to point to ops worktree
	cmd := exec.CommandContext(context.Background(), "git", "-C", parentRepo, "config", "armature.ops-worktree-path", opsWorktree)
	require.NoError(t, cmd.Run())

	// When ResolveContext is called from the worktree checkout path,
	// it should detect that .git is a file and resolve parent repo and ops worktree
	ctx, err := ResolveContext(worktreeCheckout)
	require.NoError(t, err)
	assert.Equal(t, parentRepo, ctx.RepoPath)
	assert.Equal(t, opsIssuesDir, ctx.IssuesDir)
	assert.Equal(t, opsWorktree, ctx.WorktreePath)
}
