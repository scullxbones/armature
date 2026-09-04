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
	"github.com/scullxbones/armature/internal/deliverygate"
	"github.com/scullxbones/armature/internal/ops"
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

	wt := filepath.Join(repo, ".worktrees", "gate-01")
	_, err = runTrls(t, repo, "claim", "gate-01", "--worktree")
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

	wt := filepath.Join(repo, ".worktrees", "gate-02")
	_, err = runTrls(t, repo, "claim", "gate-02", "--worktree")
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
// records the override in the transition op's payload alongside its outcome.
func TestTransitionDoneGateOverride_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-03", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-03")
	_, err = runTrls(t, repo, "claim", "gate-03", "--worktree")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "no conventional reference here")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-03", "--to", "done", "--outcome", testIntroductionOutcome, "--skip-delivery-gate", "--force")
	require.NoError(t, err)

	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	for _, op := range allOps {
		if op.Type == ops.OpTransition && op.TargetID == "gate-03" {
			assert.Equal(t, "done", op.Payload.To)
			assert.Equal(t, testIntroductionOutcome, op.Payload.Outcome)
			assert.True(t, op.Payload.SkippedDeliveryGate)
			return
		}
	}
	t.Fatal("expected a transition op with the recorded delivery-gate override")
}

// TestTransitionRejectsGateOverrideOutsideDone_REQ_LNGHZN_S4_T2 verifies that
// the delivery-gate override is only valid for a transition to done and does
// not append a misleading audit field to transitions where no gate exists.
func TestTransitionRejectsGateOverrideOutsideDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-override-01", "--title", "Gate task", "--type", "task")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "transition", "--issue", "gate-override-01", "--to", "blocked", "--skip-delivery-gate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid with --to done")

	allOps, err := readAllOpsFromDir(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	for _, op := range allOps {
		assert.False(t, op.Type == ops.OpTransition && op.TargetID == "gate-override-01", "invalid override must not append a transition op")
	}
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

	wt := filepath.Join(repo, ".worktrees", "gate-04")
	_, err = runTrls(t, repo, "claim", "gate-04", "--worktree")
	require.NoError(t, err)

	// No commits, no scoped changes at all: gate checks would fail if run,
	// but transitioning to "blocked" must not invoke the gate.
	_, err = runTrls(t, wt, "transition", "--issue", "gate-04", "--to", "blocked", "--outcome", "waiting")
	assert.NoError(t, err)
}

// TestTransitionDoneReplaysAmendedScopeBeforeGating_REQ_LNGHZN_S4_T2 verifies
// that an amend op immediately narrows the delivery scope, even before a
// separate materialize run updates the derived index. The append-only ops log
// is authoritative, so an out-of-scope delivery must be rejected rather than
// slipping through on the snapshot's former, broader scope.
func TestTransitionDoneReplaysAmendedScopeBeforeGating_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-scope-01", "--title", "Gate task", "--type", "task",
		"--scope", "foo.go", "--scope", "bar.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-scope-01")
	_, err = runTrls(t, repo, "claim", "gate-scope-01", "--worktree")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "foo.go", "bar.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-scope-01): add delivery files")

	// amend is low-stakes and deliberately does not materialize synchronously;
	// this leaves the on-disk index at its former [foo.go, bar.go] scope.
	_, err = runTrls(t, repo, "amend", "gate-scope-01", "--scope", "foo.go")
	require.NoError(t, err)

	_, err = runTrls(t, wt, "transition", "--issue", "gate-scope-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "the amended scope must reject the committed bar.go without requiring materialization")
	assert.Contains(t, err.Error(), "bar.go")
}

// TestTransitionDoneClaimedIssueAmendedToEpicStillRunsGate_REQ_LNGHZN_S4_T2
// verifies that a type amendment cannot turn a claimed, worktree-bound issue
// into a delivery-gate exemption. The amend retains the active claim, so a
// dirty bound worktree must still be rejected even though epics are normally
// completed outside the claimed-worktree workflow.
func TestTransitionDoneClaimedIssueAmendedToEpicStillRunsGate_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-amended-epic-01", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-amended-epic-01")
	_, err = runTrls(t, repo, "claim", "gate-amended-epic-01", "--worktree")
	require.NoError(t, err)

	// amend is low-stakes and does not materialize synchronously. Its replayed
	// type is now epic, but its active claim and bound worktree remain live.
	_, err = runTrls(t, repo, "amend", "gate-amended-epic-01", "--type", "epic")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-amended-epic-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "a claimed issue remains bound after its type changes and must be delivery-gated")
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestGateSkippedForNonTaskIssueKindOnDone_REQ_LNGHZN_S4_T2 verifies that the delivery
// gate — which validates a claimed task's own worktree binding, scope, and
// commits — is not invoked at all for non-task issue kinds (e.g. "story")
// transitioning to done. Stories are transitioned to done from a manually
// created feat/STORY-ID branch per the coordinator workflow and are never
// claimed or worktree-bound, so the gate must not apply to them — not even
// require --skip-delivery-gate.
func TestGateSkippedForNonTaskIssueKindOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
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

// TestTransitionDoneUnmaterializedStoryReplaysAuthoritativeOps_REQ_LNGHZN_S4_T2
// verifies that an unclaimed story created since the last materialization can
// complete through its coordinator-level flow. The derived index is empty,
// but the append-only create op is authoritative and confirms that the issue
// exists and is exempt from the claimed-worktree delivery gate.
func TestTransitionDoneUnmaterializedStoryReplaysAuthoritativeOps_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-unmaterialized-story-01", "--title", "Gate story", "--type", "story")
	require.NoError(t, err)

	// Do not materialize: the index has no entry for this story. A coordinator
	// completes unclaimed stories from its own feature branch.
	run(t, repo, "git", "checkout", "-b", "feat/gate-unmaterialized-story-01")
	_, err = runTrls(t, repo, "transition", "--issue", "gate-unmaterialized-story-01", "--to", "done", "--outcome", "test")
	require.NoError(t, err)

	issue, err := currentIssueFromOps(getTestContext(t, repo).IssuesDir, "gate-unmaterialized-story-01")
	require.NoError(t, err)
	assert.Equal(t, "done", issue.Status)
}

// TestGateSkippedForUnclaimedEpicOnDone_REQ_LNGHZN_S4_T2 verifies that a
// normal, unclaimed epic remains exempt from the claimed-worktree delivery
// gate. Only a live claimant turns an otherwise exempt issue type into a
// fail-closed gate path.
func TestGateSkippedForUnclaimedEpicOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-epic-01", "--title", "Gate epic", "--type", "epic")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	run(t, repo, "git", "checkout", "-b", "feat/gate-epic-01")
	_, err = runTrls(t, repo, "transition", "--issue", "gate-epic-01", "--to", "done", "--outcome", "test")
	assert.NoError(t, err)
}

// TestGateAppliesToClaimedStoryWorktreeOnDone_REQ_LNGHZN_S4_T2 verifies that
// the delivery gate is NOT blanket-skipped for issue kind "story": claim.go
// supports claiming a story directly (deriving a feat/<ID> branch and
// creating/binding a linked worktree for it, same as task/bug/feature), so a
// story that went through that claimed-worktree workflow must be gated the
// same way before being marked done. Only the *unclaimed* coordinator-level
// story transition (see TestGateSkippedForNonTaskIssueKindOnDone above) is
// exempt.
func TestGateAppliesToClaimedStoryWorktreeOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-01", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-01")
	_, err = runTrls(t, repo, "claim", "gate-story-01", "--worktree")
	require.NoError(t, err)

	// Dirty the tree without committing: the clean-tree check should block,
	// proving the gate actually ran against this claimed story worktree
	// instead of being skipped outright because the issue kind is "story".
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-story-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestGateAppliesToClaimedStoryWorktreeFromDifferentCheckout_REQ_LNGHZN_S4_T2
// verifies that the delivery gate cannot be silently bypassed by running
// `arm transition` from a checkout other than the one a story was actually
// claimed into. Before the fix, isClaimedWorktreeForIssue only inspected the
// invoking checkout's own armature-issue-id marker; invoking from the main
// coordinator repo (never claimed, no marker) made the story-type gate
// probe return false and skip the gate entirely -- with
// SkippedDeliveryGate: false recorded, leaving no audit trail that anything
// was bypassed. See
// docs/dogfood/findings/raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md.
func TestGateAppliesToClaimedStoryWorktreeFromDifferentCheckout_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-02", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-02")
	_, err = runTrls(t, repo, "claim", "gate-story-02", "--worktree")
	require.NoError(t, err)

	// Dirty the claimed worktree without committing: the clean-tree check
	// would block if the gate actually ran.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	// Invoke the transition from the MAIN repo checkout, not the worktree the
	// story was claimed into. Before the fix, this made
	// isClaimedWorktreeForIssue read the main repo's own (absent) marker,
	// return false, and skip the gate -- letting the dirty claimed worktree
	// pass "done" undetected.
	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-02", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "transitioning a claimed story to done from a different checkout must still run the delivery gate")
	assert.Contains(t, err.Error(), "Working tree is not clean", "must fail specifically on the clean-tree check this test dirtied, not some unrelated error")
}

// TestTransitionDoneSelfUnassignThenDoneStillGatesLingeringWorktree_REQ_LNGHZN_S4_T2
// reproduces the self-unassign delivery-gate bypass: `arm unassign` is
// self-service with no permission check and clears ClaimedBy via a
// transition->open op, but it does not touch the worktree's
// armature-issue-id marker or its working tree contents. A worker could
// previously claim a story into a worktree, make dirty/out-of-scope/
// uncommitted changes, self-unassign, and then `transition --to done
// --force` — since the gate was keyed solely on ClaimedBy != "", it never
// ran against the still-bound, still-dirty worktree. The gate must now run
// against any worktree whose marker still names this issue, regardless of
// materialized claim state.
func TestTransitionDoneSelfUnassignThenDoneStillGatesLingeringWorktree_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-released-01", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-released-01")
	_, err = runTrls(t, repo, "claim", "gate-story-released-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "unassign", "--issue", "gate-story-released-01")
	require.NoError(t, err)

	// The released worktree still carries its armature-issue-id marker (kept
	// so it can be reused safely) and is now dirtied — exactly the exploit
	// scenario: claim, dirty, self-unassign, done --force.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-released-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "self-unassigning must not exempt a still-bound, dirty worktree from the delivery gate")
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestTransitionDoneReopenedThenDoneStillGatesLingeringWorktree_REQ_LNGHZN_S4_T2
// verifies the same bypass is closed when the story is released via
// `transition --to open` rather than `unassign`.
func TestTransitionDoneReopenedThenDoneStillGatesLingeringWorktree_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-reopened-01", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-reopened-01")
	_, err = runTrls(t, repo, "claim", "gate-story-reopened-01", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-reopened-01", "--to", "open", "--force")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-reopened-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "transition --to open must not exempt a still-bound, dirty worktree from the delivery gate")
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestTransitionDoneReleasedStoryWithWorktreeFullyRemovedStaysExempt_REQ_LNGHZN_S4_T2
// verifies the legitimate case the gate must still allow: a story is
// claimed, its worktree is later fully removed and pruned (no lingering
// marker anywhere), and it is released. Since no worktree anywhere still
// carries this issue's marker, the coordinator-level completion flow stays
// exempt, distinct from the self-unassign exploit above where the marker
// and dirty contents linger.
func TestTransitionDoneReleasedStoryWithWorktreeFullyRemovedStaysExempt_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-released-02", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-released-02")
	_, err = runTrls(t, repo, "claim", "gate-story-released-02", "--worktree")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "unassign", "--issue", "gate-story-released-02")
	require.NoError(t, err)

	run(t, repo, "git", "worktree", "remove", wt, "--force")
	run(t, repo, "git", "worktree", "prune")

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-released-02", "--to", "done", "--outcome", "test", "--force")
	require.NoError(t, err)
}

// TestGateAppliesToClaimedStoryWorktreeInDetachedHEAD_REQ_LNGHZN_S4_T2
// verifies that the delivery gate cannot be bypassed by leaving the claimed
// story worktree in a detached HEAD (or checked out to some other branch)
// before transitioning from a different checkout. Before this fix,
// resolveClaimedStoryWorktree located the claimed worktree by first finding
// whichever worktree had refs/heads/feat/<id> checked out
// (deriveBranchName + findWorktreePathByBranch) and only then checked its
// marker file -- so a claimed worktree that wasn't currently on its story
// branch (detached HEAD mid-rebase/mid-bisect, or checked out to a scratch
// branch) was invisible to the scan and the gate silently skipped, exactly
// as in the original bug. See
// docs/dogfood/findings/raw/2026-08-02T1600Z-claude-workflow-story-gate-bypass-via-wrong-checkout.md.
func TestGateAppliesToClaimedStoryWorktreeInDetachedHEAD_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-03", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-03")
	_, err = runTrls(t, repo, "claim", "gate-story-03", "--worktree")
	require.NoError(t, err)

	// Dirty the claimed worktree without committing: the clean-tree check
	// would block if the gate actually ran.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	// Leave the claimed worktree in a detached HEAD, as if mid-rebase or
	// mid-bisect: no worktree now has refs/heads/feat/gate-story-03 checked
	// out, so a branch-first scan finds nothing even though the worktree's
	// own armature-issue-id marker still names gate-story-03.
	run(t, wt, "git", "checkout", "--detach")

	// Invoke the transition from the MAIN repo checkout, not the (now
	// detached) worktree the story was claimed into.
	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-03", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "transitioning a claimed story to done must still run the delivery gate even when the claimed worktree is in a detached HEAD")
	// The branch-binding check runs before the clean-tree check (see
	// runDeliveryGateCheck), so a detached HEAD trips that check first --
	// still proof the gate ran against the claimed worktree instead of being
	// silently skipped.
	assert.Contains(t, err.Error(), "gate-story-03")
	assert.Contains(t, err.Error(), "--skip-delivery-gate")
}

// TestTransitionDoneClaimedStoryWithoutWorktreeFailsClosed_REQ_LNGHZN_S4_T2
// verifies that a story which remains claimed after its claimed
// worktree was manually removed cannot silently transition to done. An
// unclaimed coordinator-level story is still exempt, but a recorded claimant
// proves this story previously entered the claimed-worktree workflow and must
// require an explicit delivery-gate override when that worktree is gone.
func TestTransitionDoneClaimedStoryWithoutWorktreeFailsClosed_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-story-04", "--title", "Gate story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-story-04")
	_, err = runTrls(t, repo, "claim", "gate-story-04", "--worktree")
	require.NoError(t, err)

	// Simulate manual removal and pruning after claim. This removes the only
	// marker file, while materialized state still records the story as claimed.
	run(t, repo, "git", "worktree", "remove", wt, "--force")
	run(t, repo, "git", "worktree", "prune")

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-04", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "a materially assigned story with no discoverable claimed worktree must fail closed")
	assert.Contains(t, err.Error(), "claimed worktree")
	assert.Contains(t, err.Error(), "--skip-delivery-gate")
}

// TestTransitionDoneUnassignedThenRetypedToEpicStillGatesLingeringWorktree_REQ_LNGHZN_S4
// reproduces the open review-thread bug: a task is claimed (creating a
// worktree+marker), self-unassigned (clearing ClaimedBy), then amended to an
// unmapped type (epic) — WITHOUT the worktree marker ever being removed.
// Before the fix, the gate's default trigger was keyed off
// `ClaimedBy != "" || Type in {task,bug,feature}`: after unassign clears
// ClaimedBy and the amend retypes away from task/bug/feature, BOTH halves of
// that predicate go false, and the (task-only) story-specific worktree scan
// never runs for a non-story type, so the gate was skipped outright even
// though the worktree's own armature-issue-id marker still names this issue
// and the tree is dirty. deliverygateRequired must catch this by looking for
// a live binding first, independent of Type/ClaimedBy.
func TestTransitionDoneUnassignedThenRetypedToEpicStillGatesLingeringWorktree_REQ_LNGHZN_S4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-unassign-retype-01", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-unassign-retype-01")
	_, err = runTrls(t, repo, "claim", "gate-unassign-retype-01", "--worktree")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "unassign", "--issue", "gate-unassign-retype-01")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "amend", "gate-unassign-retype-01", "--type", "epic")
	require.NoError(t, err)

	// The lingering worktree still carries its armature-issue-id marker and
	// is now dirtied — the gate must still catch this instead of treating
	// the unassigned, retyped-to-epic issue as exempt.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-unassign-retype-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "unassign+retype-to-epic must not exempt a still-bound, dirty worktree from the delivery gate")
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestGateAppliesToBugIssueKindOnDone_REQ_LNGHZN_S4_T2 verifies that the delivery gate
// applies to issue kind "bug" the same way it applies to "task": bugs get a
// worktree+branch created on claim (see internal/materialize/branch.go's
// "bug" case) and are expected to go through the same scoped/committed
// worker workflow, so the gate must not be silently skipped for them.
func TestGateAppliesToBugIssueKindOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-bug-01", "--title", "Gate bug", "--type", "bug", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-bug-01")
	_, err = runTrls(t, repo, "claim", "gate-bug-01", "--worktree")
	require.NoError(t, err)

	// Dirty the tree without committing: the clean-tree check should block.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-bug-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestGateAppliesToFeatureIssueKindOnDone_REQ_LNGHZN_S4_T2 verifies that the delivery
// gate applies to issue kind "feature" the same way it applies to "task":
// features get a worktree+branch created on claim (see
// internal/materialize/branch.go's "feature" case) and are expected to go
// through the same scoped/committed worker workflow, so the gate must not be
// silently skipped for them.
func TestGateAppliesToFeatureIssueKindOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-feat-01", "--title", "Gate feature", "--type", "feature", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-feat-01")
	_, err = runTrls(t, repo, "claim", "gate-feat-01", "--worktree")
	require.NoError(t, err)

	// Dirty the tree without committing: the clean-tree check should block.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-feat-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestTransitionDoneNoBoundWorktreeFailsClosed_REQ_LNGHZN_S4_T2 verifies
// that the delivery gate fails closed (refuses the transition, does not skip
// silently) when the target issue does not exist in the authoritative ops
// stream — e.g. because no bound context could be resolved for it.
func TestTransitionDoneNoBoundWorktreeFailsClosed_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	// No `create` was run for "ghost-01": the issue has no authoritative op.
	_, err = runTrls(t, repo, "transition", "--issue", "ghost-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue not found in current ops")
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

	wt := filepath.Join(repo, ".worktrees", "gate-05")
	_, err = runTrls(t, repo, "claim", "gate-05", "--worktree")
	require.NoError(t, err)

	// Commit a file outside declared scope ("bar.go" is not in scope "foo.go").
	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "bar.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-05): add bar")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-05", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

// TestRunDeliveryGateCheck_AllThreeChecksFailSimultaneously verifies that
// when a worktree fails all three delivery-gate checks at once — dirty
// tree, an out-of-scope committed change, and no commit referencing the
// issue in the conventional format — the aggregated error surfaces all
// three remediations, not just the first one encountered.
func TestRunDeliveryGateCheck_AllThreeChecksFailSimultaneously(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-all-three", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-all-three")
	_, err = runTrls(t, repo, "claim", "gate-all-three", "--worktree")
	require.NoError(t, err)

	// Commit a file outside declared scope ("bar.go" is not in scope
	// "foo.go") with a message that does not reference the issue in the
	// conventional commit format — this alone fails both ScopeContainment
	// and CommitReference.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "bar.go")
	run(t, wt, "git", "commit", "-m", "misc change with no issue reference")

	// Leave an untracked, uncommitted file so the working tree is also
	// dirty — this fails CleanTree.
	require.NoError(t, os.WriteFile(filepath.Join(wt, "baz.txt"), []byte("scratch\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-all-three", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CleanTree:")
	assert.Contains(t, err.Error(), "ScopeContainment:")
	assert.Contains(t, err.Error(), "CommitReference:")
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

	wt1 := filepath.Join(repo, ".worktrees", "gate-06")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree")
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
	wt2 := filepath.Join(repo, ".worktrees", "gate-06")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree")
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

	wt := filepath.Join(repo, ".worktrees", "gate-07")
	_, err = runTrls(t, repo, "claim", "gate-07", "--worktree")
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

	wt := filepath.Join(repo, ".worktrees", "gate-09")
	_, err = runTrls(t, repo, "claim", "gate-09", "--worktree")
	require.NoError(t, err)

	// Simulate a pre-fix claim record: force the persisted parent-branch
	// config value for this task branch to the literal string "HEAD", as
	// would have been written before commit 978405cc's idempotency guard
	// existed.
	git := adapters.New(repo)
	require.NoError(t, git.SetGitConfig(deliverygate.ParentBranchConfigKey("gate-09"), "HEAD"))

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

	wtA := filepath.Join(repo, ".worktrees", "gate-10a")
	_, err = runTrls(t, repo, "claim", "gate-10a", "--worktree")
	require.NoError(t, err)

	wtB := filepath.Join(repo, ".worktrees", "gate-10b")
	_, err = runTrls(t, repo, "claim", "gate-10b", "--worktree")
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

// TestDeliveryGateBlocksWrongBranchCheckout_REQ_LNGHZN_S4_T2 verifies that the delivery
// gate fails closed when the claimed worktree's HEAD is on some branch other
// than the expected task/<issueID> branch. The armature-issue-id marker file
// checked by verifyIssueWorktreeBinding persists in .git regardless of which
// branch is checked out, so a worker could check out an unrelated scratch
// branch after claiming, commit clean, correctly-scoped, correctly-referenced
// changes there, and (absent this check) pass the gate even though the actual
// task branch the coordinator integrates never received the commit.
func TestDeliveryGateBlocksWrongBranchCheckout_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-11", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-11")
	_, err = runTrls(t, repo, "claim", "gate-11", "--worktree")
	require.NoError(t, err)

	// Check out an unrelated scratch branch in the worktree, then commit
	// clean, in-scope, correctly-referenced changes there instead of on
	// task/gate-11.
	run(t, wt, "git", "checkout", "-b", "scratch-branch")
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-11): add foo")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-11", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err, "transition must fail when HEAD is not on the expected task branch")
	assert.Contains(t, err.Error(), "scratch-branch")
	assert.Contains(t, err.Error(), "task/gate-11")
	assert.Contains(t, err.Error(), "--skip-delivery-gate")
}

// TestTransitionDoneFromWorktreeSubdirectory_REQ_LNGHZN_S4 verifies that
// `arm transition --to done` (with no --repo flag, so gateRepoPath defaults
// to ".") succeeds when run from a subdirectory of the claimed worktree, not
// only from the worktree's top level. Before the fix, gateRepoPath was
// passed unresolved into VerifyIssueWorktreeBinding -> worktree.ResolveGitDir,
// which stats "<gateRepoPath>/.git" with no walk-up, so running from a
// subdirectory failed with "stat .git: no such file or directory" even
// though the subdirectory genuinely is inside the bound worktree.
func TestTransitionDoneFromWorktreeSubdirectory_REQ_LNGHZN_S4(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-12", "--title", "Gate task", "--type", "task", "--scope", "sub/foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-12")
	_, err = runTrls(t, repo, "claim", "gate-12", "--worktree")
	require.NoError(t, err)

	subdir := filepath.Join(wt, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "foo.go"), []byte("package sub\n"), 0o644))
	run(t, wt, "git", "add", "sub/foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-12): add sub/foo")

	// Run transition with cwd inside the subdirectory of the worktree, and no
	// --repo flag, so gateRepoPath defaults to "." relative to that cwd.
	_, err = runTrls(t, subdir, "transition", "--issue", "gate-12", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "transition to done must succeed when run from a subdirectory of the bound worktree")
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

	wt := filepath.Join(repo, ".worktrees", "gate-08")
	_, err = runTrls(t, repo, "claim", "gate-08", "--worktree")
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
