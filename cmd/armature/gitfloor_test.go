package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/testexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFloor_WorkerInitRejectsOldGit_CLI_REQ_GIT234_MIN(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	testexec.Write(t, filepath.Join(bin, "git"), []byte("#!/bin/sh\necho 'git version 2.33.0'\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	repo := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := executeThenHandleRootError(t, &stdout, &stderr, "--repo", repo, "worker-init")
	require.NotEqual(t, 0, code, "worker-init must fail closed under git 2.33.0")
	out := stdout.String() + stderr.String()
	assert.Contains(t, out, "2.33.0")
	assert.Contains(t, out, "2.34.0")
	assert.Contains(t, out, "upgrade Git to at least 2.34.0")
	assert.Contains(t, out, "docs/getting-started.md")
}

func TestGitFloor_ReadyRejectsOldGit_CLI_REQ_GIT234_MIN(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	testexec.Write(t, filepath.Join(bin, "git"), []byte("#!/bin/sh\necho 'git version 2.33.0'\n"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	repo := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := executeThenHandleRootError(t, &stdout, &stderr, "--repo", repo, "ready")
	require.NotEqual(t, 0, code, "ready must fail closed under git 2.33.0")
	out := stdout.String() + stderr.String()
	assert.Contains(t, out, "IO")
	assert.Contains(t, out, "upgrade Git to at least 2.34.0")
	assert.Contains(t, out, "docs/getting-started.md")
}

func TestGitFloorCommandFailure_NextActions_REQ_GIT234_MIN(t *testing.T) {
	t.Parallel()
	err := gitFloorCommandFailure(errors.New("installed git 2.33.0 is below the required minimum git 2.34.0"))
	require.Error(t, err)
	var cf *armerrors.CommandFailure
	require.True(t, errors.As(err, &cf))
	assert.Equal(t, armerrors.CodeIO, cf.Code)
	assert.Equal(t, []string{
		"upgrade Git to at least 2.34.0",
		"see docs/getting-started.md",
	}, cf.NextActions)
	assert.Contains(t, cf.Cause, "2.33.0")
}
