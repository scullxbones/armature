package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionCommand_Bash(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "bash"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.NotEmpty(t, out, "bash completion output should not be empty")
	assert.True(t, strings.Contains(out, "bash"), "bash completion should contain 'bash'")
}

func TestCompletionCommand_Zsh(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "zsh"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.NotEmpty(t, out, "zsh completion output should not be empty")
}

func TestCompletionCommand_Fish(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "fish"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.NotEmpty(t, out, "fish completion output should not be empty")
}

func TestCompletionCommand_PowerShell(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "powershell"})

	err := root.Execute()
	require.NoError(t, err)
	out := buf.String()
	assert.NotEmpty(t, out, "powershell completion output should not be empty")
}

func TestCompletionCommand_UnknownShell(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "unknownshell"})

	err := root.Execute()
	assert.Error(t, err, "unknown shell should return an error")
}

func TestCompletionCommand_NoArgs(t *testing.T) {
	buf := new(bytes.Buffer)
	root := newRootCmd()
	root.SetOut(buf)
	root.SetArgs([]string{"completion"})

	err := root.Execute()
	assert.Error(t, err, "missing shell argument should return an error")
}
