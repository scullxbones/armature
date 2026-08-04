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

	// Verify all expected commits are found. chore is intentionally excluded:
	// it is not one of the commit types docs/conventions.md documents (feat,
	// fix, refactor, test, docs, style, polish), so ReviewCommits' pattern is
	// restricted to that allow-list — the same fix applied to
	// internal/deliverygate/gate.go's CommitReferenceCheck, which previously
	// used an unrestricted ^[a-z]+ that would also match a bogus type.
	assert.Equal(t, 6, len(commits), "should find exactly 6 commits for TOPTIER-S1-T3")

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
	assert.False(t, subjects["chore(TOPTIER-S1-T3): update dependencies"], "chore is not a valid conventions.md type and should not be found")
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

// TestReviewCommits_RejectsDisallowedType verifies that a commit type outside
// the repo's documented convention (feat, fix, refactor, test, docs, style,
// polish — see docs/conventions.md) is not matched, even though it matches
// "some lowercase word" followed by (ISSUE-ID):. Same bug class as the
// overly-permissive ^[a-z]+ pattern fixed in
// internal/deliverygate/gate.go's CommitReferenceCheck.
func TestReviewCommits_RejectsDisallowedType(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "maintenance.auto", "false")
	run(t, repo, "git", "config", "gc.auto", "0")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "oops.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "oops.go")
	run(t, repo, "git", "commit", "-m", "oops(TOPTIER-S1-T3): bypass convention")

	git := adapters.New(repo)
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, 0, len(commits), "commit with disallowed type should not be matched")
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

// TestReviewCommits_IncludesMergeCommitFormat_REQ_LNGHZN_S4 fixes the open
// PR review comment on commits.go:36: ReviewCommits' pattern only matched
// the typed `type(ID): ...` form and silently excluded the documented
// `merge: ID description` form that the delivery gate's CommitReferenceCheck
// already accepts (see docs/conventions.md). An issue delivered solely via a
// merge commit must still show up in `arm review commits <ID>` discovery.
func TestReviewCommits_IncludesMergeCommitFormat_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "maintenance.auto", "false")
	run(t, repo, "git", "config", "gc.auto", "0")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	initialBranchOut := runOutput(t, repo, "git", "rev-parse", "--abbrev-ref", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "feature.go")
	run(t, repo, "git", "commit", "-m", "some feature work")
	run(t, repo, "git", "checkout", "-b", "feature-branch")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "more feature work")

	run(t, repo, "git", "checkout", "-b", "integration-branch", initialBranchOut)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "other.go"), []byte("package main\n"), 0o644))
	run(t, repo, "git", "add", "other.go")
	run(t, repo, "git", "commit", "-m", "unrelated integration commit")
	run(t, repo, "git", "merge", "--no-ff", "feature-branch", "-m", "merge: TOPTIER-S1-T3 integrate feature branch")

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

// TestReviewCommits_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4 mirrors
// internal/deliverygate/gate_test.go's
// TestCommitReferenceCheck_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4:
// the merge: ID description subject form must only be recognized on a
// genuine merge commit (2+ parents). A holistic branch review found that the
// multi-parent guard added to CommitReferenceCheck was never wired into
// ReviewCommits, so an ordinary single-parent commit whose author merely
// wrote a subject that looks like the merge form was still discoverable
// here — exactly the bug class the guard was supposed to eliminate
// everywhere.
func TestReviewCommits_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	repo := tmpDir

	run(t, repo, "git", "init")
	run(t, repo, "git", "config", "maintenance.auto", "false")
	run(t, repo, "git", "config", "gc.auto", "0")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test User")
	run(t, repo, "git", "config", "commit.gpgsign", "false")
	run(t, repo, "git", "commit", "--allow-empty", "-m", "initial commit")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "file.txt"), []byte("content"), 0o644))
	run(t, repo, "git", "add", "file.txt")
	run(t, repo, "git", "commit", "-m", "merge: TOPTIER-S1-T3 integrate feature work")

	git := adapters.New(repo)
	commits, err := ReviewCommits(git, "TOPTIER-S1-T3", "HEAD")
	require.NoError(t, err)
	for _, c := range commits {
		assert.NotEqual(t, "merge: TOPTIER-S1-T3 integrate feature work", c.Subject,
			"merge: ID subject on a single-parent (non-merge) commit must not be discoverable")
	}
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

// runOutput runs a git command and returns its trimmed stdout.
func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	require.NoError(t, err, "command %s %v failed", name, args)
	return strings.TrimSpace(string(out))
}
