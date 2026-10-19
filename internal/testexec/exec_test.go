package testexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRunsSidecarByAbsolutePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	Write(t, bin, []byte("#!/bin/sh\necho ok-abs\n"))
	out, err := exec.Command(bin).CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok-abs\n", string(out))
}

func TestWriteRunsSidecarWhenArgv0IsBasename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	Write(t, bin, []byte("#!/bin/sh\necho ok-path\n"))
	cmd := exec.Command(bin)
	cmd.Args[0] = "tool"
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok-path\n", string(out))
}

func TestWriteParallelInstallAndExec(t *testing.T) {
	t.Parallel()
	for i := 0; i < 32; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			bin := filepath.Join(dir, "tool")
			Write(t, bin, []byte("#!/bin/sh\nexit 0\n"))
			out, err := exec.Command(bin).CombinedOutput()
			require.NoError(t, err, "%s", out)
		})
	}
}
