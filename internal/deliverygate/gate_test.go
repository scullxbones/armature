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
// commit in the range does not satisfy the check. Before the fix,
// CommitReferenceCheck only checked that the matching commit itself had
// nonempty CommitChangedFiles, without confirming the change survived into
// the net base-to-HEAD diff, so a matching commit immediately followed by a
// revert would wrongly pass despite zero net delivery.
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

// TestCommitReferenceCheck_RejectsPaddedSelfCancellingRevert verifies that a
// matching conventional commit whose change is fully reverted cannot be
// smuggled past the check by padding the range with a later, unrelated,
// trivial in-scope commit. Before this fix, CommitReferenceCheck only
// confirmed that the net base-to-HEAD diff was nonempty for ANY reason,
// without checking that the net diff overlaps with the files the matching
// commit itself touched — so an unrelated filler commit could keep the net
// diff nonempty even though the matching commit's own substance was fully
// reverted.
func TestCommitReferenceCheck_RejectsPaddedSelfCancellingRevert(t *testing.T) {
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

	// C3: unrelated trivial edit to another in-scope file, keeping the net
	// diff nonempty despite C1's substance being fully reverted.
	require.NoError(t, os.WriteFile(other, []byte("base other\n// trivial comment"), 0644))
	runGit(t, tmpDir, "add", "other.txt")
	runGit(t, tmpDir, "commit", "-m", "add trivial comment")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a matching commit whose change is reverted must not be satisfied by an unrelated padding commit")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsSameFileContentRevert verifies that a
// matching conventional commit whose added content is fully undone cannot be
// smuggled past the check merely because the SAME file it touched still
// shows up in the net base..HEAD diff (due to an unrelated trivial edit to
// that same file). Before this fix, CommitReferenceCheck only checked that
// the matching commit's touched FILENAMES overlapped with the net diff's
// changed filenames -- not that any of the matching commit's actual added
// content survives in the net diff. A later commit can revert the matching
// commit's real change and pad the same file with an unrelated trivial edit,
// keeping the filename in the net diff while none of the delivered content
// survives.
func TestCommitReferenceCheck_RejectsSameFileContentRevert(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	widget := filepath.Join(tmpDir, "widget.txt")
	require.NoError(t, os.WriteFile(widget, []byte("base widget\n"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// C1: matching format, adds real delivered content to widget.txt.
	require.NoError(t, os.WriteFile(widget, []byte("base widget\nDELIVERED FEATURE\n"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement widget")

	// C2: removes C1's delivered line and adds an unrelated trivial line to
	// the SAME file, so widget.txt still appears in the net base..HEAD diff
	// even though none of C1's actual content survives.
	require.NoError(t, os.WriteFile(widget, []byte("base widget\ntrailing padding\n"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "swap in padding")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass,
		"a matching commit whose delivered content is undone must not pass merely because its filename still appears in the net diff")
	assert.NotEmpty(t, result.Remediation)
}

// TestCommitReferenceCheck_RejectsShortCoincidentalLineMatch_REQ_LNGHZN_S4 verifies
// that a matching commit whose real (long, distinctive) added content is fully
// reverted cannot be smuggled past the survival check merely because a SHORT,
// common line it also happened to add (e.g. a lone "}") coincidentally
// reappears as an added line elsewhere in the net diff for the same file, for
// entirely unrelated reasons. Before this fix, the survival check treated ANY
// single non-blank added-line match — however short and generic — as proof of
// survival, so two coincidentally identical short lines in unrelated commits
// could produce a false "survives" even though the matching commit's actual
// distinctive content was completely undone.
func TestCommitReferenceCheck_RejectsShortCoincidentalLineMatch_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	widget := filepath.Join(tmpDir, "widget.go")
	require.NoError(t, os.WriteFile(widget, []byte("package widget\n"), 0644))
	runGit(t, tmpDir, "add", "widget.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	// C1: matching format, adds a distinctive delivered line plus a short,
	// generic closing-brace line.
	require.NoError(t, os.WriteFile(widget, []byte(
		"package widget\n\nfunc New() {\n\tdoDistinctiveDeliveredWork()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "widget.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement widget")

	// C2: fully reverts C1's change back to base content.
	require.NoError(t, os.WriteFile(widget, []byte("package widget\n"), 0644))
	runGit(t, tmpDir, "add", "widget.go")
	runGit(t, tmpDir, "commit", "-m", "revert the widget change")

	// C3: an unrelated later change to the SAME file that happens to add a
	// short, common line ("}") for reasons that have nothing to do with C1's
	// reverted content.
	require.NoError(t, os.WriteFile(widget, []byte(
		"package widget\n\nfunc Unrelated() {\n\tdoOtherThing()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "widget.go")
	runGit(t, tmpDir, "commit", "-m", "add unrelated function")

	result := CommitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass,
		"a coincidental short-line match (e.g. a lone \"}\") must not count as survival of the matching commit's reverted content")
	assert.NotEmpty(t, result.Remediation)
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
