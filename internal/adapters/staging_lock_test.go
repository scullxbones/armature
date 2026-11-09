package adapters_test

import (
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/gittest"
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
		require.NoError(t, err)
		require.NotEmpty(t, ref)
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
			ref, release, err := c.BeginPerRunRef("refs/remotes/origin/_armature-run")
			require.NoError(t, err)
			record("peer-acquired")
			release()
			close(peerGotLock)
			_ = ref
		}()
		select {
		case <-peerGotLock:
			t.Fatal("peer acquired staging lock while holder still live")
		case <-time.After(50 * time.Millisecond):
		}
		orderMu.Lock()
		for _, s := range order {
			require.NotEqual(t, "peer-acquired", s)
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

func gitHTTPBackend(t *testing.T) string {
	t.Helper()
	// Prefer the real exec-path helper, not PATH / a polluted GIT_EXEC_PATH.
	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	require.NoError(t, err)
	path := filepath.Join(strings.TrimSpace(string(out)), "git-http-backend")
	if _, statErr := os.Stat(path); statErr == nil {
		return path
	}
	backend, err := exec.LookPath("git-http-backend")
	require.NoError(t, err)
	return backend
}

func startGatedUploadPack(t *testing.T, originBare string, onUploadPackPost func()) (baseURL string) {
	t.Helper()
	projectRoot := filepath.Dir(originBare)
	originBase := filepath.Base(originBare)
	backend := gitHTTPBackend(t)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() }) //nolint:errcheck // best-effort

	var once sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/git-upload-pack") && r.Method == http.MethodPost {
			once.Do(func() {
				if onUploadPackPost != nil {
					onUploadPackPost()
				}
			})
		}
		h := &cgi.Handler{
			Path: backend,
			Env: []string{
				"GIT_PROJECT_ROOT=" + projectRoot,
				"GIT_HTTP_EXPORT_ALL=1",
			},
		}
		if !strings.HasPrefix(r.URL.Path, "/"+originBase) {
			r.URL.Path = "/" + originBase + r.URL.Path
		}
		h.ServeHTTP(w, r)
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }() //nolint:errcheck // test server
	t.Cleanup(func() { _ = srv.Close() }) //nolint:errcheck // best-effort
	return "http://" + ln.Addr().String() + "/" + originBase
}

// TestRestoreEvidence_StagingLockBlocksPeerReleaseMidFetch_REQ_435 is the
// forced-interleaving repro for issue #435. Race class: concurrent clone-local
// `git fetch` negotiates against sibling refs/armature-tmp|…|_armature-run
// refs; a peer release mid-fetch yields `fatal: bad object refs/…`. Fix: the
// staging flock is held from beginArmatureTmpNS / BeginPerRunRef through
// release. This test gates upload-pack on a channel (not sleep), starts a peer
// BeginPerRunRef while RestoreEvidence is mid-fetch, and asserts the peer
// cannot acquire until the victim finishes — the interleaving that produced
// bad-object before the flock.
func TestRestoreEvidence_StagingLockBlocksPeerReleaseMidFetch_REQ_435(t *testing.T) {
	t.Parallel()

	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "-b", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	tip := runGitOutput(t, fx.Origin, "rev-parse", "refs/heads/_armature")

	c := adapters.New(repo).WithNetworkTimeout(15 * time.Second)
	bundle := filepath.Join(t.TempDir(), "seed.bundle")
	require.NoError(t, c.BundleCreateAdvertised(bundle, tip, "refs/heads/_armature"))

	fetchEntered := make(chan struct{})
	allowFetch := make(chan struct{})
	peerFinished := make(chan struct{})
	var peerSawVictimStillFetching atomic.Bool

	baseURL := startGatedUploadPack(t, fx.Origin, func() {
		close(fetchEntered)
		<-allowFetch
	})
	gittest.Git(t, repo, "remote", "set-url", "origin", baseURL)

	var restoreErr error
	restoreDone := make(chan struct{})
	go func() {
		defer close(restoreDone)
		_, _, _, _, _, restoreErr = c.RestoreEvidence(bundle, "refs/heads/_armature", "_armature")
	}()

	<-fetchEntered
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
	<-restoreDone
	<-peerFinished

	require.NoError(t, restoreErr)
	assert.False(t, peerSawVictimStillFetching.Load(),
		"peer must not acquire staging lock while victim RestoreEvidence still mid-fetch")
}
