package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGateRunRecordsEvidence_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	head := gitRevParse(t, repo)

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, "full", ev.Profile)
	assert.Equal(t, []string{"true"}, ev.Command)
	assert.Equal(t, head, ev.HeadSHA)
	assert.Equal(t, 0, ev.Exit)
	assert.False(t, ev.Uncommitted, "clean tree must be citable")
	assert.GreaterOrEqual(t, ev.End, ev.Start)
	assert.NotZero(t, ev.Start)
}

func TestGateRunRecordsOutputDigest_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"echo", "hello-gate"},
	})

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.NotEmpty(t, ev.OutputHash)
	assert.NotEmpty(t, ev.LogPath)
	raw, err := os.ReadFile(ev.LogPath)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	assert.Equal(t, hex.EncodeToString(sum[:]), ev.OutputHash)
	assert.Contains(t, ev.OutputHead, "hello-gate")
}

func TestGateRunDirtyTreeUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("uncited"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err, "dirty tree still executes the configured command")

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "dirty tree must record the run as uncommitted")
	assert.Equal(t, 0, ev.Exit)
	assert.Equal(t, "full", ev.Profile)
}

func TestGateRunUnconfiguredRepoErrors_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no gates configured")
}

func TestGateRunUnknownProfileErrors(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	_, err := runTrls(t, repo, "gate", "run", "fast")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGateRunEmptyCommandErrors(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {},
	})
	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command")
}

func TestGateRunFailedCommandRecordsEvidence(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"false"},
	})
	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	ev := requireOneGateEvidence(t, repo)
	assert.NotEqual(t, 0, ev.Exit)
	assert.Equal(t, "full", ev.Profile)
}

// TestGateRunUsesInvokingWorktree_REQ_LNGHZN_S10_T3 is the P1 seam: when
// invoked from a linked task worktree, HEAD, cleanliness, and the configured
// command must use that checkout — not the parent repo ResolveContext walks to.
func TestGateRunUsesInvokingWorktree_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"test", "-f", "wt-only"},
	})
	parentHEAD := gitRevParse(t, repo)

	worktreeDir := t.TempDir()
	run(t, repo, "git", "worktree", "add", worktreeDir, "-b", "task/task-01")
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "wt-only"), []byte("marker"), 0o644))
	run(t, worktreeDir, "git", "add", "wt-only")
	run(t, worktreeDir, "git", "commit", "-m", "feat(task-01): worktree-only marker")
	worktreeHEAD := gitRevParse(t, worktreeDir)
	require.NotEqual(t, parentHEAD, worktreeHEAD, "fixture must diverge parent and worktree HEAD")

	_, err := runTrls(t, worktreeDir, "gate", "run", "full")
	require.NoError(t, err, "command must run in the worktree where wt-only exists")

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, worktreeHEAD, ev.HeadSHA, "evidence must record the invoking worktree HEAD, not the parent")
	assert.NotEqual(t, parentHEAD, ev.HeadSHA)
	assert.Equal(t, 0, ev.Exit)
	assert.False(t, ev.Uncommitted)
}

// TestGateRunDirtiedTreeUncitable_REQ_LNGHZN_S10_T3 covers a clean checkout
// whose configured command leaves a non-ignored file: the run must still be
// recorded uncommitted so evidence cannot cite a tree that no longer matches HEAD.
func TestGateRunDirtiedTreeUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"touch", "generated-by-gate.txt"},
	})

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err, "the configured command itself should succeed")

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "a gate that dirties the tree must not be citable")
	assert.Equal(t, 0, ev.Exit)
	assert.FileExists(t, filepath.Join(repo, "generated-by-gate.txt"))
}

// TestGateRunUncommittedGatesJSON_REQ_LNGHZN_S10_T3 is the I5 seam: a
// gates.json edit that is not in HEAD must dirty the tree so the run cannot
// be cited as evidence for the committed definition.
func TestGateRunUncommittedGatesJSON_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	writeGatesFile(t, repo, map[string][]string{
		"full": {"true", "and-changed"},
	})

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "uncommitted gates.json change must record uncommitted=true")
	assert.Equal(t, []string{"true"}, ev.Command, "must execute the HEAD command, not the worktree edit")
}

// TestGateRunSkipWorktreeGatesJSONUsesHEADCommand_REQ_LNGHZN_S10_T3 is the
// I5 seam: skip-worktree hides a mutated gates.json from porcelain. The
// wrapper must still execute the HEAD blob command, not the worktree file.
func TestGateRunSkipWorktreeGatesJSONUsesHEADCommand_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"false"},
	})
	writeGatesFile(t, repo, map[string][]string{
		"full": {"true"},
	})
	run(t, repo, "git", "update-index", "--skip-worktree", "gates.json")

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err, "HEAD command is false; worktree true must not be executed")

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, []string{"false"}, ev.Command)
	assert.NotEqual(t, 0, ev.Exit)
	assert.True(t, ev.Uncommitted, "skip-worktree on gates.json must record uncommitted")
}

func TestGateRunSkipWorktreeSourceUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	run(t, repo, "git", "add", "src.txt")
	run(t, repo, "git", "commit", "-m", "add src")
	run(t, repo, "git", "update-index", "--skip-worktree", "src.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "skip-worktree source mutation must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunAssumeUnchangedSourceUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	run(t, repo, "git", "add", "src.txt")
	run(t, repo, "git", "commit", "-m", "add src")
	run(t, repo, "git", "update-index", "--assume-unchanged", "src.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "assume-unchanged source mutation must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

// TestGateRunUntrackedGatesJSONIsUnconfigured_REQ_LNGHZN_S10_T3: a
// worktree-only gates.json is not in HEAD, so the repo is unconfigured.
func TestGateRunUntrackedGatesJSONIsUnconfigured_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesFile(t, repo, map[string][]string{
		"full": {"true"},
	})

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no gates configured")
}

// TestGateRunReadsInvokingWorktreeGatesJSON_REQ_LNGHZN_S10_T3 is the I5
// seam: arm gate run must load the tracked gates.json in the invoking
// checkout, not the parent's .armature/config.json Gates map.
func TestGateRunReadsInvokingWorktreeGatesJSON_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"false"},
	})
	writeArmatureConfigGates(t, repo, map[string][]string{
		"full": {"false"},
	})

	worktreeDir := t.TempDir()
	run(t, repo, "git", "worktree", "add", worktreeDir, "-b", "task/task-01")
	writeGatesConfig(t, worktreeDir, map[string][]string{
		"full": {"true"},
	})

	_, err := runTrls(t, worktreeDir, "gate", "run", "full")
	require.NoError(t, err, "command must come from the worktree gates.json, not parent config")

	ev := requireOneGateEvidence(t, repo)
	assert.Equal(t, []string{"true"}, ev.Command)
	assert.Equal(t, 0, ev.Exit)
}

// TestGateRunHEADMoveRecordsUncommitted_REQ_LNGHZN_S10_T3 covers a command
// that advances HEAD (empty commit). HeadSHA stays the pre-command revision
// so attach cannot match a different delivery head; Uncommitted is set.
func TestGateRunHEADMoveRecordsUncommitted_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"sh", "-c", "git commit --allow-empty -m x"},
	})
	headBefore := gitRevParse(t, repo)

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "a command that moves HEAD must not be citable")
	assert.Equal(t, headBefore, ev.HeadSHA, "HeadSHA is the pre-command revision")
	assert.Equal(t, 0, ev.Exit)
	assert.NotEqual(t, headBefore, gitRevParse(t, repo))
}

func TestGateRunSkipWorktreeInsideSubmoduleUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	sub := t.TempDir()
	run(t, sub, "git", "init")
	run(t, sub, "git", "config", "user.email", "test@test.com")
	run(t, sub, "git", "config", "user.name", "Test")
	run(t, sub, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "test.txt"), []byte("v1"), 0o644))
	run(t, sub, "git", "add", "test.txt")
	run(t, sub, "git", "commit", "-m", "init")
	run(t, repo, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	run(t, repo, "git", "commit", "-m", "add submodule")
	subCheckout := filepath.Join(repo, "vendor/lib")
	run(t, subCheckout, "git", "update-index", "--skip-worktree", "test.txt")
	require.NoError(t, os.WriteFile(filepath.Join(subCheckout, "test.txt"), []byte("mutated"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "skip-worktree mutation inside a submodule must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunAssumeUnchangedInsideSubmoduleUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	sub := t.TempDir()
	run(t, sub, "git", "init")
	run(t, sub, "git", "config", "user.email", "test@test.com")
	run(t, sub, "git", "config", "user.name", "Test")
	run(t, sub, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "test.txt"), []byte("v1"), 0o644))
	run(t, sub, "git", "add", "test.txt")
	run(t, sub, "git", "commit", "-m", "init")
	run(t, repo, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	run(t, repo, "git", "commit", "-m", "add submodule")
	subCheckout := filepath.Join(repo, "vendor/lib")
	run(t, subCheckout, "git", "update-index", "--assume-unchanged", "test.txt")
	require.NoError(t, os.WriteFile(filepath.Join(subCheckout, "test.txt"), []byte("mutated"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "assume-unchanged mutation inside a submodule must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunDirtySubmoduleDespiteIgnore_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})

	sub := t.TempDir()
	run(t, sub, "git", "init")
	run(t, sub, "git", "config", "user.email", "test@test.com")
	run(t, sub, "git", "config", "user.name", "Test")
	run(t, sub, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644))
	run(t, sub, "git", "add", "a.txt")
	run(t, sub, "git", "commit", "-m", "init")
	run(t, repo, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	run(t, repo, "git", "commit", "-m", "add submodule")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "vendor/lib", "dirty.txt"), []byte("x"), 0o644))
	run(t, repo, "git", "config", "submodule.vendor/lib.ignore", "dirty")

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "dirty submodule must not be hidden by submodule.ignore")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunGITWorkTreeExportStillSeesDirtyCheckout_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	run(t, repo, "git", "add", "src.txt")
	run(t, repo, "git", "commit", "-m", "add src")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	exportDir := t.TempDir()
	archive := exec.CommandContext(context.Background(), "git", "-C", repo, "archive", "--format=tar", "HEAD")
	tarOut, err := archive.Output()
	require.NoError(t, err)
	untar := exec.CommandContext(context.Background(), "tar", "-C", exportDir, "-xf", "-")
	untar.Stdin = strings.NewReader(string(tarOut))
	out, err := untar.CombinedOutput()
	require.NoError(t, err, "tar: %s", out)

	t.Setenv("GIT_WORK_TREE", exportDir)

	_, err = runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "GIT_WORK_TREE pointing at a clean export must not hide the dirty invoking checkout")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunCoreWorktreeExportStillSeesDirtyCheckout_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("v1"), 0o644))
	run(t, repo, "git", "add", "src.txt")
	run(t, repo, "git", "commit", "-m", "add src")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "src.txt"), []byte("mutated"), 0o644))

	exportDir := t.TempDir()
	archive := exec.CommandContext(context.Background(), "git", "-C", repo, "archive", "--format=tar", "HEAD")
	tarOut, err := archive.Output()
	require.NoError(t, err)
	untar := exec.CommandContext(context.Background(), "tar", "-C", exportDir, "-xf", "-")
	untar.Stdin = strings.NewReader(string(tarOut))
	out, err := untar.CombinedOutput()
	require.NoError(t, err, "tar: %s", out)

	run(t, repo, "git", "config", "core.worktree", exportDir)

	_, err = runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "core.worktree pointing at a clean export must not hide the dirty invoking checkout")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunGitlinkWithoutGitUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	sub := t.TempDir()
	run(t, sub, "git", "init")
	run(t, sub, "git", "config", "user.email", "test@test.com")
	run(t, sub, "git", "config", "user.name", "Test")
	run(t, sub, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644))
	run(t, sub, "git", "add", "a.txt")
	run(t, sub, "git", "commit", "-m", "init")
	run(t, repo, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	run(t, repo, "git", "commit", "-m", "add submodule")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "vendor/lib", "dirty.txt"), []byte("x"), 0o644))
	require.NoError(t, os.RemoveAll(filepath.Join(repo, "vendor/lib", ".git")))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "a gitlink directory with files but no .git must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunSubmoduleUntrackedDespiteShowUntrackedFilesNo_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	sub := t.TempDir()
	run(t, sub, "git", "init")
	run(t, sub, "git", "config", "user.email", "test@test.com")
	run(t, sub, "git", "config", "user.name", "Test")
	run(t, sub, "git", "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644))
	run(t, sub, "git", "add", "a.txt")
	run(t, sub, "git", "commit", "-m", "init")
	run(t, repo, "git", "-c", "protocol.file.allow=always", "submodule", "add", sub, "vendor/lib")
	run(t, repo, "git", "commit", "-m", "add submodule")
	subCheckout := filepath.Join(repo, "vendor/lib")
	run(t, subCheckout, "git", "config", "status.showUntrackedFiles", "no")
	require.NoError(t, os.WriteFile(filepath.Join(subCheckout, "helper.txt"), []byte("x"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "untracked file inside a submodule must not be hidden by the submodule's showUntrackedFiles=no")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunUntrackedDespiteShowUntrackedFilesNo_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	run(t, repo, "git", "config", "status.showUntrackedFiles", "no")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "helper.txt"), []byte("x"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "untracked files must not be hidden by status.showUntrackedFiles=no")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunInfoExcludeUncitable_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("helper.txt\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "helper.txt"), []byte("x"), 0o644))

	_, err = runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.True(t, ev.Uncommitted, "a file ignored only by .git/info/exclude must not be citable")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunCommittedGitignoreStillExempt_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("coverage.out\n"), 0o644))
	run(t, repo, "git", "add", ".gitignore")
	run(t, repo, "git", "commit", "-m", "ignore coverage")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "coverage.out"), []byte("x"), 0o644))

	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)

	ev := requireOneGateEvidence(t, repo)
	assert.False(t, ev.Uncommitted, "a committed .gitignore entry must still exempt the ignored file")
	assert.Equal(t, 0, ev.Exit)
}

func TestGateRunRejectsUnsafeProfileName_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	for _, name := range []string{"../evil", "foo/bar"} {
		_, err := runTrls(t, repo, "gate", "run", name)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "invalid gate profile", name)
	}
}

func TestGateRunLogMode0600_REQ_LNGHZN_S10_T3(t *testing.T) {
	repo := setupRepoWithTask(t)
	writeGatesConfig(t, repo, map[string][]string{
		"full": {"true"},
	})
	_, err := runTrls(t, repo, "gate", "run", "full")
	require.NoError(t, err)
	ev := requireOneGateEvidence(t, repo)
	info, err := os.Stat(ev.LogPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm()&0o777)
}

func writeGatesFile(t *testing.T, repo string, commands map[string][]string) {
	t.Helper()
	gates := make(map[string]any, len(commands))
	for name, cmd := range commands {
		gates[name] = map[string]any{"command": cmd}
	}
	out, err := json.Marshal(gates)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "gates.json"), out, 0o644))
}

func writeGatesConfig(t *testing.T, repo string, commands map[string][]string) {
	t.Helper()
	writeGatesFile(t, repo, commands)
	run(t, repo, "git", "add", "gates.json")
	run(t, repo, "git", "commit", "-m", "test: add gates.json")
}

func writeArmatureConfigGates(t *testing.T, repo string, commands map[string][]string) {
	t.Helper()
	path := filepath.Join(repo, ".armature", "config.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(raw, &cfg))
	gates := make(map[string]any, len(commands))
	for name, cmd := range commands {
		gates[name] = map[string]any{"command": cmd}
	}
	cfg["gates"] = gates
	out, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
}

func gitRevParse(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

type gateEvidenceJSON struct {
	Profile     string   `json:"profile"`
	Command     []string `json:"command"`
	HeadSHA     string   `json:"head_sha"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Exit        int      `json:"exit"`
	Uncommitted bool     `json:"uncommitted"`
	OutputHash  string   `json:"output_hash"`
	OutputHead  string   `json:"output_head"`
	OutputTail  string   `json:"output_tail"`
	LogPath     string   `json:"log_path"`
}

func requireOneGateEvidence(t *testing.T, repo string) gateEvidenceJSON {
	t.Helper()
	found := collectGateEvidence(t, repo)
	require.Len(t, found, 1, "expected exactly one gate-evidence op")
	return found[0]
}

func collectGateEvidence(t *testing.T, repo string) []gateEvidenceJSON {
	t.Helper()
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)

	var found []gateEvidenceJSON
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(opsDir, entry.Name()))
		require.NoError(t, err)
		for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var arr []json.RawMessage
			if err := json.Unmarshal([]byte(line), &arr); err != nil || len(arr) < 5 {
				continue
			}
			var opType string
			if err := json.Unmarshal(arr[0], &opType); err != nil || opType != "gate-evidence" {
				continue
			}
			var ev gateEvidenceJSON
			require.NoError(t, json.Unmarshal(arr[4], &ev))
			found = append(found, ev)
		}
	}
	return found
}
