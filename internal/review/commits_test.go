package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewCommits_REQ_TOPTIER_S1_T3(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	// Background Git maintenance can keep writing pack files after git commit
	// exits, racing t.TempDir cleanup. This ephemeral repository needs neither
	// automatic maintenance nor garbage collection.
	run(t, repo, "config", "maintenance.auto", "false")
	run(t, repo, "config", "gc.auto", "0")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "feat.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "feat.go")
	run(t, repo, "commit", "-m", "feat(TOPTIER-S1-T3): add feature")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "fix.go")
	run(t, repo, "commit", "-m", "fix(TOPTIER-S1-T3): fix issue")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "refactor.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "refactor.go")
	run(t, repo, "commit", "-m", "refactor(TOPTIER-S1-T3): refactor module")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "test.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "test.go")
	run(t, repo, "commit", "-m", "test(TOPTIER-S1-T3): add tests")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs.md"), []byte("# Docs\n"), 0o644))
	run(t, repo, "add", "docs.md")
	run(t, repo, "commit", "-m", "docs(TOPTIER-S1-T3): update documentation")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "chore.txt"), []byte("chore\n"), 0o644))
	run(t, repo, "add", "chore.txt")
	run(t, repo, "commit", "-m", "chore(TOPTIER-S1-T3): update dependencies")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "breaking.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "breaking.go")
	run(t, repo, "commit", "-m", "feat(TOPTIER-S1-T3)!: breaking change")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "other.go")
	run(t, repo, "commit", "-m", "feat(OTHER-ISSUE): unrelated feature")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "untracked.go")
	run(t, repo, "commit", "-m", "some random commit without issue")

	git := adapters.New(repo)

	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err, "ReviewCommits should succeed")

	assert.Equal(t, 6, len(commits), "should find exactly 6 commits for TOPTIER-S1-T3")

	subjects := make(map[string]bool)
	for _, commit := range commits {
		subjects[commit.Subject] = true
	}

	assert.True(t, subjects["feat(TOPTIER-S1-T3): add feature"], "feat commit should be found")
	assert.True(t, subjects["fix(TOPTIER-S1-T3): fix issue"], "fix commit should be found")
	assert.True(t, subjects["refactor(TOPTIER-S1-T3): refactor module"], "refactor commit should be found")
	assert.True(t, subjects["test(TOPTIER-S1-T3): add tests"], "test commit should be found")
	assert.True(t, subjects["docs(TOPTIER-S1-T3): update documentation"], "docs commit should be found")
	assert.False(t, subjects["chore(TOPTIER-S1-T3): update dependencies"], "chore is not a valid conventions.md type and should not be found")
	assert.True(t, subjects["feat(TOPTIER-S1-T3)!: breaking change"], "breaking-change (feat(ID)!:) commit should be found")

	assert.False(t, subjects["feat(OTHER-ISSUE): unrelated feature"], "commits for other issues should not be included")
	assert.False(t, subjects["some random commit without issue"], "commits without issue ID should not be included")
}

func TestReviewCommits_EmptyRepo(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	git := adapters.New(repo)

	commits, err := ReviewCommits(git, "NONEXISTENT-ISSUE", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "should return empty slice for issue with no commits")
}

func TestReviewCommits_RejectsOptionLikeBranch(t *testing.T) {
	t.Parallel()
	git := adapters.New(t.TempDir())
	_, err := ReviewCommits(git, "TOPTIER-S1-T3", "--all")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid branch")
}

func TestReviewCommits_RejectsDisallowedType(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "maintenance.auto", "false")
	run(t, repo, "config", "gc.auto", "0")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "oops.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "oops.go")
	run(t, repo, "commit", "-m", "oops(TOPTIER-S1-T3): bypass convention")

	git := adapters.New(repo)
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "commit with disallowed type should not be matched")
}

func TestReviewCommits_PartialMatchIgnored(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "file.go")
	run(t, repo, "commit", "-m", "feat(TOPTIER-S1): work on TOPTIER-S1-T3 but not scoped")

	git := adapters.New(repo)

	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "should not match when issue ID is in body but not scope")
}

func TestReviewCommits_MultilineMessage(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "file.go")

	msg := "feat(TOPTIER-S1-T3): add feature\n\nThis is a longer description\nof the change being made."
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "-c", "commit.gpgsign=false", "commit", "-m", msg)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit with multiline message failed: %s", out)

	git := adapters.New(repo)

	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 1, len(commits), "should find commit with multiline message")
	assert.Contains(t, commits[0].Subject, "feat(TOPTIER-S1-T3): add feature")
}

func TestReviewCommits_IncludesMergeCommitFormat_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "maintenance.auto", "false")
	run(t, repo, "config", "gc.auto", "0")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	initialBranchOut := runOutput(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "feature.go")
	run(t, repo, "commit", "-m", "some feature work")
	run(t, repo, "checkout", "-b", "feature-branch")
	run(t, repo, "commit", "--allow-empty", "-m", "more feature work")

	run(t, repo, "checkout", "-b", "integration-branch", initialBranchOut)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	run(t, repo, "add", "other.go")
	run(t, repo, "commit", "-m", "unrelated integration commit")
	run(t, repo, "merge", "--no-ff", "feature-branch", "-m", "merge: TOPTIER-S1-T3 integrate feature branch")

	git := adapters.New(repo)
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "integration-branch")
	require.NoError(t, err)
	require.NotEmpty(t, commits, "issue delivered via a merge commit should be discoverable")

	found := false
	for _, c := range commits {
		if c.Subject == "merge: TOPTIER-S1-T3 integrate feature branch" {
			found = true
		}
	}
	assert.True(t, found, "merge: ID description commit should be included")
}

func TestReviewCommits_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "init")
	run(t, repo, "config", "maintenance.auto", "false")
	run(t, repo, "config", "gc.auto", "0")
	run(t, repo, "config", "user.email", "test@example.com")
	run(t, repo, "config", "user.name", "Test User")
	run(t, repo, "config", "commit.gpgsign", "false")
	run(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0o644))
	run(t, repo, "add", "file.txt")
	run(t, repo, "commit", "-m", "merge: TOPTIER-S1-T3 integrate feature work")

	git := adapters.New(repo)
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	for _, c := range commits {
		assert.NotEqual(t, "merge: TOPTIER-S1-T3 integrate feature work", c.Subject,
			"merge: ID subject on a single-parent (non-merge) commit must not be discoverable")
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command git %v failed: %s", args, out)
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	require.NoError(t, err, "command git %v failed", args)
	return strings.TrimSpace(string(out))
}
