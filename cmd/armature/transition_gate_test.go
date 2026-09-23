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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-01): add foo")

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n// dirty\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

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

// TestTransitionDoneAcceptsSquashOnMainWhenClaimWorktreeStale_REQ_MATENC
// simulates the dogfood failure: evidence lives only on main after a
// squash-land, the claim worktree is still on the stale task branch, and
// `arm transition --to done` must succeed without --skip-delivery-gate.
func TestTransitionDoneAcceptsSquashOnMainWhenClaimWorktreeStale_REQ_MATENC(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	run(t, repo, "git", "branch", "-M", "main")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-squash-01", "--title", "Gate squash", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt := filepath.Join(repo, ".worktrees", "gate-squash-01")
	_, err = runTrls(t, repo, "claim", "gate-squash-01", "--worktree")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, repo, "git", "add", "foo.go")
	run(t, repo, "git", "commit", "-m", "feat(gate-squash-01): land squash (#217)")

	head, err := adapters.New(wt).CurrentBranch()
	require.NoError(t, err)
	require.Equal(t, "task/gate-squash-01", head, "claim worktree must remain on the stale task branch")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-squash-01", "--to", "done", "--outcome", "test", "--force")
	require.NoError(t, err, "main-only squash evidence must pass the delivery gate without --skip-delivery-gate")
}

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

	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
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

	allOps, _, err := readAllOpsFromDirWithOffsets(filepath.Join(getTestContext(t, repo).IssuesDir, "ops"))
	require.NoError(t, err)
	for _, op := range allOps {
		assert.False(t, op.Type == ops.OpTransition && op.TargetID == "gate-override-01", "invalid override must not append a transition op")
	}
}

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

	_, err = runTrls(t, wt, "transition", "--issue", "gate-04", "--to", "blocked", "--outcome", "waiting")
	assert.NoError(t, err)
}

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

	_, err = runTrls(t, repo, "amend", "gate-scope-01", "--scope", "foo.go")
	require.NoError(t, err)

	_, err = runTrls(t, wt, "transition", "--issue", "gate-scope-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "the amended scope must reject the committed bar.go without requiring materialization")
	assert.Contains(t, err.Error(), "bar.go")
}

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

	_, err = runTrls(t, repo, "amend", "gate-amended-epic-01", "--type", "epic")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-amended-epic-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "a claimed issue remains bound after its type changes and must be delivery-gated")
	assert.Contains(t, err.Error(), "delivery gate")
}

func TestGateSkippedForNonTaskIssueKindOnDone_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "story-01", "--title", "A story", "--type", "story", "--scope", "foo.go")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	run(t, repo, "git", "checkout", "-b", "feat/story-01")
	_, err = runTrls(t, repo, "transition", "--issue", "story-01", "--to", "done", "--outcome", "test")
	assert.NoError(t, err)
}

func TestTransitionDoneUnmaterializedStoryReplaysAuthoritativeOps_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--id", "gate-unmaterialized-story-01", "--title", "Gate story", "--type", "story")
	require.NoError(t, err)

	run(t, repo, "git", "checkout", "-b", "feat/gate-unmaterialized-story-01")
	_, err = runTrls(t, repo, "transition", "--issue", "gate-unmaterialized-story-01", "--to", "done", "--outcome", "test")
	require.NoError(t, err)

	issue, _, err := replayIssueOps(getTestContext(t, repo).IssuesDir, "gate-unmaterialized-story-01")
	require.NoError(t, err)
	assert.Equal(t, "done", issue.Status)
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-story-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-02", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "transitioning a claimed story to done from a different checkout must still run the delivery gate")
	assert.Contains(t, err.Error(), "Working tree is not clean", "must fail specifically on the clean-tree check this test dirtied, not some unrelated error")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-released-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "self-unassigning must not exempt a still-bound, dirty worktree from the delivery gate")
	assert.Contains(t, err.Error(), "delivery gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	run(t, wt, "git", "checkout", "--detach")

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-03", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "transitioning a claimed story to done must still run the delivery gate even when the claimed worktree is in a detached HEAD")
	assert.Contains(t, err.Error(), "gate-story-03")
	assert.Contains(t, err.Error(), "--skip-delivery-gate")
}

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

	run(t, repo, "git", "worktree", "remove", wt, "--force")
	run(t, repo, "git", "worktree", "prune")

	_, err = runTrls(t, repo, "transition", "--issue", "gate-story-04", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "a materially assigned story with no discoverable claimed worktree must fail closed")
	assert.Contains(t, err.Error(), "claimed worktree")
	assert.Contains(t, err.Error(), "--skip-delivery-gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, repo, "transition", "--issue", "gate-unassign-retype-01", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err, "unassign+retype-to-epic must not exempt a still-bound, dirty worktree from the delivery gate")
	assert.Contains(t, err.Error(), "delivery gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-bug-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-feat-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

func TestTransitionDoneNoBoundWorktreeFailsClosed_REQ_LNGHZN_S4_T2(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	_, err = runTrls(t, repo, "transition", "--issue", "ghost-01", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issue not found in current ops")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "bar.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-05): add bar")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-05", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delivery gate")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(wt, "bar.go"), []byte("package bar\n"), 0o644))
	run(t, wt, "git", "add", "bar.go")
	run(t, wt, "git", "commit", "-m", "misc change with no issue reference")

	require.NoError(t, os.WriteFile(filepath.Join(wt, "baz.txt"), []byte("scratch\n"), 0o644))

	_, err = runTrls(t, wt, "transition", "--issue", "gate-all-three", "--to", "done", "--outcome", "test", "--force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CleanTree:")
	assert.Contains(t, err.Error(), "ScopeContainment:")
	assert.Contains(t, err.Error(), "CommitReference:")
}

func TestDeliveryGateSurvivesWorktreeRecreation_REQ_LNGHZN_S4_T1(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	run(t, repo, "git", "checkout", "-b", "story-branch")

	_, err = runTrls(t, repo, "create", "--id", "gate-06", "--title", "Gate task", "--type", "task", "--scope", "foo.go")
	require.NoError(t, err)

	wt1 := filepath.Join(repo, ".worktrees", "gate-06")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree")
	require.NoError(t, err)

	run(t, repo, "git", "worktree", "remove", wt1, "--force")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(gate-sibling): unrelated sibling work")

	wt2 := filepath.Join(repo, ".worktrees", "gate-06")
	_, err = runTrls(t, repo, "claim", "gate-06", "--worktree")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt2, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt2, "git", "add", "foo.go")
	run(t, wt2, "git", "commit", "-m", "feat(gate-06): add foo")

	_, err = runTrls(t, wt2, "transition", "--issue", "gate-06", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "sibling commit added after worktree removal must not be misattributed as in-scope diff")
}

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

	require.NoError(t, os.WriteFile(filepath.Join(repo, "sibling.go"), []byte("package sibling\n"), 0o644))
	run(t, repo, "git", "add", "sibling.go")
	run(t, repo, "git", "commit", "-m", "feat(gate-sibling): unrelated sibling work")

	run(t, wt, "git", "rebase", "story-branch")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-07", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "sibling commit pulled in by rebase onto updated parent tip must not be misattributed as in-scope diff")
}

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

	git := adapters.New(repo)
	require.NoError(t, git.SetGitConfig(deliverygate.ParentBranchConfigKey("gate-09"), "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(wt, "foo.go"), []byte("package foo\n"), 0o644))
	run(t, wt, "git", "add", "foo.go")
	run(t, wt, "git", "commit", "-m", "feat(gate-09): add foo")

	_, err = runTrls(t, wt, "transition", "--issue", "gate-09", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "stale literal-HEAD parent-branch config must self-heal via fallback, not collapse the merge-base range")
}

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

	_, err = runTrls(t, wtB, "transition", "--issue", "gate-10a", "--to", "done", "--outcome", "test", "--force")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gate-10b")

	_, err = runTrls(t, wtA, "transition", "--issue", "gate-10a", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err)
}

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

	_, err = runTrls(t, subdir, "transition", "--issue", "gate-12", "--to", "done", "--outcome", "test", "--force")
	assert.NoError(t, err, "transition to done must succeed when run from a subdirectory of the bound worktree")
}

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
