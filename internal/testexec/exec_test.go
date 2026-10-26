package testexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fatalRecorder struct {
	*testing.T
	failed bool
	msg    string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failed = true
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func (r *fatalRecorder) Fatal(args ...any) {
	r.failed = true
	r.msg = fmt.Sprint(args...)
	runtime.Goexit()
}

func (r *fatalRecorder) FailNow() {
	r.failed = true
	runtime.Goexit()
}

func writeAndCapture(t *testing.T, path string, content []byte) *fatalRecorder {
	t.Helper()
	rec := &fatalRecorder{T: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		Write(rec, path, content)
	}()
	<-done
	return rec
}

func restoreTrampoline(t *testing.T) {
	t.Helper()
	mkdirTemp = os.MkdirTemp
	writeFile = os.WriteFile
	remove = os.Remove
	link = os.Link
	symlink = os.Symlink
	lstat = os.Lstat
	chmod = os.Chmod
	prepareMu.Lock()
	prepared = false
	trampolinePath = ""
	prepareErr = nil
	prepareMu.Unlock()
	require.NoError(t, prepare())
}

func TestAbortIfPanicsOnError(t *testing.T) {
	assert.Panics(t, func() { abortIf(errors.New("boom")) })
	assert.NotPanics(t, func() { abortIf(nil) })
}

func TestWriteRunsSidecarByAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	Write(t, bin, []byte("#!/bin/sh\necho ok-abs\n"))
	out, err := exec.CommandContext(t.Context(), bin).CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok-abs\n", string(out))
}

func TestWriteRunsSidecarWhenArgv0IsBasename(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	Write(t, bin, []byte("#!/bin/sh\necho ok-path\n"))
	cmd := exec.CommandContext(t.Context(), bin)
	cmd.Args[0] = "tool"
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok-path\n", string(out))
}

func TestWriteRelativeNameAfterChdir(t *testing.T) {
	t.Chdir(t.TempDir())
	Write(t, "tool", []byte("#!/bin/sh\necho ok-rel\n"))
	out, err := exec.CommandContext(t.Context(), "./tool").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "ok-rel\n", string(out))
}

func TestWriteReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(bin, []byte("stale"), 0o600))
	Write(t, bin, []byte("#!/bin/sh\necho replaced\n"))
	out, err := exec.CommandContext(t.Context(), bin).CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "replaced\n", string(out))
}

func TestWriteFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))
	rec := writeAndCapture(t, filepath.Join(parent, "tool"), []byte("#!/bin/sh\nexit 0\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "mkdir")
}

func TestWriteFailsWhenSidecarPathIsDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	require.NoError(t, os.Mkdir(bin+".script", 0o750))
	rec := writeAndCapture(t, bin, []byte("#!/bin/sh\nexit 0\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "sidecar")
}

func TestWriteFailsWhenDestIsNonemptyDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	require.NoError(t, os.Mkdir(bin, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "inside"), []byte("x"), 0o600))
	rec := writeAndCapture(t, bin, []byte("#!/bin/sh\nexit 0\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "remove")
}

func TestWriteUsesSymlinkWhenLinkFails(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	link = func(string, string) error { return errors.New("cross-device") }
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	Write(t, bin, []byte("#!/bin/sh\necho via-symlink\n"))
	out, err := exec.CommandContext(t.Context(), bin).CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "via-symlink\n", string(out))
}

func TestWriteFailsWhenLinkAndSymlinkFail(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	link = func(string, string) error { return errors.New("no link") }
	symlink = func(string, string) error { return errors.New("no symlink") }
	dir := t.TempDir()
	rec := writeAndCapture(t, filepath.Join(dir, "tool"), []byte("#!/bin/sh\nexit 0\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "link")
}

func TestPrepareMkdirTempError(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	prepareMu.Lock()
	prepared = false
	trampolinePath = ""
	prepareErr = nil
	prepareMu.Unlock()
	mkdirTemp = func(string, string) (string, error) { return "", errors.New("no tmp") }
	err := prepare()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir trampoline")
	rec := writeAndCapture(t, filepath.Join(t.TempDir(), "tool"), []byte("#!/bin/sh\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "mkdir trampoline")
}

func TestPrepareWriteFileError(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	prepareMu.Lock()
	prepared = false
	trampolinePath = ""
	prepareErr = nil
	prepareMu.Unlock()
	writeFile = func(string, []byte, os.FileMode) error { return errors.New("rofs") }
	err := prepare()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write trampoline")
}

func TestWriteFailsWhenTrampolinePathEmpty(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	prepareMu.Lock()
	prepared = true
	prepareErr = nil
	trampolinePath = ""
	prepareMu.Unlock()
	rec := writeAndCapture(t, filepath.Join(t.TempDir(), "tool"), []byte("#!/bin/sh\n"))
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "was not prepared")
}

func chmodDirAndCapture(t *testing.T, path string, mode os.FileMode) *fatalRecorder {
	t.Helper()
	rec := &fatalRecorder{T: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ChmodDir(rec, path, mode)
	}()
	<-done
	return rec
}

func TestChmodDirSetsDirectoryMode(t *testing.T) {
	dir := t.TempDir()
	ChmodDir(t, dir, 0o500)
	info, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o500), info.Mode().Perm())
	ChmodDir(t, dir, 0o755)
	info, err = os.Lstat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestChmodDirFailsOnFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	rec := chmodDirAndCapture(t, path, 0o755)
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "not a directory")
}

func TestChmodDirFailsWhenMissing(t *testing.T) {
	rec := chmodDirAndCapture(t, filepath.Join(t.TempDir(), "missing"), 0o755)
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "stat")
}

func TestChmodDirFailsWhenLstatErrors(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	lstat = func(string) (os.FileInfo, error) { return nil, errors.New("enoent") }
	rec := chmodDirAndCapture(t, t.TempDir(), 0o755)
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "stat")
}

func TestChmodDirFailsWhenChmodErrors(t *testing.T) {
	t.Cleanup(func() { restoreTrampoline(t) })
	chmod = func(string, os.FileMode) error { return errors.New("eperm") }
	rec := chmodDirAndCapture(t, t.TempDir(), 0o755)
	require.True(t, rec.failed)
	assert.Contains(t, rec.msg, "chmod")
}

func TestWriteParallelInstallAndExec(t *testing.T) {
	for i := 0; i < 32; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "tool")
			Write(t, bin, []byte("#!/bin/sh\nexit 0\n"))
			out, err := exec.CommandContext(t.Context(), bin).CombinedOutput()
			require.NoError(t, err, "%s", out)
		})
	}
}
