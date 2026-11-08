package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hostRefReadURLA = "https://github.com/octocat/hello-world/issues/1"
	hostRefReadURLB = "https://github.com/octocat/hello-world/issues/2"
)

func TestArmRefReadPrintsStdout_REQ_HOSTREF_S1_T6(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	withGitHubIssueTransport(t, http.StatusOK, map[string]any{
		"title": "Plan title",
		"body":  "Plan body line",
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	before := workerLogSnapshot(t, repo)
	out, err := runTrls(t, repo, "ref-read", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Equal(t, "Plan title\n\nPlan body line\n", out)
	assert.Equal(t, before, workerLogSnapshot(t, repo), "ref-read must append no op")
}

func TestArmRefReadRejectsUnknownURL_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	before := workerLogSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01", "--url", hostRefReadURLB)
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, hostRefReadURLB)
	assert.NotContains(t, combined, "test-token")
	assert.Equal(t, before, workerLogSnapshot(t, repo), "rejected ref-read must append no op")
}

func TestArmRefReadJSON_REQ_HOSTREF_S1_T6(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "support")
	withGitHubIssueTransport(t, http.StatusOK, map[string]any{
		"title": "JSON title",
		"body":  "JSON body",
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	out, err := runTrls(t, repo, "ref-read", "task-01", "--format", "json")
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &got))
	assert.Equal(t, map[string]string{
		"url":   hostRefReadURLA,
		"title": "JSON title",
		"body":  "JSON body",
	}, got)
}

func TestArmRefReadURLSelectsLiveName_REQ_HOSTREF_S1_T6(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "support")
	plantHostRef(t, repo, hostRefReadURLB, "engineering")
	withGitHubIssueTransport(t, http.StatusOK, map[string]any{
		"title": "Older live",
		"body":  "still live",
	}, func(t *testing.T, req *http.Request) {
		t.Helper()
		assert.Equal(t, "https://api.github.com/repos/octocat/hello-world/issues/1", req.URL.String())
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	out, err := runTrls(t, repo, "ref-read", "task-01", "--url", hostRefReadURLA, "--format", "human")
	require.NoError(t, err)
	assert.Equal(t, "Older live\n\nstill live\n", out)
}

func TestArmRefReadMissingToken_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	setCLIEnv(t, "GITHUB_TOKEN", "")

	before := workerLogSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, "GITHUB_TOKEN")
	assert.Equal(t, before, workerLogSnapshot(t, repo), "missing token must append no op")
}

func TestArmRefReadNoCurrentHostRef_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, strings.ToLower(combined), "host")
}

func TestArmRefReadHyphenAllowlist_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	root := testRootCmd(t)
	cmd, _, err := root.Find([]string{"ref-read"})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Equal(t, "ref-read", cmd.Name())
}

func plantHostRef(t *testing.T, repo, url, role string) {
	t.Helper()
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpExternalRef,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{ExternalRef: url, ExternalRole: role},
	}))
}

func withGitHubIssueTransport(t *testing.T, status int, payload map[string]any, inspect ...func(*testing.T, *http.Request)) {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if len(inspect) > 0 && inspect[0] != nil {
			inspect[0](t, req)
		}
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err = w.Write(body)
		assert.NoError(t, err)
	})
	prev := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Result(), nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = prev
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestArmRefReadRejectsWithdrawnURL_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)
	require.NoError(t, ops.AppendOp(logPath, ops.Op{
		Type:      ops.OpExternalRef,
		TargetID:  "task-01",
		Timestamp: nowEpoch(),
		WorkerID:  workerID,
		Payload:   ops.Payload{WithdrawExternalRef: hostRefReadURLA},
	}))
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	_, _, err = runTrlsWithStderr(t, repo, "ref-read", "task-01", "--url", hostRefReadURLA)
	require.Error(t, err)
}
