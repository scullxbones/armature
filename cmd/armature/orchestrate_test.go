package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrchestrateCmd_Registered verifies the "orchestrate" command is wired into the root.
func TestOrchestrateCmd_Registered(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)
	assert.Equal(t, "orchestrate", cmd.Name())
}

// TestOrchestrateCmd_FlagsMissingIssue verifies that omitting --issue returns an error.
func TestOrchestrateCmd_FlagsMissingIssue(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"orchestrate", "--repo", repo})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--issue")
}

// TestOrchestrateCmd_RequiredFlags checks that all expected flags are registered.
func TestOrchestrateCmd_RequiredFlags(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)

	flags := cmd.Flags()
	for _, name := range []string{"issue", "harness", "model", "retries", "timeout", "dry-run", "show-network-plan", "auth-check"} {
		assert.NotNil(t, flags.Lookup(name), "expected flag --%s to be registered", name)
	}
}

// TestOrchestrateCmd_HarnessDefault verifies the --harness flag defaults to "claude".
func TestOrchestrateCmd_HarnessDefault(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)
	f := cmd.Flags().Lookup("harness")
	require.NotNil(t, f)
	assert.Equal(t, "claude", f.DefValue)
}

// TestResolveModel_FlagWins verifies that the --model flag takes top priority.
func TestResolveModel_FlagWins(t *testing.T) {
	t.Parallel()
	result := resolveModel("flag-model", "task-model", "config-model")
	assert.Equal(t, "flag-model", result)
}

// TestResolveModel_TaskModelWins verifies that task.PreferredModel is used when no flag is set.
func TestResolveModel_TaskModelWins(t *testing.T) {
	t.Parallel()
	result := resolveModel("", "task-model", "config-model")
	assert.Equal(t, "task-model", result)
}

// TestResolveModel_ConfigDefault verifies that config default is used when no flag or task model.
func TestResolveModel_ConfigDefault(t *testing.T) {
	t.Parallel()
	result := resolveModel("", "", "config-model")
	assert.Equal(t, "config-model", result)
}

// TestResolveModel_AllEmpty verifies that empty string is returned when no model is set anywhere.
func TestResolveModel_AllEmpty(t *testing.T) {
	t.Parallel()
	result := resolveModel("", "", "")
	assert.Equal(t, "", result)
}

// TestOrchestrateCmd_DryRunFlag verifies that --dry-run flag type is bool.
func TestOrchestrateCmd_DryRunFlag(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)
	f := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, f)
	assert.Equal(t, "bool", f.Value.Type())
}

// TestOrchestrateCmd_RetriesDefault verifies the --retries flag defaults to 3.
func TestOrchestrateCmd_RetriesDefault(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)
	f := cmd.Flags().Lookup("retries")
	require.NotNil(t, f)
	assert.Equal(t, "3", f.DefValue)
}

// TestOrchestrateCmd_TimeoutDefault verifies the --timeout flag defaults to 0 (no limit).
func TestOrchestrateCmd_TimeoutDefault(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"orchestrate"})
	require.NoError(t, err)
	f := cmd.Flags().Lookup("timeout")
	require.NotNil(t, f)
	assert.Equal(t, "0", f.DefValue)
}

// TestBuildOrchestateJSONPayload_BasicFields verifies the standard fields are always present.
func TestBuildOrchestateJSONPayload_BasicFields(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{
		Phase:      "complete",
		Run:        1,
		Model:      "claude-3",
		Harness:    "claude",
		AuthSource: "env",
	}
	payload := buildOrchestateJSONPayload("TASK-001", false, result)
	assert.Equal(t, "TASK-001", payload["issue"])
	assert.Equal(t, "complete", payload["phase"])
	assert.Equal(t, 1, payload["run"])
	assert.Equal(t, false, payload["dry_run"])
	assert.Equal(t, "claude-3", payload["model"])
	assert.Equal(t, "claude", payload["harness"])
	assert.Equal(t, "env", payload["auth_source"])
}

// TestBuildOrchestateJSONPayload_BlockedReasonIncluded verifies blocked_reason is present when set.
func TestBuildOrchestateJSONPayload_BlockedReasonIncluded(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{
		BlockedReason: "task claimed by worker-42",
	}
	payload := buildOrchestateJSONPayload("TASK-002", false, result)
	assert.Equal(t, "task claimed by worker-42", payload["blocked_reason"])
}

// TestBuildOrchestateJSONPayload_BlockedReasonOmittedWhenEmpty verifies blocked_reason is absent when empty.
func TestBuildOrchestateJSONPayload_BlockedReasonOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{}
	payload := buildOrchestateJSONPayload("TASK-003", false, result)
	_, present := payload["blocked_reason"]
	assert.False(t, present, "blocked_reason should be omitted when empty")
}

// TestBuildOrchestateJSONPayload_WouldDispatchIncluded verifies would_dispatch is present when false (blocked).
func TestBuildOrchestateJSONPayload_WouldDispatchIncluded(t *testing.T) {
	t.Parallel()
	resultBlocked := orchestrate.RunResult{
		WouldDispatch: false,
		BlockedReason: "scope conflict",
	}
	payload := buildOrchestateJSONPayload("TASK-004", false, resultBlocked)
	assert.Equal(t, false, payload["would_dispatch"])

	resultDispatching := orchestrate.RunResult{
		WouldDispatch: true,
	}
	payload2 := buildOrchestateJSONPayload("TASK-004", false, resultDispatching)
	assert.Equal(t, true, payload2["would_dispatch"])
}

// TestBuildOrchestateJSONPayload_ScopeConflictsIncluded verifies scope_conflicts is included when non-empty.
func TestBuildOrchestateJSONPayload_ScopeConflictsIncluded(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{
		BlockedReason: "scope conflict",
		ScopeConflicts: []orchestrate.ScopeConflict{
			{TaskID: "TASK-999", Paths: []string{"cmd/foo.go", "internal/bar.go"}},
		},
	}
	payload := buildOrchestateJSONPayload("TASK-005", false, result)
	conflicts, ok := payload["scope_conflicts"].([]orchestrate.ScopeConflict)
	require.True(t, ok, "scope_conflicts should be []orchestrate.ScopeConflict")
	require.Len(t, conflicts, 1)
	assert.Equal(t, "TASK-999", conflicts[0].TaskID)
	assert.Equal(t, []string{"cmd/foo.go", "internal/bar.go"}, conflicts[0].Paths)
}

// TestBuildOrchestateJSONPayload_ScopeConflictsOmittedWhenEmpty verifies scope_conflicts is absent when empty.
func TestBuildOrchestateJSONPayload_ScopeConflictsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{}
	payload := buildOrchestateJSONPayload("TASK-006", false, result)
	_, present := payload["scope_conflicts"]
	assert.False(t, present, "scope_conflicts should be omitted when empty")
}

// TestBuildOrchestateJSONPayload_WouldClaimAndClaimOwner verifies would_claim and claim_owner are included when set.
func TestBuildOrchestateJSONPayload_WouldClaimAndClaimOwner(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{
		WouldClaim: true,
		ClaimOwner: "worker-99",
	}
	payload := buildOrchestateJSONPayload("TASK-007", false, result)
	assert.Equal(t, true, payload["would_claim"])
	assert.Equal(t, "worker-99", payload["claim_owner"])
}

// TestBuildOrchestateJSONPayload_WouldClaimOmittedWhenFalseAndNoOwner verifies would_claim and claim_owner absent when zero.
func TestBuildOrchestateJSONPayload_WouldClaimOmittedWhenFalseAndNoOwner(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{}
	payload := buildOrchestateJSONPayload("TASK-008", false, result)
	_, wcPresent := payload["would_claim"]
	_, coPresent := payload["claim_owner"]
	assert.False(t, wcPresent, "would_claim should be omitted when false")
	assert.False(t, coPresent, "claim_owner should be omitted when empty")
}

// TestBuildOrchestateJSONPayload_IsJSONSerializable verifies the payload round-trips through JSON cleanly.
func TestBuildOrchestateJSONPayload_IsJSONSerializable(t *testing.T) {
	t.Parallel()
	result := orchestrate.RunResult{
		Phase:         "complete",
		Run:           2,
		WouldDispatch: true,
		WouldClaim:    true,
		ClaimOwner:    "worker-1",
		Harness:       "claude",
		Model:         "claude-3",
		AuthSource:    "env",
	}
	payload := buildOrchestateJSONPayload("TASK-009", false, result)
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "payload must produce valid JSON")
}
