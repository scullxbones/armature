package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGCExitError_AmbiguousExitsNonZero_REQ_LNGHZN_S5 verifies that `arm worktree
// gc` fails closed for the two classes that must never look like a clean run:
// removal failures and ambiguous terminal issues reconcile refused to GC. An
// ambiguous candidate previously exited 0, silently dropping it from gc's output.
func TestGCExitError_AmbiguousExitsNonZero_REQ_LNGHZN_S5(t *testing.T) {
	t.Parallel()

	// Nothing failed or ambiguous: clean exit.
	assert.NoError(t, gcExitError(nil, nil))

	// Ambiguous alone must exit non-zero and name the class.
	err := gcExitError(nil, []string{"task-13"})
	require.Error(t, err, "ambiguous GC candidates must not exit clean")
	assert.Contains(t, err.Error(), "ambiguous")

	// A removal failure alone exits non-zero.
	assert.Error(t, gcExitError([]string{"task-1"}, nil))

	// A removal failure takes precedence in the message over ambiguity.
	both := gcExitError([]string{"task-1"}, []string{"task-13"})
	require.Error(t, both)
	assert.Contains(t, both.Error(), "failed to remove")
}

// TestAddWorktreeDetached_RecoversPrunableRegistration_REQ_LNGHZN_S5 verifies that
// when a managed worktree directory is deleted out from under git (leaving a
// prunable administrative registration), a subsequent addWorktreeDetached at the
// same canonical path succeeds by clearing that stale registration with an
// exact-path --force add, instead of failing with "missing but already
// registered worktree" and looping every re-claim.
func TestAddWorktreeDetached_RecoversPrunableRegistration_REQ_LNGHZN_S5(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	wtPath := filepath.Join(repo, ".worktrees", "task-01")
	require.NoError(t, os.MkdirAll(filepath.Dir(wtPath), 0o755))
	require.NoError(t, addWorktreeDetached(repo, wtPath, "HEAD"))
	require.DirExists(t, wtPath)

	// Delete the worktree directory out from under git: its registration under
	// .git/worktrees survives and git marks it prunable.
	require.NoError(t, os.RemoveAll(wtPath))

	// A plain re-add would fail; addWorktreeDetached must detect the prunable
	// registration and clear it via an exact-path --force add.
	require.NoError(t, addWorktreeDetached(repo, wtPath, "HEAD"))
	assert.DirExists(t, wtPath)
}

// worktreeReconcileFixture is a repo wired with real claim/transition ops so the
// worktree commands exercise the true production read path (snapshot store ->
// materialize -> Reconcile). Hand-editing state JSON is useless here: the
// snapshot store re-materializes from the op log on every load, so the classes
// below are driven entirely by ops.
type worktreeReconcileFixture struct {
	repo             string
	boundPath        string // .worktrees/task-bound   (claimed, live)          -> bound
	orphanPath       string // .worktrees/task-orphan  (bound, claim released)  -> orphan
	ghostPath        string // .worktrees/task-ghost   (claimed, removed)       -> ghost
	gcPath           string // .worktrees/task-gc      (merged, on disk)        -> gc_ready / removed
	unrecognizedPath string // .worktrees/task-unbound (on disk, NO binding)    -> unrecognized
}

// setupWorktreeReconcileFixture builds a repo with one worktree in each
// reconcile class using real arm commands.
func setupWorktreeReconcileFixture(t *testing.T) worktreeReconcileFixture {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Distinct scopes so concurrent claims never trip scope-overlap handling.
	create := func(id, scope string) {
		_, cerr := runTrls(t, repo, "create", "--id", id, "--title", id, "--type", "task", "--scope", scope)
		require.NoError(t, cerr)
	}
	create("task-bound", "a.go")
	create("task-orphan", "b.go")
	create("task-ghost", "c.go")
	create("task-gc", "d.go")

	// task-bound: live claim with a managed worktree -> bound.
	_, err = runTrls(t, repo, "claim", "task-bound", "--worktree")
	require.NoError(t, err)

	// task-ghost: live claim, then delete the worktree off disk without arm.
	// The claim op still records its WorktreePath, so reconcile sees a live
	// claim whose worktree vanished -> ghost.
	_, err = runTrls(t, repo, "claim", "task-ghost", "--worktree")
	require.NoError(t, err)
	ghostPath := filepath.Join(repo, ".worktrees", "task-ghost")
	run(t, repo, "git", "worktree", "remove", "--force", ghostPath)

	// task-gc: live claim (records WorktreePath), then transition to merged
	// while the worktree is still on disk -> gc_ready.
	_, err = runTrls(t, repo, "claim", "task-gc", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-gc", "--to", "merged", "--force")
	require.NoError(t, err)

	// task-orphan: a BOUND worktree whose claim was released -> orphan. Claim it
	// so the binding is written, then transition back to open so no live claim
	// remains. An orphan is a real worktree with a binding and no live owner; a
	// worktree with no binding at all is a different class entirely (below).
	_, err = runTrls(t, repo, "claim", "task-orphan", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-orphan", "--to", "open")
	require.NoError(t, err)
	orphanPath := filepath.Join(repo, ".worktrees", "task-orphan")

	// task-unbound: a worktree git knows about, at a canonical-looking path, with
	// NO armature-issue-id binding -> unrecognized. Its basename would name a
	// plausible issue ID, which is precisely why identity must not be inferred
	// from it.
	unrecognizedPath := filepath.Join(repo, ".worktrees", "task-unbound")
	run(t, repo, "git", "worktree", "add", unrecognizedPath, "-b", "task/task-unbound")

	// Materialize so on-disk state reflects the ops (belt and suspenders; the
	// snapshot store also materializes on load).
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	return worktreeReconcileFixture{
		repo:             repo,
		boundPath:        filepath.Join(repo, ".worktrees", "task-bound"),
		orphanPath:       orphanPath,
		ghostPath:        ghostPath,
		gcPath:           filepath.Join(repo, ".worktrees", "task-gc"),
		unrecognizedPath: unrecognizedPath,
	}
}

// listJSON runs `worktree list --format json` and decodes it into string slices.
func listJSON(t *testing.T, repo string) map[string][]string {
	t.Helper()
	out, err := runTrls(t, repo, "worktree", "list", "--format", "json")
	require.NoError(t, err)

	var raw map[string][]string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &raw))
	return raw
}

// TestWorktreeListClassifiesEachClass_REQ_LNGHZN_S5_T2 asserts real reconciliation
// outcomes: the claimed worktree is bound, the unclaimed one is an orphan, the
// merged one is gc_ready, and the vanished claim is a ghost. These assertions
// FAIL against the pre-F1 code, which read from a non-existent issues dir and so
// classified nothing (every list came back empty).
func TestWorktreeListClassifiesEachClass_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	res := listJSON(t, fx.repo)

	assert.Contains(t, res["bound"], "task-bound", "claimed live worktree must be bound")
	assert.Contains(t, res["orphans"], "task-orphan", "unclaimed managed worktree must be an orphan")
	assert.Contains(t, res["gc_ready"], "task-gc", "merged worktree still on disk must be gc_ready")
	assert.Contains(t, res["ghosts"], "task-ghost", "claimed-but-removed worktree must be a ghost")

	// A worktree carrying no binding is unrecognized, never classified against
	// the issue its directory name happens to resemble.
	assert.Contains(t, res["unrecognized"], fx.unrecognizedPath, "unbound worktree must be unrecognized")
	assert.NotContains(t, res["bound"], "task-unbound")
	assert.NotContains(t, res["orphans"], "task-unbound")

	// Cross-class: a bound worktree is never simultaneously an orphan/gc target.
	assert.NotContains(t, res["orphans"], "task-bound")
	assert.NotContains(t, res["gc_ready"], "task-bound")
}

// TestWorktreeGCRemovesMergedWorktree_REQ_LNGHZN_S5_T2 asserts gc actually removes
// the merged worktree from disk and reports it as removed. This FAILS against the
// pre-F1 code, where gc found zero issues and removed nothing.
func TestWorktreeGCRemovesMergedWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	// Precondition: the merged worktree exists on disk.
	_, err := os.Stat(fx.gcPath)
	require.NoError(t, err, "gc worktree must exist before gc")

	out, err := runTrls(t, fx.repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)

	var res struct {
		Removed []string `json:"removed"`
		Skipped []string `json:"skipped"`
		Failed  []string `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.Contains(t, res.Removed, "task-gc", "merged worktree must be reported removed")
	assert.NotContains(t, res.Skipped, "task-gc")
	assert.Empty(t, res.Failed)

	// The worktree is actually gone from disk.
	_, statErr := os.Stat(fx.gcPath)
	assert.True(t, os.IsNotExist(statErr), "gc must remove the worktree directory from disk")
}

// TestWorktreeLifecycleIncludesExplicitClaimDestination_REQ_LNGHZN_S9_T1
// verifies that an Armature-claimed worktree outside .worktrees/ remains in
// lifecycle inventory: it is bound while claimed, GC-ready after cancellation,
// and removed by worktree gc.
func TestWorktreeLifecycleIncludesExplicitClaimDestination_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "child")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination, "--from", repo)
	require.NoError(t, err)

	listed := listJSON(t, repo)
	assert.Contains(t, listed["bound"], "task-01", "an explicitly claimed custom worktree must be visible as bound")

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled", "--force")
	require.NoError(t, err)

	listed = listJSON(t, repo)
	assert.Contains(t, listed["gc_ready"], "task-01", "a cancelled custom worktree must be visible as GC-ready")

	out, err := runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)
	var result struct {
		Removed []string `json:"removed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Contains(t, result.Removed, "task-01", "gc must remove the cancelled custom worktree")
	_, statErr := os.Stat(destination)
	assert.True(t, os.IsNotExist(statErr), "gc must remove the custom worktree directory")
}

func TestWorktreeGCRemovesArmatureOwnedCustomExclusionAndAllowsReuse_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-reuse")
	excludePath := filepath.Join(repo, ".git", "info", "exclude")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	exclude, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "custom-reuse", "external custom destinations must not change shared exclusions")

	_, err = runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err, "a live worktree must not be removed by gc")
	exclude, err = os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "custom-reuse", "external custom destinations must not change shared exclusions")

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled", "--force")
	require.NoError(t, err)
	out, err := runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)
	var result struct {
		Removed []string `json:"removed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Contains(t, result.Removed, "task-01")

	exclude, err = os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.NotContains(t, string(exclude), "custom-reuse", "external custom destinations do not use shared exclusions")
}

func TestWorktreeGCPreservesPreExistingCustomExclusion_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-user-exclude")
	excludePath := filepath.Join(repo, ".git", "info", "exclude")
	excludeBefore, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	content := string(excludeBefore)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "/custom-user-exclude/\n"
	require.NoError(t, os.WriteFile(excludePath, []byte(content), 0o600)) //nolint:gosec // G703: excludePath is derived from the test repository's Git directory

	_, err = runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled", "--force")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)

	excludeAfter, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	assert.Contains(t, string(excludeAfter), "/custom-user-exclude/", "gc must preserve a pre-existing identical user exclusion")
}

func TestWorktreeGCPreservesDirtyExplicitClaimDestination_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-dirty")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	workerFile := filepath.Join(destination, "worker-output.txt")
	require.NoError(t, os.WriteFile(workerFile, []byte("keep this\n"), 0o600))

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled", "--force")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.Error(t, err, "gc must fail rather than force-delete dirty custom worktree output")
	var result struct {
		Failed []string `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Contains(t, result.Failed, "task-01")
	contents, readErr := os.ReadFile(workerFile)
	require.NoError(t, readErr)
	assert.Equal(t, "keep this\n", string(contents))
}

func TestWorktreeListReportsMissingCustomGhostWithCloneLocalEvidence_REQ_LNGHZN_S9_T1(t *testing.T) {
	for _, destinationFor := range []struct {
		name string
		path func(repo string) string
	}{
		{name: "canonical worktree root", path: func(repo string) string { return filepath.Join(repo, ".worktrees", "custom-child") }},
		{name: "outside repository but registered", path: func(string) string { return filepath.Join(t.TempDir(), "custom-child") }},
	} {
		t.Run(destinationFor.name, func(t *testing.T) {
			repo := setupRepoWithTask(t)
			destination := destinationFor.path(repo)
			claim := newRootCmd()
			claim.SetOut(new(bytes.Buffer))
			claim.SetArgs([]string{"claim", "task-01", "--repo", repo, "--worktree", destination})
			require.NoError(t, claim.Execute())
			require.NoError(t, os.RemoveAll(destination), "simulate a missing/prunable custom worktree")

			listed := listJSON(t, repo)
			assert.Contains(t, listed["ghosts"], "task-01", "a missing local custom worktree must be visible as a ghost")
			assert.NotContains(t, listed["bound"], "task-01")
		})
	}
}

// TestWorktreeGCPreservesDirtyWorktree_REQ_LNGHZN_S5 verifies that gc reports
// a failed removal instead of silently force-deleting tracked or untracked
// worker output.
func TestWorktreeGCPreservesDirtyWorktree_REQ_LNGHZN_S5(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(t *testing.T, path string)
		file    string
	}{
		{
			name: "untracked",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(path, "untracked-worker-output.txt"), []byte("must survive gc\n"), 0o600))
			},
			file: "untracked-worker-output.txt",
		},
		{
			name: "tracked",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				dirtyPath := filepath.Join(path, "tracked-worker-output.txt")
				require.NoError(t, os.WriteFile(dirtyPath, []byte("committed\n"), 0o600))
				run(t, path, "git", "add", "tracked-worker-output.txt")
				run(t, path, "git", "commit", "-m", "worker output")
				require.NoError(t, os.WriteFile(dirtyPath, []byte("must survive gc\n"), 0o600))
			},
			file: "tracked-worker-output.txt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := setupWorktreeReconcileFixture(t)
			tc.prepare(t, fx.gcPath)
			dirtyPath := filepath.Join(fx.gcPath, tc.file)

			out, err := runTrls(t, fx.repo, "worktree", "gc", "--format", "json")
			require.Error(t, err, "gc must fail when git refuses to remove a dirty worktree")
			var result struct {
				Failed []string `json:"failed"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
			assert.Contains(t, result.Failed, "task-gc")
			contents, readErr := os.ReadFile(dirtyPath)
			require.NoError(t, readErr)
			assert.Equal(t, "must survive gc\n", string(contents))
		})
	}
}

func TestWorktreeGCDuplicateMarkerRemovesRecordedPathOnly_REQ_LNGHZN_S5_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-duplicate", "--title", "Duplicate", "--type", "task", "--scope", "duplicate.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-duplicate", "--worktree")
	require.NoError(t, err)

	canonicalPath := filepath.Join(repo, ".worktrees", "task-duplicate")
	legacyPath := filepath.Join(t.TempDir(), "legacy-duplicate")
	run(t, repo, "git", "worktree", "add", "-b", "legacy-duplicate", legacyPath)
	require.NoError(t, updateIssueIDFile(legacyPath, "task-duplicate"))

	_, err = runTrls(t, repo, "transition", "--issue", "task-duplicate", "--to", "merged", "--force")
	require.NoError(t, err)
	out, err := runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)
	var result struct {
		Removed []string `json:"removed"`
		Skipped []string `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Contains(t, result.Removed, "task-duplicate")
	assert.NotContains(t, result.Skipped, "task-duplicate")
	_, statErr := os.Stat(canonicalPath)
	assert.True(t, os.IsNotExist(statErr), "GC must remove the exact recorded canonical path")
	_, statErr = os.Stat(legacyPath)
	assert.NoError(t, statErr, "GC must not remove a duplicate marker at an external legacy path")
}

// TestWorktreeListTreatsPrunableRegistrationAsMissing_REQ_LNGHZN_S5_T2 verifies
// that a worktree directory removed outside git is not treated as a live bound
// worktree while git still reports its stale registration as prunable.
func TestWorktreeListTreatsPrunableRegistrationAsMissing_REQ_LNGHZN_S5_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-prunable", "--title", "Prunable", "--type", "task", "--scope", "prunable.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-prunable", "--worktree")
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".worktrees", "task-prunable")
	require.NoError(t, os.RemoveAll(worktreePath))

	res := listJSON(t, repo)
	assert.Contains(t, res["ghosts"], "task-prunable", "a prunable registration must not count as a live worktree")
	assert.NotContains(t, res["bound"], "task-prunable")
}

// TestWorktreeGCRemovesDetachedTerminalWorktree_REQ_LNGHZN_S5_T2 verifies gc
// removes a terminal managed worktree even after its branch is detached.
func TestWorktreeGCRemovesDetachedTerminalWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-detached-gc", "--title", "Detached GC", "--type", "task", "--scope", "detached.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "claim", "task-detached-gc", "--worktree")
	require.NoError(t, err)

	worktreePath := filepath.Join(repo, ".worktrees", "task-detached-gc")
	run(t, worktreePath, "git", "checkout", "--detach", "HEAD")
	_, err = runTrls(t, repo, "transition", "--issue", "task-detached-gc", "--to", "merged", "--force")
	require.NoError(t, err)

	out, err := runTrls(t, repo, "worktree", "gc", "--format", "json")
	require.NoError(t, err)

	var res struct {
		Removed []string `json:"removed"`
		Skipped []string `json:"skipped"`
		Failed  []string `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.Contains(t, res.Removed, "task-detached-gc", "detached terminal worktree must be removed")
	assert.NotContains(t, res.Skipped, "task-detached-gc")
	assert.Empty(t, res.Failed)
	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "gc must remove the detached worktree directory")
}

// TestWorktreeGCDryRunKeepsWorktree_REQ_LNGHZN_S5_T2 verifies --dry-run reports the
// merged worktree as a removal candidate but leaves it on disk.
func TestWorktreeGCDryRunKeepsWorktree_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	out, err := runTrls(t, fx.repo, "worktree", "gc", "--dry-run", "--format", "json")
	require.NoError(t, err)

	var res struct {
		DryRun      bool     `json:"dry_run"`
		WouldRemove []string `json:"would_remove"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.True(t, res.DryRun)
	assert.Contains(t, res.WouldRemove, "task-gc")

	// Still on disk after a dry run.
	_, statErr := os.Stat(fx.gcPath)
	assert.NoError(t, statErr, "dry-run must not remove the worktree")
}

// TestWorktreeGCDryRunWithNoCandidatesSucceeds_REQ_LNGHZN_S5_T10 verifies
// that a clean dry run has a successful exit status. Ambiguity, rather than an
// empty removal set, is what makes a dry run fail.
func TestWorktreeGCDryRunWithNoCandidatesSucceeds_REQ_LNGHZN_S5_T10(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	out, err := runTrls(t, repo, "worktree", "gc", "--dry-run", "--format", "json")
	require.NoError(t, err)
	var result struct {
		WouldRemove []string `json:"would_remove"`
		Ambiguous   []string `json:"ambiguous"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &result))
	assert.Empty(t, result.WouldRemove)
	assert.Empty(t, result.Ambiguous)
}

func setupAmbiguousGCRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-ambig", "--title", "Ambiguous", "--type", "task", "--scope", "ambig.go")
	require.NoError(t, err)
	for _, leaf := range []string{"task-ambig-a", "task-ambig-b"} {
		path := filepath.Join(repo, ".worktrees", leaf)
		run(t, repo, "git", "worktree", "add", "-b", leaf, path)
		require.NoError(t, updateIssueIDFile(path, "task-ambig"))
	}
	_, err = runTrls(t, repo, "transition", "--issue", "task-ambig", "--to", "cancelled", "--force")
	require.NoError(t, err)
	return repo, "task-ambig"
}

// TestWorktreeGCDryRunReportsAmbiguous_REQ_LNGHZN_S5_T10 requires dry-run
// to expose the same unsafe ambiguity and exit status as a real gc run.
func TestWorktreeGCDryRunReportsAmbiguous_REQ_LNGHZN_S5_T10(t *testing.T) {
	repo, issueID := setupAmbiguousGCRepo(t)

	out, dryErr := runTrls(t, repo, "worktree", "gc", "--dry-run", "--format", "json")
	require.Error(t, dryErr)
	var res struct {
		Ambiguous []string `json:"ambiguous"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &res))
	assert.Contains(t, res.Ambiguous, issueID)

	_, stderr, humanErr := runTrlsWithStderr(t, repo, "worktree", "gc", "--dry-run", "--format", "human")
	require.Error(t, humanErr)
	assert.Contains(t, stderr, issueID)
}

// TestWorktreeListHumanFormat_REQ_LNGHZN_S5_T2 verifies the human format renders
// the class headings and the classified issue IDs.
func TestWorktreeListHumanFormat_REQ_LNGHZN_S5_T2(t *testing.T) {
	fx := setupWorktreeReconcileFixture(t)

	out, err := runTrls(t, fx.repo, "worktree", "list", "--format", "human")
	require.NoError(t, err)

	assert.Contains(t, out, "BOUND WORKTREES:")
	assert.Contains(t, out, "task-bound")
	assert.Contains(t, out, "task-orphan")
	assert.Contains(t, out, "task-gc")
}

// TestManagedWorktreeRoots_RelativeRepoPath_REQ_LNGHZN_S5_T2 pins the fix for
// the reconcile no-op that survived F1: in production ctx.RepoPath defaults
// to "." when --repo is not passed, while `git worktree list` emits absolute
// paths. Both local-evidence roots must be absolute even for a relative
// repoPath. `worktree.ListManaged` fail-closed on a git inventory error is
// covered in internal/worktree.
func TestManagedWorktreeRoots_RelativeRepoPath_REQ_LNGHZN_S5_T2(t *testing.T) {
	t.Parallel()
	roots := managedWorktreeRoots(".")
	require.Len(t, roots, 2)
	assert.True(t, filepath.IsAbs(roots[0]), "canonical root must be absolute, got %q", roots[0])
	assert.True(t, filepath.IsAbs(roots[1]), "repo root must be absolute, got %q", roots[1])
}

// TestGoWorkMitigationApplied_REQ_LNGHZN_S5_T3 drives `arm claim --worktree`
// end-to-end and asserts the worktree mitigation ran with the correct effect:
// with no go.work in the main tree, the mitigation is a no-op — it neither
// creates a go.work in the main tree nor a bare go.work in the worktree (the
// latter would break `go build ./...` inside the worktree).
func TestGoWorkMitigationApplied_REQ_LNGHZN_S5_T3(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "task-mit", "--title", "Mit task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "claim", "task-mit", "--worktree")
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(repo, "go.work"))
	assert.True(t, os.IsNotExist(statErr), "no go.work should be created in the main tree")

	wt := filepath.Join(repo, ".worktrees", "task-mit")
	_, statErr = os.Stat(filepath.Join(wt, "go.work"))
	assert.True(t, os.IsNotExist(statErr), "no go.work should be created in the worktree")
}

// TestWorktreeListHumanFormatReportsAmbiguous_REQ_LNGHZN_S9_T1 verifies that
// `arm worktree list --format human` surfaces the same ambiguous-terminal-issue
// class that gc refuses to touch, so a human operator reviewing `list` output
// is not left unaware of an issue with two candidate worktrees.
func TestWorktreeListHumanFormatReportsAmbiguous_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo, issueID := setupAmbiguousGCRepo(t)

	out, err := runTrls(t, repo, "worktree", "list", "--format", "human")
	require.NoError(t, err)
	assert.Contains(t, out, "AMBIGUOUS GC CANDIDATES", "list must render the ambiguous heading")
	assert.Contains(t, out, issueID, "list must name the ambiguous issue")
}

// TestWorktreeGCHumanFormatReportsFailure_REQ_LNGHZN_S9_T1 verifies that a
// real (non-dry-run) `arm worktree gc --format human` invocation prints the
// "Failed to remove" section and names the issue whose worktree could not be
// force-removed (a dirty custom claim destination), matching the JSON
// "failed" list that TestWorktreeGCPreservesDirtyExplicitClaimDestination
// already asserts for the JSON format.
func TestWorktreeGCHumanFormatReportsFailure_REQ_LNGHZN_S9_T1(t *testing.T) {
	repo := setupRepoWithTask(t)
	destination := filepath.Join(t.TempDir(), "custom-dirty-human")

	_, err := runTrls(t, repo, "claim", "task-01", "--worktree", destination)
	require.NoError(t, err)
	workerFile := filepath.Join(destination, "worker-output.txt")
	require.NoError(t, os.WriteFile(workerFile, []byte("keep this\n"), 0o600))

	_, err = runTrls(t, repo, "transition", "--issue", "task-01", "--to", "cancelled", "--force")
	require.NoError(t, err)

	_, stderr, gcErr := runTrlsWithStderr(t, repo, "worktree", "gc", "--format", "human")
	require.Error(t, gcErr, "gc must fail rather than force-delete dirty custom worktree output")
	assert.Contains(t, stderr, "Failed to remove", "human format must render the failure section")
	assert.Contains(t, stderr, "task-01")

	contents, readErr := os.ReadFile(workerFile)
	require.NoError(t, readErr)
	assert.Equal(t, "keep this\n", string(contents), "the dirty worker output must survive the failed removal attempt")
}
