package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICommand_NonInteractive(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := testRootCmd(t)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tui", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "board:")
}
