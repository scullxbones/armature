package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDAGTransitionCmd_RejectsInvalidToValue verifies that --to on dag-transition
// is validated as a confidence value (draft, verified), not silently accepted as
// an arbitrary string (which would otherwise stamp a nonsensical value like
// "done" into Provenance.Confidence — see surface-census.md dag-transition --to row).
func TestDAGTransitionCmd_RejectsInvalidToValue(t *testing.T) {
	repo := setupRepoWithDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"dag", "transition", "--repo", repo, "--issue", "draft-task-01", "--to", "done"})
	err := root.Execute()
	require.Error(t, err, "dag-transition --to done should be rejected: done is a status, not a confidence value")
	assert.Contains(t, err.Error(), "confidence")
}

// TestDAGTransitionCmd_AcceptsValidToValue verifies the legitimate confidence values
// still work.
func TestDAGTransitionCmd_AcceptsValidToValue(t *testing.T) {
	repo := setupRepoWithDraftNode(t)

	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"dag", "transition", "--repo", repo, "--issue", "draft-task-01", "--to", "verified"})
	require.NoError(t, root.Execute())
}
