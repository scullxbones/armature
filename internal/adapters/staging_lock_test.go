package adapters_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/testexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStagingLock_SerializesBeginRelease_REQ_435 proves the clone staging
// flock is held from begin through release: a peer BeginPerRunRef cannot
// proceed while another staging section is live. Forced channel interleaving
// (not sleep-based): holder acquires → peer starts → assert peer blocked →
// holder releases → peer acquires. Without the flock, order collapses to
// peer-acquired overlapping holder-acquired.
func TestStagingLock_SerializesBeginRelease_REQ_435(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	c := adapters.New(repo)

	holderEntered := make(chan struct{})
	releaseHolder := make(chan struct{})
	peerGotLock := make(chan struct{})
	peerStarted := make(chan struct{})

	var order []string
	var orderMu sync.Mutex
	record := func(s string) {
		orderMu.Lock()
		order = append(order, s)
		orderMu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ref, release, err := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
		if err != nil {
			t.Errorf("holder BeginPerRunRef: %v", err)
			return
		}
		if ref == "" {
			t.Error("holder BeginPerRunRef: empty ref")
			return
		}
		record("holder-acquired")
		close(holderEntered)
		<-releaseHolder
		release()
		record("holder-released")
	}()
	go func() {
		defer wg.Done()
		<-holderEntered
		go func() {
			close(peerStarted)
			ref, release, err := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
			if err != nil {
				t.Errorf("peer BeginPerRunRef: %v", err)
				return
			}
			record("peer-acquired")
			release()
			close(peerGotLock)
			_ = ref
		}()
		<-peerStarted
		select {
		case <-peerGotLock:
			t.Error("peer acquired staging lock while holder still live")
			close(releaseHolder)
			return
		case <-time.After(50 * time.Millisecond):
		}
		orderMu.Lock()
		for _, s := range order {
			if s == "peer-acquired" {
				t.Error("peer-acquired recorded while holder still live")
			}
		}
		orderMu.Unlock()
		close(releaseHolder)
		<-peerGotLock
	}()
	wg.Wait()

	orderMu.Lock()
	defer orderMu.Unlock()
	assert.Equal(t, []string{"holder-acquired", "holder-released", "peer-acquired"}, order)
}

// installOriginFetchGate wraps git so the first `fetch … origin …` blocks on
// FIFOs (entered then continue) before exec'ing the real binary. Callers
// coordinate by reading/writing those FIFOs — no sleep in the interleaving path.
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

// TestRestoreEvidence_StagingLockBlocksPeerReleaseMidFetch_REQ_435 is the
// forced-interleaving repro for issue #435. Race class: concurrent clone-local
// `git fetch` negotiates against sibling refs/armature-tmp|…|_armature-run
// refs; a peer release mid-fetch yields `fatal: bad object refs/…`. Fix: the
// staging flock is held from beginArmatureTmpNS / BeginPerRunRef through
// release. A FIFO-gated git wrapper pauses the first origin fetch (not sleep);
// a peer BeginPerRunRef must block until the victim finishes — the interleaving
// that produced bad-object before the flock.
func TestRestoreEvidence_StagingLockBlocksPeerReleaseMidFetch_REQ_435(t *testing.T) {
	t.Parallel()

	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "-b", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	tip := runGitOutput(t, fx.Origin, "rev-parse", "refs/heads/_armature")

	wrapperDir, enteredFIFO, continueFIFO := installOriginFetchGate(t)
	c := adapters.New(repo).
		WithEnviron([]string{"PATH=" + wrapperDir + string(os.PathListSeparator) + os.Getenv("PATH")}).
		WithNetworkTimeout(15 * time.Second)
	bundle := filepath.Join(t.TempDir(), "seed.bundle")
	// Bundle create must use real git (not the gated wrapper).
	require.NoError(t, adapters.New(repo).BundleCreateAdvertised(bundle, tip, "refs/heads/_armature"))

	peerFinished := make(chan struct{})
	var peerSawVictimStillFetching atomic.Bool
	allowFetch := make(chan struct{})

	var restoreErr error
	restoreDone := make(chan struct{})
	go func() {
		defer close(restoreDone)
		_, _, _, _, _, restoreErr = c.RestoreEvidence(bundle, "refs/heads/_armature", "_armature")
	}()

	enteredDone := make(chan error, 1)
	go func() {
		f, err := os.Open(enteredFIFO) // path under t.TempDir gate FIFOs
		if err != nil {
			enteredDone <- err
			return
		}
		buf := make([]byte, 16)
		_, err = f.Read(buf)
		_ = f.Close() //nolint:errcheck // best-effort
		enteredDone <- err
	}()
	select {
	case err := <-enteredDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated origin fetch to enter")
	}

	go func() {
		defer close(peerFinished)
		_, release, beginErr := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
		if beginErr != nil {
			t.Errorf("peer BeginPerRunRef: %v", beginErr)
			return
		}
		select {
		case <-allowFetch:
		default:
			peerSawVictimStillFetching.Store(true)
		}
		release()
	}()

	select {
	case <-peerFinished:
		t.Fatal("peer completed BeginPerRunRef while victim held staging lock mid-fetch")
	case <-time.After(50 * time.Millisecond):
	}

	close(allowFetch)
	cont, err := os.OpenFile(continueFIFO, os.O_WRONLY, 0) // path under t.TempDir gate FIFOs
	require.NoError(t, err)
	_, err = cont.Write([]byte("go\n"))
	require.NoError(t, err)
	require.NoError(t, cont.Close())

	<-restoreDone
	<-peerFinished

	require.NoError(t, restoreErr)
	assert.False(t, peerSawVictimStillFetching.Load(),
		"peer must not acquire staging lock while victim RestoreEvidence still mid-fetch")
}
