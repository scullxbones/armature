package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUICommand_NonInteractive(t *testing.T) {
	repo := setupRepoWithTask(t)

	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tui", "--repo", repo})

	err := cmd.Execute()
	require.NoError(t, err)
	// In non-interactive mode (tests run without a TTY), print summary
	assert.Contains(t, buf.String(), "board:")
}
