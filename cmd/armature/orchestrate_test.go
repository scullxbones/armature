package main

import (
	"bytes"
	"testing"

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
