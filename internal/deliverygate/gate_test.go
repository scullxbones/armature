package deliverygate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanTreeCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	cleanFile := filepath.Join(tmpDir, "clean.txt")
	require.NoError(t, os.WriteFile(cleanFile, []byte("clean content"), 0644))
	runGit(t, tmpDir, "add", "clean.txt")
	runGit(t, tmpDir, "commit", "-m", "initial commit")

	result := cleanTreeCheck(tmpDir)
	assert.True(t, result.Pass, "clean tree should pass")
	assert.Empty(t, result.Remediation, "clean tree should have no remediation")

	require.NoError(t, os.WriteFile(cleanFile, []byte("modified content"), 0644))

	result = cleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "dirty tree should fail")
	assert.NotEmpty(t, result.Remediation, "dirty tree should have remediation message")
}

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

	result := cleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "rename from outside .armature/ into .armature/ must not be filtered out")
	assert.Contains(t, result.Remediation, "outside.go")
}

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

	result := cleanTreeCheck(tmpDir)
	assert.False(t, result.Pass, "ignored build artifacts must fail the clean-tree gate")
	assert.Contains(t, result.Remediation, "bin/")
	assert.Contains(t, result.Remediation, "coverage.out")
	assert.NotContains(t, result.Remediation, ".armature/")
}

func TestScopeContainmentCheck_AllFilesWithinScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file1, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): modify file1")

	result := scopeContainmentCheck(tmpDir, baseCommit, "HEAD", []string{"pkg/**"})
	assert.True(t, result.Pass, "all files within scope should pass")
	assert.Empty(t, result.Remediation)
}

func TestScopeContainmentCheck_FileOutsideScope_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file1 := filepath.Join(tmpDir, "pkg", "file1.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file1), 0755))
	require.NoError(t, os.WriteFile(file1, []byte("package pkg"), 0644))
	runGit(t, tmpDir, "add", "pkg/file1.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	file2 := filepath.Join(tmpDir, "cmd", "main.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file2), 0755))
	require.NoError(t, os.WriteFile(file2, []byte("package main"), 0644))
	runGit(t, tmpDir, "add", "cmd/main.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST): add main")

	result := scopeContainmentCheck(tmpDir, baseCommit, "HEAD", []string{"pkg/**"})
	assert.False(t, result.Pass, "file outside scope should fail")
	assert.NotEmpty(t, result.Remediation)
	assert.Contains(t, result.Remediation, "cmd/main.go")
}

func TestScopeContainmentCheck_RenameFromOutOfScopeToInScope_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	outsideFile := filepath.Join(tmpDir, "outside", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideFile), 0755))
	// Content needs enough bulk for git's rename heuristic to recognize the
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

	result := scopeContainmentCheck(tmpDir, baseCommit, "HEAD", []string{"inside/**"})
	assert.False(t, result.Pass, "rename from out-of-scope path should fail scope containment")
	assert.Contains(t, result.Remediation, "outside/a.go")
}

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

	result := scopeContainmentCheck(tmpDir, baseCommit, "HEAD", []string{"pkg/**"})
	assert.True(t, result.Pass, "rename fully within scope should pass")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_ValidConventionalCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
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
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): add feature")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "valid conventional commit should pass")
	assert.Empty(t, result.Remediation)
}

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

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "bare subject with no description should fail")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_NoMatchingCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
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
	runGit(t, tmpDir, "commit", "-m", "feat: generic feature")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "commits without issue ID should fail")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_AcceptsPrimaryBranchWhenWorktreeStale_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "main")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")
	require.NoError(t, os.WriteFile(file, []byte("stale task tip"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "wip: not a conventional reference")

	runGit(t, tmpDir, "checkout", "main")
	require.NoError(t, os.WriteFile(file, []byte("squash landed"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): land squash (#217)")

	runGit(t, tmpDir, "checkout", "task/TEST-123")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "matching conventional commit on main must satisfy CommitReference when the worktree is still on the stale task branch")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_StaleWorktreeStillFailsWithoutPrimaryEvidence_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "main")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")
	require.NoError(t, os.WriteFile(file, []byte("stale task tip"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "wip: not a conventional reference")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "stale worktree with no matching commit on main must still fail")
	assert.NotEmpty(t, result.Remediation)
}

func TestDeliveryGate_OutOfScopeSquashOnMainFailsWhenWorktreeStale_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	inFile := filepath.Join(tmpDir, "pkg", "in.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(inFile), 0755))
	require.NoError(t, os.WriteFile(inFile, []byte("package pkg\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/in.go")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "main")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")

	runGit(t, tmpDir, "checkout", "main")
	outFile := filepath.Join(tmpDir, "cmd", "out.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outFile), 0755))
	require.NoError(t, os.WriteFile(outFile, []byte("package main\n"), 0644))
	runGit(t, tmpDir, "add", "cmd/out.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): land squash (#217)")

	runGit(t, tmpDir, "checkout", "task/TEST-123")

	gate := DeliveryGate(tmpDir, "TEST-123", baseCommit, []string{"pkg/**"})
	assert.True(t, gate.CleanTree.Pass, "claim worktree is clean")
	assert.True(t, gate.CommitReference.Pass, "matching squash on main must still satisfy CommitReference")
	assert.False(t, gate.ScopeContainment.Pass, "out-of-scope squash on main must fail scope even when worktree HEAD is empty")
	assert.Contains(t, gate.ScopeContainment.Remediation, "cmd/out.go")
}

func TestDeliveryGate_InterveningOutOfScopePrimaryCommitDoesNotBlockInScopeSquash_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	inFile := filepath.Join(tmpDir, "pkg", "in.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(inFile), 0755))
	require.NoError(t, os.WriteFile(inFile, []byte("package pkg\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/in.go")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "main")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")
	stale := filepath.Join(tmpDir, "pkg", "stale.go")
	require.NoError(t, os.WriteFile(stale, []byte("package pkg\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/stale.go")
	runGit(t, tmpDir, "commit", "-m", "wip: not a conventional reference")

	runGit(t, tmpDir, "checkout", "main")
	require.NoError(t, os.WriteFile(inFile, []byte("package pkg\n\nfunc In() {}\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/in.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): land squash")

	outFile := filepath.Join(tmpDir, "cmd", "unrelated.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outFile), 0755))
	require.NoError(t, os.WriteFile(outFile, []byte("package main\n"), 0644))
	runGit(t, tmpDir, "add", "cmd/unrelated.go")
	runGit(t, tmpDir, "commit", "-m", "feat(OTHER-1): unrelated landing")

	runGit(t, tmpDir, "checkout", "task/TEST-123")

	gate := DeliveryGate(tmpDir, "TEST-123", baseCommit, []string{"pkg/**"})
	assert.True(t, gate.CleanTree.Pass, "claim worktree is clean")
	assert.True(t, gate.CommitReference.Pass, "in-scope squash on main must satisfy CommitReference")
	assert.True(t, gate.ScopeContainment.Pass, "later unrelated out-of-scope commit on main must not fail this task's scope")
	assert.Empty(t, gate.ScopeContainment.Remediation)
}

func TestDeliveryGate_MultiCommitMergeLandingScopesAllCommits_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	inFile := filepath.Join(tmpDir, "pkg", "in.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(inFile), 0755))
	require.NoError(t, os.WriteFile(inFile, []byte("package pkg\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/in.go")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "main")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")
	stale := filepath.Join(tmpDir, "pkg", "stale.go")
	require.NoError(t, os.WriteFile(stale, []byte("package pkg\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/stale.go")
	runGit(t, tmpDir, "commit", "-m", "wip: not a conventional reference")

	runGit(t, tmpDir, "checkout", "main")
	runGit(t, tmpDir, "checkout", "-b", "feat/TEST-123")
	outFile := filepath.Join(tmpDir, "cmd", "out.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(outFile), 0755))
	require.NoError(t, os.WriteFile(outFile, []byte("package main\n"), 0644))
	runGit(t, tmpDir, "add", "cmd/out.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): out of scope change")
	require.NoError(t, os.WriteFile(inFile, []byte("package pkg\n\nfunc In() {}\n"), 0644))
	runGit(t, tmpDir, "add", "pkg/in.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): in scope change")

	runGit(t, tmpDir, "checkout", "main")
	runGit(t, tmpDir, "merge", "--no-ff", "feat/TEST-123", "-m", "Merge branch 'feat/TEST-123'")

	runGit(t, tmpDir, "checkout", "task/TEST-123")

	gate := DeliveryGate(tmpDir, "TEST-123", baseCommit, []string{"pkg/**"})
	assert.True(t, gate.CleanTree.Pass, "claim worktree is clean")
	assert.True(t, gate.CommitReference.Pass, "matching commits in the merge landing must satisfy CommitReference")
	assert.False(t, gate.ScopeContainment.Pass, "complete merge landing must include the older out-of-scope path")
	assert.Contains(t, gate.ScopeContainment.Remediation, "cmd/out.go")
}

func TestCommitReferenceCheck_SkipsStaleMainWhenEvidenceIsOnMaster_REQ_MATENC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")
	runGit(t, tmpDir, "branch", "-M", "master")
	baseCommit := getHeadSHA(t, tmpDir)
	runGit(t, tmpDir, "branch", "main")

	runGit(t, tmpDir, "checkout", "-b", "task/TEST-123")

	runGit(t, tmpDir, "checkout", "master")
	require.NoError(t, os.WriteFile(file, []byte("squash landed"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): land squash on master")

	runGit(t, tmpDir, "checkout", "task/TEST-123")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "matching conventional commit on master must satisfy CommitReference when local main is stale and empty")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_RejectsDisallowedType_REQ_LNGHZN_S4_T1(t *testing.T) {
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
	runGit(t, tmpDir, "commit", "-m", "oops(TEST-123): bypass convention")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "commit with disallowed type should fail")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_AcceptsMergeCommitFormat_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "checkout", "-b", "feature-branch")
	featureFile := filepath.Join(tmpDir, "feature.txt")
	require.NoError(t, os.WriteFile(featureFile, []byte("feature content"), 0644))
	runGit(t, tmpDir, "add", "feature.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): feature work")

	runGit(t, tmpDir, "checkout", "-b", "integration-branch", baseCommit)
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "unrelated integration commit")
	runGit(t, tmpDir, "merge", "--no-ff", "feature-branch", "-m", "merge: TEST-123 integrate feature work")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "documented merge: ID description format should be accepted on a genuine merge commit")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_RejectsMergeFormOnSingleParentCommit_REQ_LNGHZN_S4(t *testing.T) {
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

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "merge: ID subject on a single-parent (non-merge) commit must be rejected")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_IgnoresMatchBeforeBase_REQ_LNGHZN_S4_T2(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("v0"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): earlier work")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("v1"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "no reference in this one")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a match before base must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_RejectsEmptyCommit_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "commit", "--allow-empty", "-m", "fix(TEST-123): busywork")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "an empty commit must not satisfy the commit reference check")
	assert.NotEmpty(t, result.Remediation)
}

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

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a later real-content matching commit should still pass")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_RejectsSelfCancellingRevert_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("base content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("changed content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): make a change")

	require.NoError(t, os.WriteFile(file, []byte("base content"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "revert the change")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a matching commit whose change is fully reverted must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

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

	require.NoError(t, os.WriteFile(widget, []byte("changed widget"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement widget")

	require.NoError(t, os.WriteFile(widget, []byte("base widget"), 0644))
	runGit(t, tmpDir, "add", "widget.txt")
	runGit(t, tmpDir, "commit", "-m", "revert the widget change")

	require.NoError(t, os.WriteFile(other, []byte("base other\n// trivial comment"), 0644))
	runGit(t, tmpDir, "add", "other.txt")
	runGit(t, tmpDir, "commit", "-m", "add trivial comment")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a matching commit reference plus a non-empty net diff from elsewhere in the range satisfies the check")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_DeletionOnlyCommitSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale line")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a deletion-only commit whose deletion is never undone must satisfy the check")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_RejectsRevertedDeletionOnlyCommit_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale line")

	require.NoError(t, os.WriteFile(file, []byte("keep this line\nDELETE THIS LONG DISTINCTIVE LINE HERE\n"), 0644))
	runGit(t, tmpDir, "add", "file.txt")
	runGit(t, tmpDir, "commit", "-m", "restore the line")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.False(t, result.Pass, "a deletion-only commit whose deletion is later undone must not satisfy the check")
	assert.NotEmpty(t, result.Remediation)
}

func TestCommitReferenceCheck_CosmeticReformattingByLaterCommitStillSatisfies_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "file.go")
	require.NoError(t, os.WriteFile(file, []byte("package p\n"), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(file, []byte(
		"package p\n\nfunc New() {\n    return doDistinctiveDeliveredWork()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): implement New")

	require.NoError(t, os.WriteFile(file, []byte(
		"package p\n\nfunc New() {\n\treturn doDistinctiveDeliveredWork()\n}\n",
	), 0644))
	runGit(t, tmpDir, "add", "file.go")
	runGit(t, tmpDir, "commit", "-m", "gofmt cleanup")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check even after a cosmetic reformat")
	assert.Empty(t, result.Remediation)
}

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

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a matching commit adding a non-ASCII-named file must be recognized as surviving")
	assert.Empty(t, result.Remediation)
}

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

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a content-preserving (pure) rename must be recognized as a surviving delivered change")
	assert.Empty(t, result.Remediation)
}

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

				require.NoError(t, os.WriteFile(file, []byte("keep this line\nADDED REPLACEMENT DELIVERY LINE HERE\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): replace stale line")

				require.NoError(t, os.WriteFile(file, []byte("keep this line\nSOME OTHER UNRELATED LINE ENTIRELY\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "edit away the added line")
				return base
			},
		},
		{
			name:         "commit whose every added line is short still finds survival evidence",
			expectedPass: true,
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file.go")
				require.NoError(t, os.WriteFile(file, []byte("package p\n"), 0644))
				runGit(t, dir, "add", "file.go")
				runGit(t, dir, "commit", "-m", "base")
				base := getHeadSHA(t, dir)

				require.NoError(t, os.WriteFile(file, []byte("package p\nvar X = 1\n"), 0644))
				runGit(t, dir, "add", "file.go")
				runGit(t, dir, "commit", "-m", "feat(TEST-123): add short var")
				return base
			},
		},
		{
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

				require.NoError(t, os.WriteFile(file, []byte("keep this line\n"), 0644))
				runGit(t, dir, "add", "file.txt")
				runGit(t, dir, "commit", "-m", "fix(TEST-123): remove stale lines")

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

			_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
			assert.Equal(t, tc.expectedPass, result.Pass, "case %q", tc.name)
			if tc.expectedPass {
				assert.Empty(t, result.Remediation)
			} else {
				assert.NotEmpty(t, result.Remediation)
			}
		})
	}
}

func TestDeliveryGate_IntegrationCheck_REQ_LNGHZN_S4_T1(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	scopedFile := filepath.Join(tmpDir, "pkg", "file.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(scopedFile), 0755))
	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 1"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(scopedFile, []byte("package pkg\nvar X = 2"), 0644))
	runGit(t, tmpDir, "add", "pkg/file.go")
	runGit(t, tmpDir, "commit", "-m", "feat(ISSUE-001): valid change")

	gate := DeliveryGate(tmpDir, "ISSUE-001", baseCommit, []string{"pkg/**"})

	assert.True(t, gate.CleanTree.Pass, "tree is clean after commit")
	assert.True(t, gate.ScopeContainment.Pass, "all files within scope")
	assert.True(t, gate.CommitReference.Pass, "commit has proper format")
}

func TestCommitReferenceCheck_WholeFileDeletionSurvives(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	file := filepath.Join(tmpDir, "gone.txt")
	require.NoError(t, os.WriteFile(file, []byte("first removed line\nsecond removed line\n"), 0644))
	runGit(t, tmpDir, "add", "gone.txt")
	runGit(t, tmpDir, "commit", "-m", "base")

	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "rm", "gone.txt")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale file")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass, "a whole-file deletion whose deletion is never undone must satisfy the check")
	assert.Empty(t, result.Remediation)
}

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

	destPath := filepath.Join(tmpDir, "dest.txt")
	require.NoError(t, os.WriteFile(destPath, []byte(
		"line one\nline two replaced\nline three\n"), 0644))
	runGit(t, tmpDir, "add", "-A")
	runGit(t, tmpDir, "commit", "-m", "feat(TEST-123): copy and adapt")

	require.NoError(t, os.WriteFile(destPath, []byte("totally different content now\n"), 0644))
	runGit(t, tmpDir, "add", "dest.txt")
	runGit(t, tmpDir, "commit", "-m", "unrelated: further edit dest")

	runGit(t, tmpDir, "rm", "source.txt")
	runGit(t, tmpDir, "commit", "-m", "unrelated: remove source file")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check regardless of copy/rename source-path bookkeeping")
	assert.Empty(t, result.Remediation)
}

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

	runGit(t, tmpDir, "checkout", "-")
	runGit(t, tmpDir, "merge", "--no-ff", "-m", "fix(TEST-123): merge feature", "feature-branch")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a merge commit whose subject matches and whose first-parent diff carries real content must be recognized as delivering")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_BinaryFileFurtherModifiedStillSatisfiesNetDiffCheck_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	binFile := filepath.Join(tmpDir, "asset.bin")
	require.NoError(t, os.WriteFile(binFile, []byte("\x00AAAA_UNIQUE_LINE_ONE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	require.NoError(t, os.WriteFile(binFile, []byte("\x00BBBB_DIFFERENT_LINE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): update binary asset")

	require.NoError(t, os.WriteFile(binFile, []byte("\x00CCCC_FINAL_LINE\ntrailing\n"), 0644))
	runGit(t, tmpDir, "add", "asset.bin")
	runGit(t, tmpDir, "commit", "-m", "unrelated: replace binary asset")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit reference plus a non-empty net diff satisfies the check for binary content too")
	assert.Empty(t, result.Remediation)
}

func TestCommitReferenceCheck_BinaryFileDeletionSurvives_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	binFile := filepath.Join(tmpDir, "stale-asset.bin")
	require.NoError(t, os.WriteFile(binFile, []byte("\x00STALE_BINARY_CONTENT\n"), 0644))
	runGit(t, tmpDir, "add", "stale-asset.bin")
	runGit(t, tmpDir, "commit", "-m", "base")
	baseCommit := getHeadSHA(t, tmpDir)

	runGit(t, tmpDir, "rm", "stale-asset.bin")
	runGit(t, tmpDir, "commit", "-m", "fix(TEST-123): remove stale binary asset")

	_, result := commitReferenceCheck(tmpDir, baseCommit, "TEST-123")
	assert.True(t, result.Pass,
		"a matching commit that deletes a binary file, with the deletion never undone, must satisfy the check")
	assert.Empty(t, result.Remediation)
}

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
	return strings.TrimSpace(string(output))
}
