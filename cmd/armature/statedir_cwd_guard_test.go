package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStateDirPorcelainGuard_FailsOnCwdWrite documents the class guard used by
// TestMain: a materialize write into the package cwd is visible both as a
// local Stat leak and as `git status --porcelain` on those paths. Not
// parallel — it plants a file in the shared package directory briefly.
func TestStateDirPorcelainGuard_FailsOnCwdWrite(t *testing.T) {
	require.Empty(t, packageCwdStateDirLeak(), "precondition: package cwd must be clean")
	require.Empty(t, stateDirPorcelainDirty(), "precondition: porcelain must be clean for state-dir paths")

	require.NoError(t, os.WriteFile("checkpoint.json", []byte(`{"state_version":1}`), 0o600))
	t.Cleanup(func() {
		swallowErr(os.Remove("checkpoint.json"))
	})

	assert.Contains(t, packageCwdStateDirLeak(), "checkpoint.json")
	assert.Contains(t, stateDirPorcelainDirty(), "checkpoint.json",
		"guard must fail git status --porcelain when a test writes state-dir files into cwd")
}
