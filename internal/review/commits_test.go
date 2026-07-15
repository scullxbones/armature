package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewCommits_REQ_TOPTIER-S1-T3 verifies that ReviewCommits discovers
// delivery commits for an issue across all conventional-commit type prefixes
// (feat, fix, refactor, test, docs, chore), not just feat.
// This requirement replaces the coordinator skill's feat-only grep pseudocode
// which silently dropped other types.
func TestReviewCommits_REQ_TOPTIER_S1_T3(t *testing.T) {
	t.Parallel()
	// Setup: create a temporary repo
	tmpDir := t.TempDir()
	repo := tmpDir

	// Initialize git repo
	run(t, repo, "git", "init")
	// Background Git maintenance can keep writing pack files after git commit
	// exits, racing t.TempDir cleanup. This ephemeral repository needs neither
	// automatic maintenance nor garbage collection.
	run(t, repo, "git", "config", "maintenance.auto", "false")
	run(t, repo, "git", "config", "gc.auto", "0")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	// Create a commit with feat prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "feat.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "feat.go")
	run(t, repo, "git", "commit", "-m", "feat(TOPTIER-S1-T3): add feature")

	// Create a commit with fix prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "fix.go")
	run(t, repo, "git", "commit", "-m", "fix(TOPTIER-S1-T3): fix issue")

	// Create a commit with refactor prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "refactor.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "refactor.go")
	run(t, repo, "git", "commit", "-m", "refactor(TOPTIER-S1-T3): refactor module")

	// Create a commit with test prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "test.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "test.go")
	run(t, repo, "git", "commit", "-m", "test(TOPTIER-S1-T3): add tests")

	// Create a commit with docs prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs.md"), []byte("# Docs\n"), 0o644))
	run(t, repo, "git", "add", "docs.md")
	run(t, repo, "git", "commit", "-m", "docs(TOPTIER-S1-T3): update documentation")

	// Create a commit with chore prefix for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "chore.txt"), []byte("chore\n"), 0o644))
	run(t, repo, "git", "add", "chore.txt")
	run(t, repo, "git", "commit", "-m", "chore(TOPTIER-S1-T3): update dependencies")

	// Create a commit using the breaking-change syntax `feat(ISSUE-ID)!: ...`
	// for TOPTIER-S1-T3
	require.NoError(t, os.WriteFile(filepath.Join(repo, "breaking.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "breaking.go")
	run(t, repo, "git", "commit", "-m", "feat(TOPTIER-S1-T3)!: breaking change")

	// Create commits for a different issue (should not be included)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "other.go")
	run(t, repo, "git", "commit", "-m", "feat(OTHER-ISSUE): unrelated feature")

	// Create a commit without any issue ID (should not be included)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "untracked.go")
	run(t, repo, "git", "commit", "-m", "some random commit without issue")

	// Create git adapter
	git := adapters.New(repo)

	// Call ReviewCommits
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err, "ReviewCommits should succeed")

	// Verify all expected commits are found
	assert.Equal(t, 7, len(commits), "should find exactly 7 commits for TOPTIER-S1-T3")

	// Extract subjects for verification
	subjects := make(map[string]bool)
	for _, commit := range commits {
		subjects[commit.Subject] = true
	}

	// Verify each type is present
	assert.True(t, subjects["feat(TOPTIER-S1-T3): add feature"], "feat commit should be found")
	assert.True(t, subjects["fix(TOPTIER-S1-T3): fix issue"], "fix commit should be found")
	assert.True(t, subjects["refactor(TOPTIER-S1-T3): refactor module"], "refactor commit should be found")
	assert.True(t, subjects["test(TOPTIER-S1-T3): add tests"], "test commit should be found")
	assert.True(t, subjects["docs(TOPTIER-S1-T3): update documentation"], "docs commit should be found")
	assert.True(t, subjects["chore(TOPTIER-S1-T3): update dependencies"], "chore commit should be found")
	assert.True(t, subjects["feat(TOPTIER-S1-T3)!: breaking change"], "breaking-change (feat(ID)!:) commit should be found")

	// Verify that commits for other issues are not included
	assert.False(t, subjects["feat(OTHER-ISSUE): unrelated feature"], "commits for other issues should not be included")
	assert.False(t, subjects["some random commit without issue"], "commits without issue ID should not be included")
}

// TestReviewCommits_EmptyRepo verifies that ReviewCommits returns empty slice
// when there are no commits for the issue.
func TestReviewCommits_EmptyRepo(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

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

// TestReviewCommits_PartialMatchIgnored verifies that ReviewCommits only matches
// issue IDs when they are the complete scope identifier in the commit message,
// not as a substring within other text.
func TestReviewCommits_PartialMatchIgnored(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	// Create commit that mentions the issue ID in a partial way (not as the scope)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "file.go")
	run(t, repo, "git", "commit", "-m", "feat(TOPTIER-S1): work on TOPTIER-S1-T3 but not scoped")

	git := adapters.New(repo)

	// Should not match because TOPTIER-S1-T3 is not the scope, even though it's mentioned
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "should not match when issue ID is in body but not scope")
}

// TestReviewCommits_MultilineMessage verifies that ReviewCommits correctly
// parses commits with multiline messages (title + body).
func TestReviewCommits_MultilineMessage(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	// Create commit with multiline message
	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "file.go")
	// Create a commit with a multiline message
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

// Helper function to run git commands
//
//nolint:unparam // name is "git" in all current callers but helper is intentionally general
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %s %v failed: %s", name, args, out)
}
