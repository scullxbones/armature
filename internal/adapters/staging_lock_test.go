package adapters_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/testexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingLock_SerializesFetchAgainstPeerRelease_REQ_435(t *testing.T) {
	t.Parallel()

	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "-b", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	tip := runGitOutput(t, fx.Origin, "rev-parse", "refs/heads/_armature")

	c := adapters.New(repo)
	peerRef, peerRelease, err := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
	require.NoError(t, err)
	require.NoError(t, c.UpdateRef(peerRef, tip))

	wrapperDir, enteredFIFO, continueFIFO := installOriginFetchGate(t)
	gated := c.WithEnviron([]string{"PATH=" + wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH")}).
		WithNetworkTimeout(15 * time.Second)

	victimRef, victimRelease, err := gated.BeginPerRunRef("refs/remotes/origin/_armature-run")
	require.NoError(t, err)
	defer victimRelease()

	peerReleaseDone := make(chan struct{})
	var peerReleasedDuringFetch atomic.Bool
	allowFetch := make(chan struct{})

	var fetchErr error
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		fetchErr = gated.FetchBranchToRef("_armature", victimRef)
	}()

	enteredDone := make(chan error, 1)
	go func() {
		f, openErr := os.Open(enteredFIFO)
		if openErr != nil {
			enteredDone <- openErr
			return
		}
		buf := make([]byte, 16)
		_, readErr := f.Read(buf)
		swallowErr(f.Close())
		enteredDone <- readErr
	}()
	select {
	case e := <-enteredDone:
		require.NoError(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated origin fetch to enter")
	}

	go func() {
		defer close(peerReleaseDone)
		peerRelease()
		select {
		case <-allowFetch:
		default:
			peerReleasedDuringFetch.Store(true)
		}
	}()

	select {
	case <-peerReleaseDone:
		t.Fatal("peer release completed while victim fetch still held staging lock")
	case <-time.After(50 * time.Millisecond):
	}

	close(allowFetch)
	cont, err := os.OpenFile(continueFIFO, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = cont.Write([]byte("go\n"))
	require.NoError(t, err)
	require.NoError(t, cont.Close())

	<-fetchDone
	<-peerReleaseDone

	require.NoError(t, fetchErr)
	assert.False(t, peerReleasedDuringFetch.Load(),
		"peer must not delete staging refs while victim FetchBranchToRef holds the flock")
	got, resolveErr := gated.ResolveRevision(victimRef)
	require.NoError(t, resolveErr)
	assert.Equal(t, tip, got)
}

func TestRestoreEvidence_StagingLockBlocksPeerReleaseMidFetch_REQ_435(t *testing.T) {
	t.Parallel()

	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "-b", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	tip := runGitOutput(t, fx.Origin, "rev-parse", "refs/heads/_armature")

	c := adapters.New(repo)
	peerRef, peerRelease, err := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
	require.NoError(t, err)
	require.NoError(t, c.UpdateRef(peerRef, tip))

	wrapperDir, enteredFIFO, continueFIFO := installOriginFetchGate(t)
	gated := c.WithEnviron([]string{"PATH=" + wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH")}).
		WithNetworkTimeout(15 * time.Second)
	bundle := filepath.Join(t.TempDir(), "seed.bundle")
	require.NoError(t, adapters.New(repo).BundleCreateAdvertised(bundle, tip, "refs/heads/_armature"))

	peerReleaseDone := make(chan struct{})
	var peerReleasedDuringFetch atomic.Bool
	allowFetch := make(chan struct{})

	var restoreErr error
	restoreDone := make(chan struct{})
	go func() {
		defer close(restoreDone)
		_, _, _, _, _, restoreErr = gated.RestoreEvidence(bundle, "refs/heads/_armature", "_armature")
	}()

	enteredDone := make(chan error, 1)
	go func() {
		f, openErr := os.Open(enteredFIFO)
		if openErr != nil {
			enteredDone <- openErr
			return
		}
		buf := make([]byte, 16)
		_, readErr := f.Read(buf)
		swallowErr(f.Close())
		enteredDone <- readErr
	}()
	select {
	case e := <-enteredDone:
		require.NoError(t, e)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated origin fetch to enter")
	}

	go func() {
		defer close(peerReleaseDone)
		peerRelease()
		select {
		case <-allowFetch:
		default:
			peerReleasedDuringFetch.Store(true)
		}
	}()

	select {
	case <-peerReleaseDone:
		t.Fatal("peer release completed while victim RestoreEvidence still held staging lock")
	case <-time.After(50 * time.Millisecond):
	}

	close(allowFetch)
	cont, err := os.OpenFile(continueFIFO, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = cont.Write([]byte("go\n"))
	require.NoError(t, err)
	require.NoError(t, cont.Close())

	<-restoreDone
	<-peerReleaseDone

	require.NoError(t, restoreErr)
	assert.False(t, peerReleasedDuringFetch.Load(),
		"peer must not delete staging refs while victim RestoreEvidence holds the flock")
}

func installOriginFetchGate(t *testing.T) (wrapperDir, enteredFIFO, continueFIFO string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	gateDir := t.TempDir()
	enteredFIFO = filepath.Join(gateDir, "entered")
	continueFIFO = filepath.Join(gateDir, "continue")
	require.NoError(t, exec.CommandContext(t.Context(), "mkfifo", enteredFIFO).Run())
	require.NoError(t, exec.CommandContext(t.Context(), "mkfifo", continueFIFO).Run())

	wrapperDir = t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
entered=%q
cont=%q
is_fetch=
seen_origin=
for a in "$@"; do
  case "$a" in
    fetch) is_fetch=1 ;;
    origin) seen_origin=1 ;;
  esac
done
if [ -n "$is_fetch" ] && [ -n "$seen_origin" ]; then
  if [ ! -f "$entered.tripped" ]; then
    : > "$entered.tripped"
    echo ready > "$entered"
    cat "$cont" >/dev/null
  fi
fi
exec "$real_git" "$@"
`, realGit, enteredFIFO, continueFIFO)
	testexec.Write(t, filepath.Join(wrapperDir, "git"), []byte(script))
	return wrapperDir, enteredFIFO, continueFIFO
}
