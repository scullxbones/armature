package deliverygate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanTreeCheck_REQ_LNGHZN_S4_T1 verifies that the clean tree check
// correctly detects when a worktree has no uncommitted changes.
func TestCleanTreeCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	// Create a temporary git repository
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create and commit a file
	cleanFile := filepath.Join(tmpDir, "clean.txt")
	require.NoError(t, os.WriteFile(cleanFile, []byte("clean content"), 0644))
	runGit(t, tmpDir, "add", "clean.txt")
	runGit(t, tmpDir, "commit", "-m", "initial commit")

	// Test with a clean tree
	result := CleanTreeCheck(tmpDir)
	assert.True(t, result.Pass, "clean tree should pass")
	assert.Empty(t, result.Remediation, "clean tree should have no remediation")

	// Add an uncommitted change
	require.NoError(t, os.WriteFile(cleanFile, []byte("modified content"), 0644))

	// Test with dirty tree
	result = CleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "dirty tree should fail")
	assert.NotEmpty(t, result.Remediation, "dirty tree should have remediation message")
}

// TestCleanTreeCheck_RenameFromOutsideToArmatureDir_REQ_LNGHZN_S4_T1 verifies that a
// staged rename moving a tracked file from outside .armature/ into
// .armature/ is NOT filtered out by the .armature/ noise exclusion: the
// source path being outside .armature/ means a real tracked file was
// effectively deleted, so the tree must be reported as dirty.
func TestCleanTreeCheck_RenameFromOutsideToArmatureDir_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	outsideFile := filepath.Join(tmpDir, "outside.go")
	require.NoError(t, os.WriteFile(outsideFile, []byte("package main"), 0644))
	runGit(t, tmpDir, "add", "outside.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".armature"), 0755))
	runGit(t, tmpDir, "mv", "outside.go", ".armature/outside.go")

	result := CleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "rename from outside .armature/ into .armature/ must not be filtered out")
	assert.Contains(t, result.Remediation, "outside.go")
}

// TestCleanTreeCheck_IgnoredBuildArtifactsFailButArmatureStateIsExempt_REQ_LNGHZN_S4_T1
// verifies that ignored generated artifacts still make a delivery tree dirty,
// while Armature's derived coordination state remains safe local noise.
func TestCleanTreeCheck_IgnoredBuildArtifactsFailButArmatureStateIsExempt_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("bin/\ncoverage.out\n.armature/\n"), 0o644))
	runGit(t, tmpDir, "add", ".gitignore")
	runGit(t, tmpDir, "commit", "-m", "base")

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "bin", "arm"), []byte("binary"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "coverage.out"), []byte("coverage"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".armature", "state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".armature", "state", "index.json"), []byte("{}"), 0o644))

	result := CleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "ignored build artifacts must fail the clean-tree gate")
	assert.Contains(t, result.Remediation, "bin/")
	assert.Contains(t, result.Remediation, "coverage.out")
	assert.NotContains(t, result.Remediation, ".armature/")
}

// TestScopeContainmentCheck_AllFilesWithinScope_REQ_LNGHZN_S4_T1 verifies that
// the scope containment check passes when all changed files are within scope.
func TestScopeContainmentCheck_AllFilesWithinScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a change within scope
	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): modify file1")

	// Test scope containment
	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"pkg/**"})
	assert.True(t, result.Pass, "all files within scope should pass")
	assert.Empty(t, result.Remediation)
}

// TestScopeContainmentCheck_FileOutsideScope_REQ_LNGHZN_S4_T1 verifies that
// the scope containment check fails when a changed file is outside the declared scope.
func TestScopeContainmentCheck_FileOutsideScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create base commit
	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a change outside declared scope
	file2 := filepath.Join(tmpDir, "cmd", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file2), 0755))
	require.NoError(t, os.WriteFile(file2, []byte("package main"), 0644))
	runGit(t, tmpDir, "add", "cmd/main.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): add main")

	// Test with scope that excludes the new file
	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"pkg/**"})
	assert.False(t, result.Pass, "file outside scope should fail")
	assert.NotEmpty(t, result.Remediation)
	assert.Contains(t, result.Remediation, "cmd/main.go")
}

// TestScopeContainmentCheck_RenameFromOutOfScopeToInScope_REQ_LNGHZN_S4 verifies
// that a rename whose original path was outside the declared scope fails
// scope containment, even though the destination path is in scope. Plain
// `git diff --name-only` collapses a rename to only its destination path,
// which would otherwise mask an out-of-scope deletion via rename.
func TestScopeContainmentCheck_RenameFromOutOfScopeToInScope_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	outsideFile := filepath.Join(tmpDir, "outside", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideFile), 0755))
	// Content needs enough bulk for git's rename heuristic to recognize the
	// move rather than reporting a plain delete+add.
	content := ""
	for range 20 {
		content += "line of content\n"
	}
	require.NoError(t, os.WriteFile(outsideFile, []byte(content), 0644))
	runGit(t, tmpDir, "add", "outside/a.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	insideFile := filepath.Join(tmpDir, "inside", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(insideFile), 0755))
	runGit(t, tmpDir, "mv", "outside/a.go", "inside/a.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): rename outside to inside")

	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"inside/**"})
	assert.False(t, result.Pass, "rename from out-of-scope path should fail scope containment")
	assert.Contains(t, result.Remediation, "outside/a.go")
}

// TestScopeContainmentCheck_RenameFullyWithinScope_REQ_LNGHZN_S4 verifies that
// a rename whose source and destination are both within scope passes.
func TestScopeContainmentCheck_RenameFullyWithinScope_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	oldFile := filepath.Join(tmpDir, "pkg", "old.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldFile), 0755))
	content := ""
	for range 20 {
		content += "line of content\n"
	}
	require.NoError(t, os.WriteFile(oldFile, []byte(content), 0644))
	runGit(t, tmpDir, "add", "pkg/old.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "mv", "pkg/old.go", "pkg/new.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): rename within scope")

	result := ScopeContainmentCheck(tmpDir, baseCommit, []string{"pkg/**"})
	assert.True(t, result.Pass, "rename fully within scope should pass")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_ValidConventionalCommit_REQ_LNGHZN_S4_T1 verifies that
// the commit reference check passes when at least one commit has the proper
// conventional-commit format with the issue ID in the scope.
func TestCommitReferenceCheck_ValidConventionalCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a commit with proper conventional format
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): add feature")

	// Test commit reference
	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "valid conventional commit should pass")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsBareSubjectWithNoDescription verifies that
// a commit subject matching "fix(ISSUE-ID):" with no description after the
// colon does not satisfy the check: the regex must require a non-empty
// description, not just the type/issue-ID/colon prefix.
func TestCommitReferenceCheck_RejectsBareSubjectWithNoDescription(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123):")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "bare subject with no description should fail")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_NoMatchingCommit_REQ_LNGHZN_S4_T1 verifies that
// the commit reference check fails when no commits match the conventional format.
func TestCommitReferenceCheck_NoMatchingCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a commit WITHOUT proper conventional format (no issue ID in scope)
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat: generic feature")

	// Test commit reference - should fail
	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "commits without issue ID should fail")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsDisallowedType_REQ_LNGHZN_S4_T1 verifies that
// a commit type outside the repo's documented convention (feat, fix, refactor,
// test, docs, style, polish — see docs/conventions.md) does not satisfy the
// check, even though it matches "some lowercase word" followed by (ISSUE-ID):.
func TestCommitReferenceCheck_RejectsDisallowedType_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a base commit
	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a commit using a bogus, non-conventional type.
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "oops(TEST-123): bypass convention")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "commit with disallowed type should fail")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_AcceptsMergeCommitFormat_REQ_LNGHZN_S4 verifies
// that the documented merge-commit format ("merge: <ISSUE-ID> <description>",
// per docs/conventions.md) satisfies the commit-reference check even though
// it doesn't follow the type(ISSUE-ID): description shape used by other
// commit types.
func TestCommitReferenceCheck_AcceptsMergeCommitFormat_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "merge: TEST-123 integrate feature work")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "documented merge: ID description format should be accepted")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_IgnoresMatchBeforeBase_REQ_LNGHZN_S4_T2 verifies
// that a conventional-commit reference committed BEFORE baseCommit does not
// satisfy the check — only commits strictly after base count.
func TestCommitReferenceCheck_IgnoresMatchBeforeBase_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("v0"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	// A matching reference lands BEFORE the base commit — e.g. an older,
	// already-merged commit for the same issue ID.
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): earlier work")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("v1"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "no reference in this one")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a match before base must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsEmptyCommit_REQ_LNGHZN_S4_T1 verifies the P3 fix: a
// commit that matches the conventional-commit format but has no actual diff
// (e.g. `git commit --allow-empty -m "fix(ISSUE-ID): busywork"`) must not
// satisfy the check. Before the fix, CommitReferenceCheck only inspected
// commit subject lines, so an empty commit with the right message shape would
// wrongly pass the gate with no real content delivered.
func TestCommitReferenceCheck_RejectsEmptyCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// A conventional-commit-shaped but content-free commit.
	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "fix(TEST-123): busywork")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "an empty commit must not satisfy the commit reference check")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_AcceptsMatchingCommitAmongEmptyOnes_REQ_LNGHZN_S4_T1 verifies
// that the empty-commit tightening doesn't reject an issue whose FIRST
// matching commit happens to be empty but a LATER matching commit has real
// content — the check should keep looking rather than stop at the first
// subject-line match.
func TestCommitReferenceCheck_AcceptsMatchingCommitAmongEmptyOnes_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "fix(TEST-123): busywork")

	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): real fix")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a later real-content matching commit should still pass")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsSelfCancellingRevert_REQ_LNGHZN_S4_T1 verifies that a
// matching conventional commit whose change is fully reverted by a later
// commit in the range, with nothing else delivered, does not satisfy the
// check: the net base..HEAD diff is empty, so there is no evidence anything
// was actually delivered.
func TestCommitReferenceCheck_RejectsSelfCancellingRevert_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Commit A: matching format, changes a real file.
	require.NoError(t, os.WriteFile(file, []byte("changed content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): make a change")

	// Commit B: reverts A's change back to the base content, so the net
	// base-to-HEAD diff is empty.
	require.NoError(t, os.WriteFile(file, []byte("base content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "revert the change")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a matching commit whose change is fully reverted must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_AcceptsPaddedSelfCancellingRevert_REQ_LNGHZN_S4 documents
// an intentional design trade-off: a matching conventional commit whose own
// change is fully reverted, but the range also contains an unrelated later
// commit that delivers something else, now PASSES. The new two-part check
// (conventional-commit reference exists AND net base..HEAD diff is
// non-empty) deliberately does not attribute the surviving net diff back to
// the specific matching commit — that per-commit content-survival
// reconstruction is what caused repeated edge-case bugs (deletions, binary
// files, renames, mixed add/delete, merges) across prior implementations.
// The abuse case this check exists to prevent (a `type(ID): busywork`
// commit reverted with NOTHING else delivered) is still caught, because in
// that case the net diff is empty.
func TestCommitReferenceCheck_AcceptsPaddedSelfCancellingRevert_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	widget := filepath.Join(tmpDir, "widget.txt")
	other := filepath.Join(tmpDir, "other.txt")
	require.NoError(t, os.WriteFile(widget, []byte("base widget"), 0644))
	require.NoError(t, os.WriteFile(other, []byte("base other"), 0644))
	runGit(t, tmpDir, "add", "widget.txt", "other.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// C1: matching format, changes widget.txt.
	require.NoError(t, os.WriteFile(widget, []byte("changed widget"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement widget")

	// C2: reverts C1's change back to base content.
	require.NoError(t, os.WriteFile(widget, []byte("base widget"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "revert the widget change")

	// C3: an unrelated edit to another in-scope file — net delivery from
	// somewhere in the range, even though C1's own substance was reverted.
	require.NoError(t, os.WriteFile(other, []byte("base other\n// trivial comment"), 0644))
	runGit(t, tmpDir, "add", "other.txt")
	runGit(t, tmpDir, "commit", "-m", "add trivial comment")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a matching commit reference plus a non-empty net diff from elsewhere in the range satisfies the check")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_DeletionOnlyCommitSurvives_REQ_LNGHZN_S4 verifies
// that a matching conventional commit whose only change is removing lines
// (no lines added) is treated as delivered when that deletion is never
// undone. Before this fix, the survival check was based entirely on
// addedLinesByFile, which a pure deletion never populates, so a legitimate
// deletion-only delivery was wrongly rejected as "not surviving".
func TestCommitReferenceCheck_DeletionOnlyCommitSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit removes the distinctive line and adds nothing.
	require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale line")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a deletion-only commit whose deletion is never undone must satisfy the check")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsRevertedDeletionOnlyCommit_REQ_LNGHZN_S4
// verifies that a matching deletion-only commit does NOT satisfy the check
// when a later commit re-adds the removed content, undoing the deletion.
func TestCommitReferenceCheck_RejectsRevertedDeletionOnlyCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit removes the distinctive line and adds nothing.
	require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale line")

	// Later commit re-adds the removed content, undoing the deletion.
	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "restore the line")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a deletion-only commit whose deletion is later undone must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_CosmeticReformattingByLaterCommitStillSatisfies_REQ_LNGHZN_S4
// verifies that a later commit which only cosmetically reformats a
// previously delivered line (e.g. a gofmt-style indentation rewrap) does not
// break the check: the net base..HEAD diff is still non-empty relative to
// base, and a matching conventional-commit reference exists somewhere in the
// range, so the two-part check passes without needing to attribute the
// reformatted content back to the original matching commit specifically.
func TestCommitReferenceCheck_CosmeticReformattingByLaterCommitStillSatisfies_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.go")
	require.NoError(t, os.WriteFile(file, []byte("package p\n"), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit adds a distinctive delivered line, indented with
	// spaces.
	require.NoError(t, os.WriteFile(file, []byte(
		"package p\n\nfunc New() {\n    return doDistinctiveDeliveredWork()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement New")

	// Later commit only cosmetically reformats the delivered line's
	// indentation (spaces -> tab, gofmt-style), without changing its actual
	// tokens/content or the file's line structure.
	require.NoError(t, os.WriteFile(file, []byte(
		"package p\n\nfunc New() {\n\treturn doDistinctiveDeliveredWork()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "gofmt cleanup")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check even after a cosmetic reformat")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_NonASCIIFilenameSurvives_REQ_LNGHZN_S4 verifies
// the fix for a review finding against the prior diff-text-based survival
// check: a matching commit that adds content to a file with a non-ASCII
// name (which git quotes and octal-escapes in default diff/log text output,
// e.g. "caf\303\251.go" for "café.go") must still be recognized as
// surviving. Blob-OID comparison sidesteps this entirely: paths come from
// CommitDiffTreeStatus's -z (NUL-delimited, unquoted) output and are passed
// as literal arguments to git plumbing, with no diff-header text to decode.
func TestCommitReferenceCheck_NonASCIIFilenameSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base content\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	nonASCIIFile := filepath.Join(tmpDir, "café.go")
	require.NoError(t, os.WriteFile(nonASCIIFile, []byte("package main\n\nfunc café() {}\n"), 0644))
	runGit(t, tmpDir, "add", "café.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): add café helper")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a matching commit adding a non-ASCII-named file must be recognized as surviving")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_ContentPreservingRenameSurvives_REQ_LNGHZN_S4
// verifies the fix for a review finding against the prior diff-text-based
// survival check: a matching commit that is a pure (100%-similarity) rename
// with no textual hunk (git represents it as a rename with zero added/
// removed lines) must still be recognized as delivering a real change.
// Blob-OID comparison handles this automatically: the commit's post-image
// blob OID at the destination path is unchanged from the pre-rename blob,
// so comparing it against HEAD's blob OID at the destination path correctly
// recognizes the rename as surviving with no hunk-parsing involved.
func TestCommitReferenceCheck_ContentPreservingRenameSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	oldFile := filepath.Join(tmpDir, "oldname.txt")
	content := ""
	for range 20 {
		content += "line of content\n"
	}
	require.NoError(t, os.WriteFile(oldFile, []byte(content), 0644))
	runGit(t, tmpDir, "add", "oldname.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "mv", "oldname.txt", "newname.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): rename to newname")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a content-preserving (pure) rename must be recognized as a surviving delivered change")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_SurvivalMatrix_REQ_LNGHZN_S4 is a table-driven
// suite enumerating outcomes of CommitReferenceCheck's two-part design
// (conventional-commit reference exists AND the net base..HEAD diff is
// non-empty) across add/delete/modify/rename shapes, with and without a
// later commit undoing the change. Earlier designs tried to prove the
// matching commit's OWN content specifically survives to HEAD and were
// redefined multiple times across review rounds (filename-overlap-only ->
// same-file added-line-multiset intersection -> blob-OID comparison), each
// round's fix closing one counterexample while reopening another for a
// different diff shape (deletions, binary files, renames, merges). The
// two-part check sidesteps that entirely: it doesn't attribute the diff to
// any specific commit, only requires that a matching reference exists AND
// something was net-delivered. This suite pins down that every case below
// still lands on the intuitively-correct verdict under the simpler design.
func TestCommitReferenceCheck_SurvivalMatrix_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		expectedPass bool
		setup        func(t *testing.T, dir string) (baseCommit string)
	}{
		{
			name:         "added-only content still present at HEAD survives",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("base content\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("base content\nDISTINCTIVE ADDED DELIVERY LINE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "feat(TEST-123): add content")
				return base
			},
		},
		{
			name:         "added-only content later fully reverted does not survive",
			expectedPass: false,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("base content\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("base content\nDISTINCTIVE ADDED DELIVERY LINE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "feat(TEST-123): add content")

				require.NoError(t, os.WriteFile(file, []byte("base content\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "revert the addition")
				return base
			},
		},
		{
			name:         "deletion-only commit whose deletion persists survives",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): remove stale line")
				return base
			},
		},
		{
			name:         "deletion-only commit whose deletion is later reverted does not survive",
			expectedPass: false,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): remove stale line")

				require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "restore the line")
				return base
			},
		},
		{
			name:         "modify (remove+add on same lines) whose net change persists survives",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("original distinctive line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("replacement distinctive delivered line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): replace line")
				return base
			},
		},
		{
			name:         "modify later fully reverted back to pre-commit content does not survive",
			expectedPass: false,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("original distinctive line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("replacement distinctive delivered line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): replace line")

				require.NoError(t, os.WriteFile(file, []byte("original distinctive line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "revert the replacement")
				return base
			},
		},
		{
			name:         "rename with content change whose content persists at new path survives",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				oldFile := filepath.Join(dir, "oldname.txt")
				newFile := filepath.Join(dir, "newname.txt")
				content := "line one\nline two\nline three\nline four\n"
				require.NoError(t, os.WriteFile(oldFile, []byte(content), 0644))
				runGit(t, dir, "add", "oldname.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				runGit(t, dir, "mv", "oldname.txt", "newname.txt")
				require.NoError(t, os.WriteFile(newFile, []byte(content+"DISTINCTIVE RENAMED DELIVERY LINE\n"), 0644))
				runGit(t, dir, "add", "-A")
				runGit(t, dir, "commit", "-m", "feat(TEST-123): rename with content change")
				return base
			},
		},
		{
			name:         "mixed add+delete in same file whose added content is edited away but original deletion still holds survives",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				// Matching commit both removes the stale line and adds new
				// content to the same file (a refactor-shaped change).
				require.NoError(t, os.WriteFile(file, []byte("keep this line\nADDED REPLACEMENT DELIVERY LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): replace stale line")

				// Later commit edits away the added content (so
				// addedContentSurvives will return false for this file),
				// but does not restore the originally deleted line — the
				// matching commit's deletion still holds at HEAD.
				require.NoError(t, os.WriteFile(file, []byte("keep this line\nSOME OTHER UNRELATED LINE ENTIRELY\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "edit away the added line")
				return base
			},
		},
		{
			// Regression for PR #88 review finding: a commit whose every
			// added line is shorter than survivalMinLineLength (15 chars)
			// used to blanket-skip ALL of its lines whenever it added more
			// than one, leaving no evidence to check and permanently
			// rejecting a commit that legitimately delivers only short
			// lines.
			name:         "commit whose every added line is short still finds survival evidence",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.go")
				require.NoError(t, os.WriteFile(file, []byte("package p\n"), 0644))
				runGit(t, dir, "add", "file.go")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				// Both added lines are short (< 15 chars).
				require.NoError(t, os.WriteFile(file, []byte("package p\nvar X = 1\n"), 0644))
				runGit(t, dir, "add", "file.go")
				runGit(t, dir, "commit", "-m", "feat(TEST-123): add short var")
				return base
			},
		},
		{
			// Regression for PR #88 review finding: deletionSurvives used to
			// require ALL removed lines to remain absent from HEAD, so a
			// commit that removed several lines but had just one of them
			// coincidentally restored elsewhere (e.g. re-added by an
			// unrelated later commit for an unrelated reason) was rejected
			// wholesale, even though most of its removed content is
			// genuinely gone and the deletion substantively still holds.
			name:         "deletion with multiple removed lines survives if at least one remains absent",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte(
					"keep this line\nDELETE THIS FIRST DISTINCTIVE LINE\nDELETE THIS SECOND DISTINCTIVE LINE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				// Matching commit removes both distinctive lines and adds nothing.
				require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): remove stale lines")

				// A later, unrelated commit coincidentally reintroduces only
				// ONE of the two removed lines (e.g. an unrelated doc note
				// with the same text), not both.
				require.NoError(t, os.WriteFile(file, []byte(
					"keep this line\nDELETE THIS FIRST DISTINCTIVE LINE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "unrelated: coincidental reintroduction of one line")
				return base
			},
		},
		{
			name:         "empty/no-op commit does not satisfy the check (existing gate.go behavior)",
			expectedPass: false,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.txt")
				require.NoError(t, os.WriteFile(file, []byte("content\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				runGit(t, dir, "commit", "--allow-empty", "-m", "fix(TEST-123): busywork")
				return base
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			initGitRepo(t, tmpDir)
			baseCommit := tc.setup(t, tmpDir)

			result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
			assert.Equal(t, tc.expectedPass, result.Pass, "case %q", tc.name)
			if tc.expectedPass {
				assert.Empty(t, result.Remediation)
			} else {
				assert.NotEmpty(t, result.Remediation)
			}
		})
	}
}

// TestDeliveryGate_IntegrationCheck_REQ_LNGHZN_S4_T1 verifies that the
// DeliveryGate function returns correct combined results for all three checks.
func TestDeliveryGate_IntegrationCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Set up initial commit with scoped file
	scopedFile := filepath.Join(tmpDir, "pkg", "file.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopedFile), 0755))
	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Add a valid change with proper commit message
	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "feat(ISSUE-001): valid change")

	// Test with valid gate parameters
	gate := DeliveryGate(tmpDir, "ISSUE-001", baseCommit, []string{"pkg/**"})

	assert.True(t, gate.CleanTree.Pass, "tree is clean after commit")
	assert.True(t, gate.ScopeContainment.Pass, "all files within scope")
	assert.True(t, gate.CommitReference.Pass, "commit has proper format")
}

// TestCommitReferenceCheck_WholeFileDeletionSurvives verifies end to end that
// a matching commit which deletes an entire file (rather than removing some
// lines from a surviving file) is correctly recognized as delivering
// non-trivial, undone content when the deletion is never reverted.
func TestCommitReferenceCheck_WholeFileDeletionSurvives(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "gone.txt")
	require.NoError(t, os.WriteFile(file, []byte("first removed line\nsecond removed line\n"), 0644))
	runGit(t, tmpDir, "add", "gone.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit deletes the entire file.
	runGit(t, tmpDir, "rm", "gone.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale file")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a whole-file deletion whose deletion is never undone must satisfy the check")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_CopySourceLaterDeletedStillSatisfiesNetDiffCheck_REQ_LNGHZN_S4
// exercises a copy-with-edits followed by unrelated further edits and an
// unrelated deletion of the copy's source path. Under the two-part check,
// this passes: a matching conventional-commit reference exists, and the net
// base..HEAD diff is non-empty (dest.txt's final content differs from base,
// and source.txt was removed). The check does not attempt to prove that the
// matching commit's OWN destination-path content specifically survived —
// that per-commit attribution is exactly the class of logic (with its own
// copy-vs-rename OldPath edge cases) this design intentionally avoids.
func TestCommitReferenceCheck_CopySourceLaterDeletedStillSatisfiesNetDiffCheck_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, os.WriteFile(sourcePath, []byte(
		"line one\nDISTINCTIVE LINE TO BE EDITED AWAY\nline three\n"), 0644))
	runGit(t, tmpDir, "add", "source.txt")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit copies source.txt to dest.txt, with edits (so git
	// reports it as a copy "C..." with OldPath=source.txt, Path=dest.txt,
	// rather than a 100%-similarity copy).
	destPath := filepath.Join(tmpDir, "dest.txt")
	require.NoError(t, os.WriteFile(destPath, []byte(
		"line one\nline two replaced\nline three\n"), 0644))
	runGit(t, tmpDir, "add", "-A")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): copy and adapt")

	// Unrelated later commit overwrites dest.txt entirely, so the matching
	// commit's own post-image blob at dest.txt no longer matches HEAD (the
	// exact blob-OID check must fail, forcing the fallback path).
	require.NoError(t, os.WriteFile(destPath, []byte("totally different content now\n"), 0644))
	runGit(t, tmpDir, "add", "dest.txt")
	runGit(t, tmpDir, "commit", "-m", "unrelated: further edit dest")

	// Unrelated later commit deletes the copy's untouched source path.
	runGit(t, tmpDir, "rm", "source.txt")
	runGit(t, tmpDir, "commit", "-m", "unrelated: remove source file")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check regardless of copy/rename source-path bookkeeping")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_MergeCommitWithMatchingSubjectSurvives_REQ_LNGHZN_S4
// verifies a merge commit whose subject matches the conventional-commit
// format still satisfies the check when it legitimately merges in real
// content. LogRange (which feeds the commit-subject scan) uses plain
// `base..head` log semantics, which is NOT first-parent-only, so a merge
// commit can appear in the scanned range; under the two-part design this is
// unproblematic regardless of the merge commit's own diff-tree output, since
// the second half of the check reads the net base..HEAD diff directly
// rather than any specific commit's per-commit diff.
func TestCommitReferenceCheck_MergeCommitWithMatchingSubjectSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	baseFile := filepath.Join(tmpDir, "base.txt")
	require.NoError(t, os.WriteFile(baseFile, []byte("base content\n"), 0644))
	runGit(t, tmpDir, "add", "base.txt")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "feature-branch")
	featureFile := filepath.Join(tmpDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("DISTINCTIVE FEATURE CONTENT\n"), 0644))
	runGit(t, tmpDir, "add", "feature.txt")
	runGit(t, tmpDir, "commit", "-m", "add feature file")

	// Back to the branch containing baseCommit, then merge with a commit
	// subject matching the conventional-commit format.
	runGit(t, tmpDir, "checkout", "-")
	runGit(t, tmpDir, "merge", "--no-ff", "-m", "fix(TEST-123): merge feature", "feature-branch")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a merge commit whose subject matches and whose first-parent diff carries real content must be recognized as delivering")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_BinaryFileFurtherModifiedStillSatisfiesNetDiffCheck_REQ_LNGHZN_S4
// exercises a binary file modified by the matching commit and then further
// modified by an unrelated later commit. Under the two-part check this
// passes: a matching conventional-commit reference exists, and the net
// base..HEAD diff is non-empty (the binary asset's final content differs
// from base). No per-file-shape content-survival heuristic (which for prior
// implementations meant splitting binary blob bytes on '\n' — meaningless
// for binary content) is needed to reach that verdict.
func TestCommitReferenceCheck_BinaryFileFurtherModifiedStillSatisfiesNetDiffCheck_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	binFile := filepath.Join(tmpDir, "asset.bin")
	require.NoError(t, os.WriteFile(binFile, []byte("\x00AAAA_UNIQUE_LINE_ONE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit modifies the binary asset.
	require.NoError(t, os.WriteFile(binFile, []byte("\x00BBBB_DIFFERENT_LINE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): update binary asset")

	// Unrelated later commit replaces the binary asset with entirely
	// different content.
	require.NoError(t, os.WriteFile(binFile, []byte("\x00CCCC_FINAL_LINE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "unrelated: replace binary asset")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check for binary content too")
	assert.Empty(t, result.Remediation)
}

// TestCommitReferenceCheck_BinaryFileDeletionSurvives_REQ_LNGHZN_S4 is a
// regression test for review comment 3695646511 (gate.go, prior blob-OID
// implementation): a matching commit that deletes a binary file was NOT
// recognized as delivering content, because the deletion-survival fallback
// (removedLinesBetweenBlobs) refused to compute "removed lines" for a binary
// pre-image, and a deletion's Status ("D...") skips the exact blob-OID
// comparison entirely — so a binary-file deletion had NO surviving-evidence
// path at all and always caused the gate to reject a legitimate,
// never-undone deletion. The two-part check has no such gap: the binary
// file's absence from the tree is just another entry in the base..HEAD diff,
// so no per-file-shape (text vs. binary) special-casing is needed.
func TestCommitReferenceCheck_BinaryFileDeletionSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	binFile := filepath.Join(tmpDir, "stale-asset.bin")
	require.NoError(t, os.WriteFile(binFile, []byte("\x00STALE_BINARY_CONTENT\n"), 0644))
	runGit(t, tmpDir, "add", "stale-asset.bin")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	// Matching commit deletes the binary file outright, and the deletion is
	// never reverted.
	runGit(t, tmpDir, "rm", "stale-asset.bin")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale binary asset")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit that deletes a binary file, with the deletion never undone, must satisfy the check")
	assert.Empty(t, result.Remediation)
}

// Helper functions

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	// Disable commit signing to avoid GPG issues in tests
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func getHeadSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	require.NoError(t, err, "git rev-parse HEAD failed")
	return string(output[:len(output)-1]) // trim newline
}
