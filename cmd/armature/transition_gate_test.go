package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransitionCommandHasSkipFlag_REQ_LNGHZN_S4_T2 verifies that the
// transition command exposes the --skip-delivery-gate flag.
func TestTransitionCommandHasSkipFlag_REQ_LNGHZN_S4_T2(t *testing.T) {
	cmd := newTransitionCmd()

	flag := cmd.Flags().Lookup("skip-delivery-gate")
	if flag == nil {
		t.Fatal("--skip-delivery-gate flag not found on transition command")
	}
	if flag.Value.Type() != "bool" {
		t.Errorf("--skip-delivery-gate flag has wrong type: %s, expected bool", flag.Value.Type())
	}
}

// TestPayloadStructHasSkipField_REQ_LNGHZN_S4_T2 verifies that ops.Payload
// carries the SkippedDeliveryGate field used for the audit trail.
func TestPayloadStructHasSkipField_REQ_LNGHZN_S4_T2(t *testing.T) {
	payload := ops.Payload{SkippedDeliveryGate: true}
	if !payload.SkippedDeliveryGate {
		t.Error("SkippedDeliveryGate field not working as expected")
	}
}

// TestDeliveryGateBlocksUncleanTree_REQ_LNGHZN_S4_T2 verifies that transition
// to done is blocked when the worktree has uncommitted changes.
func TestDeliveryGateBlocksUncleanTree_REQ_LNGHZN_S4_T2(t *testing.T) {
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

// TestSkipDeliveryGateFlag_REQ_LNGHZN_S4_T2 verifies that --skip-delivery-gate
// allows transition to done even when gate checks would otherwise fail, and
// records the override in the transition op's payload.
func TestSkipDeliveryGateFlag_REQ_LNGHZN_S4_T2(t *testing.T) {
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
