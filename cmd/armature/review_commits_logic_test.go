package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListReviewCommits_REQ_TOPTIER_S1_T3(t *testing.T) {
	t.Parallel()

	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "feat.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "feat.go")
	gittest.Git(t, repo, "commit", "-m", "feat(TOPTIER-S1-T3): add feature")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "fix.go")
	gittest.Git(t, repo, "commit", "-m", "fix(TOPTIER-S1-T3): fix issue")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "refactor.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "refactor.go")
	gittest.Git(t, repo, "commit", "-m", "refactor(TOPTIER-S1-T3): refactor module")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "test.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "test.go")
	gittest.Git(t, repo, "commit", "-m", "test(TOPTIER-S1-T3): add tests")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs.md"), []byte("# Docs\n"), 0o644))
	gittest.Git(t, repo, "add", "docs.md")
	gittest.Git(t, repo, "commit", "-m", "docs(TOPTIER-S1-T3): update documentation")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "chore.txt"), []byte("chore\n"), 0o644))
	gittest.Git(t, repo, "add", "chore.txt")
	gittest.Git(t, repo, "commit", "-m", "chore(TOPTIER-S1-T3): update dependencies")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "breaking.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "breaking.go")
	gittest.Git(t, repo, "commit", "-m", "feat(TOPTIER-S1-T3)!: breaking change")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "other.go")
	gittest.Git(t, repo, "commit", "-m", "feat(OTHER-ISSUE): unrelated feature")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "untracked.go")
	gittest.Git(t, repo, "commit", "-m", "some random commit without issue")

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)

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

func TestListReviewCommits_EmptyRepo(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	commits, err := listReviewCommits(adapters.New(repo), "NONEXISTENT-ISSUE", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "should return empty slice for issue with no commits")
}

func TestListReviewCommits_RejectsOptionLikeBranch(t *testing.T) {
	t.Parallel()
	_, err := listReviewCommits(adapters.New(t.TempDir()), "TOPTIER-S1-T3", "--all")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid branch")
}

func TestListReviewCommits_RejectsDisallowedType(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "oops.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "oops.go")
	gittest.Git(t, repo, "commit", "-m", "oops(TOPTIER-S1-T3): bypass convention")

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "commit with disallowed type should not be matched")
}

func TestListReviewCommits_PartialMatchIgnored(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "file.go")
	gittest.Git(t, repo, "commit", "-m", "feat(TOPTIER-S1): work on TOPTIER-S1-T3 but not scoped")

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "should not match when issue ID is in body but not scope")
}

func TestListReviewCommits_MultilineMessage(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "file.go")

	msg := "feat(TOPTIER-S1-T3): add feature\n\nThis is a longer description\nof the change being made."
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "-c", "commit.gpgsign=false", "commit", "-m", msg)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit with multiline message failed: %s", out)

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 1, len(commits), "should find commit with multiline message")
	assert.Contains(t, commits[0].Subject, "feat(TOPTIER-S1-T3): add feature")
}

func TestListReviewCommits_IncludesMergeCommitFormat_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	abbrev := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	abbrev.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	abbrevOut, err := abbrev.Output()
	require.NoError(t, err)
	initialBranch := strings.TrimSpace(string(abbrevOut))

	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "feature.go")
	gittest.Git(t, repo, "commit", "-m", "some feature work")
	gittest.Git(t, repo, "checkout", "-b", "feature-branch")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "more feature work")

	gittest.Git(t, repo, "checkout", "-b", "integration-branch", initialBranch)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	gittest.Git(t, repo, "add", "other.go")
	gittest.Git(t, repo, "commit", "-m", "unrelated integration commit")
	gittest.Git(t, repo, "merge", "--no-ff", "feature-branch", "-m", "merge: TOPTIER-S1-T3 integrate feature branch")

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "integration-branch")
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

func TestListReviewCommits_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0o644))
	gittest.Git(t, repo, "add", "file.txt")
	gittest.Git(t, repo, "commit", "-m", "merge: TOPTIER-S1-T3 integrate feature work")

	commits, err := listReviewCommits(adapters.New(repo), "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	for _, c := range commits {
		assert.NotEqual(t, "merge: TOPTIER-S1-T3 integrate feature work", c.Subject,
			"merge: ID subject on a single-parent (non-merge) commit must not be discoverable")
	}
}
