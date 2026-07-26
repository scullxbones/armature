package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
)

// TestTransitionDoneBlockedByGate_REQ_LNGHZN_S4_T2 verifies that transition
// to done is blocked when the worktree has uncommitted changes.
func TestTransitionDoneBlockedByGate_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-01", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-01-wt")
	_, err = runTrls(t, repo, "claim", "gate-01", "--worktree", wt)
	require.NoError(t, err)

	// Commit a scoped file with a conventional-commit reference so only the
	// clean-tree check is exercised.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-01): add foo")

	// Now dirty the tree with an uncommitted change.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n// dirty\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestDeliveryGateBlocksMissingCommitReference_REQ_LNGHZN_S4_T2 verifies that
// transition to done is blocked when no commit matches the conventional format.
func TestDeliveryGateBlocksMissingCommitReference_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-02", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-02-wt")
	_, err = runTrls(t, repo, "claim", "gate-02", "--worktree", wt)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "no conventional reference here")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-02", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestTransitionDoneGateOverride_REQ_LNGHZN_S4_T2 verifies that --skip-delivery-gate
// allows transition to done even when gate checks would otherwise fail, and
// records the override in the transition op's payload.
func TestTransitionDoneGateOverride_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-03", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-03-wt")
	_, err = runTrls(t, repo, "claim", "gate-03", "--worktree", wt)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "no conventional reference here")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-03", "--to", "done", "--outcome", "test", "--skip-delivery-gate", "--force")
	assert.NoError(t, err)
}

// TestGateNotRunForNonDoneTransitions_REQ_LNGHZN_S4_T2 verifies that the
// delivery gate is only evaluated for --to done transitions.
func TestGateNotRunForNonDoneTransitions_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-04", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-04-wt")
	_, err = runTrls(t, repo, "claim", "gate-04", "--worktree", wt)
	require.NoError(t, err)

	// No commits, no scoped changes at all: gate checks would fail if run,
	// but transitioning to "blocked" must not invoke the gate.
	_, err = runTrls(t, wt, "transition", "--issue", "gate-04", "--to", "blocked", "--outcome", "waiting")
	assert.NoError(t, err)
}

// TestGateSkippedForNonTaskIssueKindOnDone_PR88 verifies that the delivery
// gate — which validates a claimed task's own worktree binding, scope, and
// commits — is not invoked at all for non-task issue kinds (e.g. "story")
// transitioning to done. Stories are transitioned to done from a manually
// created feat/STORY-ID branch per the coordinator workflow and are never
// claimed or worktree-bound, so the gate must not apply to them — not even
// require --skip-delivery-gate.
func TestGateSkippedForNonTaskIssueKindOnDone_PR88(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "story-01", "--title", "A story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)
	// Materialize the index so the story's index entry (and its Type) is on
	// disk for transition's ReadIndex to find — mirrors how `claim` triggers
	// materialization as a side effect for task-kind issues in other tests.
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Never claimed, never worktree-bound. Transition to done directly from a
	// feature branch, as the coordinator workflow does.
	run(t, repo, "git", "checkout", "-b", "feat/story-01")
	_, err = runTrls(t, repo, "transition", "--issue", "story-01", "--to", "done", "--outcome", "test")
	assert.NoError(t, err)
}

// TestGateAppliesToBugIssueKindOnDone_PR88 verifies that the delivery gate
// applies to issue kind "bug" the same way it applies to "task": bugs get a
// worktree+branch created on claim (see internal/materialize/branch.go's
// "bug" case) and are expected to go through the same scoped/committed
// worker workflow, so the gate must not be silently skipped for them.
func TestGateAppliesToBugIssueKindOnDone_PR88(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-bug-01", "--title", "Gate bug", "--type", "bug", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-bug-01-wt")
	_, err = runTrls(t, repo, "claim", "gate-bug-01", "--worktree", wt)
	require.NoError(t, err)

	// Dirty the tree without committing: the clean-tree check should block.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-bug-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestGateAppliesToFeatureIssueKindOnDone_PR88 verifies that the delivery
// gate applies to issue kind "feature" the same way it applies to "task":
// features get a worktree+branch created on claim (see
// internal/materialize/branch.go's "feature" case) and are expected to go
// through the same scoped/committed worker workflow, so the gate must not be
// silently skipped for them.
func TestGateAppliesToFeatureIssueKindOnDone_PR88(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-feat-01", "--title", "Gate feature", "--type", "feature", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-feat-01-wt")
	_, err = runTrls(t, repo, "claim", "gate-feat-01", "--worktree", wt)
	require.NoError(t, err)

	// Dirty the tree without committing: the clean-tree check should block.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-feat-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestTransitionDoneNoBoundWorktreeFailsClosed_REQ_LNGHZN_S4_T2 verifies
// that the delivery gate fails closed (refuses the transition, does not skip
// silently) when the target issue cannot be found in the materialized index —
// e.g. because no bound context could be resolved for it.
func TestTransitionDoneNoBoundWorktreeFailsClosed_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// No `create` was run for "ghost-01": the issue has no entry in the
	// materialized index at all.
	_, err = runTrls(t, repo, "transition", "--issue", "ghost-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in materialized index")
}

// TestDeliveryGateBlocksOutOfScopeFiles_REQ_LNGHZN_S4_T2 verifies that
// transition to done is blocked when a committed change touches a file
// outside the issue's declared scope.
func TestDeliveryGateBlocksOutOfScopeFiles_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-05", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-05-wt")
	_, err = runTrls(t, repo, "claim", "gate-05", "--worktree", wt)
	require.NoError(t, err)

	// Commit a file outside declared scope ("bar.go" is not in scope "foo.go").
	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "bar.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-05): add bar")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-05", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestDeliveryGateSurvivesWorktreeRecreation_REQ_LNGHZN_S4_T1 verifies that
// if a task's worktree is removed (e.g. by `arm merged`'s RemoveWorktree)
// while the branch itself still exists, and the task is later re-claimed
// (recreating the worktree at a new path), the delivery gate still scopes
// against the branch's true original divergence point rather than whatever
// happens to be checked out in the main repo at re-claim time. The parent
// branch is recorded as git config on claim (shared across worktrees), so it
// survives worktree removal even though the per-worktree base-commit file
// does not.
func TestDeliveryGateSurvivesWorktreeRecreation_REQ_LNGHZN_S4_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// Simulate a story branch the coordinator has checked out, standing in
	// for a real story branch (no need to materialize an actual story issue
	// for this test — claim.go only cares about the git branch name).
	run(t, repo, "git", "checkout", "-b", "story-branch")

	_, err = runTrls(t, repo, "create", "--id", "gate-06", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt1 := filepath.Join(t.TempDir(), "gate-06-wt1")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree", wt1)
	require.NoError(t, err)

	// Remove the worktree directly (as RemoveWorktree in merged.go would),
	// leaving the task branch intact.
	run(t, repo, "git", "worktree", "remove", wt1, "--force")

	// After removal, the story branch (still checked out in the main repo)
	// gains a new commit unrelated to the task — simulating a sibling task
	// completing after gate-06 was originally claimed.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(gate-sibling): unrelated sibling work")

	// Re-claim gate-06 at a new worktree path: the branch already exists, so
	// this exercises the worktree-recreation path.
	wt2 := filepath.Join(t.TempDir(), "gate-06-wt2")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree", wt2)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt2, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt2, "git", "add", "foo.go")
	run(t, wt2, "git", "commit", "-m", "feat(gate-06): add foo")

	// The sibling commit landed on story-branch strictly after gate-06
	// diverged from it, so it must not be attributed to gate-06's diff.
	_, err = runTrls(t, wt2, "transition", "--issue", "gate-06", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "sibling commit added after worktree removal must not be misattributed as in-scope diff")
}

// TestDeliveryGateSurvivesRebaseOntoUpdatedParent_REQ_LNGHZN_S4_T1 verifies
// that if a task branch is rebased onto an updated parent-branch tip
// (picking up new sibling commits along the way), the delivery gate
// recomputes the branch-point dynamically via merge-base rather than trusting
// a base commit SHA recorded once at claim time — a stale recorded SHA would
// misattribute the rebased-in sibling commits as in-scope diff.
func TestDeliveryGateSurvivesRebaseOntoUpdatedParent_REQ_LNGHZN_S4_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "checkout", "-b", "story-branch")

	_, err = runTrls(t, repo, "create", "--id", "gate-07", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-07-wt")
	_, err = runTrls(t, repo, "claim", "gate-07", "--worktree", wt)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-07): add foo")

	// A sibling commit lands on story-branch after gate-07 branched off.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(gate-sibling): unrelated sibling work")

	// Rebase the task branch onto the updated story-branch tip, pulling the
	// sibling commit into the task branch's own ancestry.
	run(t, wt, "git", "rebase", "story-branch")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-07", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "sibling commit pulled in by rebase onto updated parent tip must not be misattributed as in-scope diff")
}

// TestDeliveryGateFallsBackWhenParentBranchConfigIsLiteralHEAD_REQ_LNGHZN_S4_T2
// verifies that a stale parent-branch git config record from before the
// detached-HEAD guard existed (a literal "HEAD" value) is treated as no
// usable parent branch at gate-check time, rather than being resolved as the
// task branch's own tip (which would collapse the merge-base and make the
// commit-reference range empty). The gate must fall back to the existing
// getBaseCommit chain (merge-base against a default/candidate branch) and
// still pass.
func TestDeliveryGateFallsBackWhenParentBranchConfigIsLiteralHEAD_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "branch", "-m", "main")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-09", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-09-wt")
	_, err = runTrls(t, repo, "claim", "gate-09", "--worktree", wt)
	require.NoError(t, err)

	// Simulate a pre-fix claim record: force the persisted parent-branch
	// config value for this task branch to the literal string "HEAD", as
	// would have been written before commit 978405cc's idempotency guard
	// existed.
	git := adapters.New(repo)
	require.NoError(t, git.SetGitConfig(parentBranchConfigKey("gate-09"), "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-09): add foo")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-09", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "stale literal-HEAD parent-branch config must self-heal via fallback, not collapse the merge-base range")
}

// TestTransitionDoneRepoNotBoundToIssueFailsClosed_REQ_LNGHZN_S4_T2 verifies
// that transitioning issue X to done with --repo pointed at a directory that
// is NOT the worktree bound to issue X fails closed with a clear error,
// rather than running the delivery gate against the wrong directory (which
// could pass even if the actual claimed worktree for X is dirty or
// out-of-scope).
func TestTransitionDoneRepoNotBoundToIssueFailsClosed_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-10a", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-10b", "--title", "Gate task", "--type", "task", "--scope", "bar.go")
	require.NoError(t, err)

	wtA := filepath.Join(t.TempDir(), "gate-10a-wt")
	_, err = runTrls(t, repo, "claim", "gate-10a", "--worktree", wtA)
	require.NoError(t, err)

	wtB := filepath.Join(t.TempDir(), "gate-10b-wt")
	_, err = runTrls(t, repo, "claim", "gate-10b", "--worktree", wtB)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wtA, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wtA, "git", "add", "foo.go")
	run(t, wtA, "git", "commit", "-m", "feat(gate-10a): add foo")

	// Attempt to transition gate-10a to done, but point --repo at wtB (bound
	// to a different issue) instead of wtA. runTrls injects "--repo" using
	// its repo argument, so pass wtB there rather than as an extra flag.
	_, err = runTrls(t, wtB, "transition", "--issue", "gate-10a", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gate-10b")

	// Regression: transitioning with --repo correctly pointed at the bound
	// worktree still passes.
	_, err = runTrls(t, wtA, "transition", "--issue", "gate-10a", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err)
}

// TestDeliveryGateRunsAfterPreTransitionHooks_REQ_LNGHZN_S4_T2 verifies that
// the delivery gate check evaluates the worktree state produced AFTER
// pre-transition hooks run, not the state before them. A configured
// pre-transition hook here dirties a tracked file in the worktree as a side
// effect (simulating a formatter or code generator); if the gate ran before
// hooks (the previous ordering), the clean-tree check would have already
// passed and this dirty file would slip through undetected. With the gate
// running after hooks, the transition must fail.
func TestDeliveryGateRunsAfterPreTransitionHooks_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-08", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(t.TempDir(), "gate-08-wt")
	_, err = runTrls(t, repo, "claim", "gate-08", "--worktree", wt)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-08): add foo")

	// Configure a pre-transition hook that dirties the tracked file in the
	// worktree as a side effect, then reports success. This is written to the
	// shared .armature/config.json in the main repo (linked worktrees share
	// this config), so it's picked up when the transition command runs from wt.
	fooPath := filepath.Join(wt, "foo.go")
	cfg := config.DefaultConfig("go")
	cfg.Hooks = []config.HookConfig{
		{
			Name: "dirtying-hook",
			Command: []string{"sh", "-c", fmt.Sprintf(
				"echo '// dirtied by hook' >> %q && echo '{\"allowed\":true}'",
				fooPath,
			)},
		},
	}
	require.NoError(t, config.WriteConfig(filepath.Join(repo, ".armature", "config.json"), cfg))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-08", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err, "gate must catch the dirty tree left behind by the pre-transition hook")
	assert.Contains(t, err.Error(), "delivery gate")
}
