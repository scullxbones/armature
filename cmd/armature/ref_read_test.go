package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
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

	beforeLogs := workerLogSnapshot(t, repo)
	beforeState := armatureStateTreeSnapshot(t, repo)
	out, err := runTrls(t, repo, "ref-read", "task-01", "--format", "human")
	require.NoError(t, err)
	assert.Equal(t, "Plan title\n\nPlan body line\n", out)
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "ref-read must append no op")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "successful ref-read must write no state files")
}

func TestArmRefReadRejectsUnknownURL_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	beforeLogs := workerLogSnapshot(t, repo)
	beforeState := armatureStateTreeSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01", "--url", hostRefReadURLB)
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, hostRefReadURLB)
	assert.NotContains(t, combined, "test-token")
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "rejected ref-read must append no op")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "rejected unknown URL must write no state files")
}

func TestArmRefReadUnknownIssueLeavesState_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	beforeLogs := workerLogSnapshot(t, repo)
	beforeState := armatureStateTreeSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "missing-issue")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, "missing-issue")
	assert.Contains(t, strings.ToLower(combined), "not found")
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "unknown issue must append no op")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "unknown issue must leave on-disk state unchanged")
}

func TestArmRefReadFetchFailureLeavesState_REQ_HOSTREF_S1_T6(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	withGitHubIssueTransport(t, http.StatusInternalServerError, map[string]any{
		"message": "boom",
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	beforeLogs := workerLogSnapshot(t, repo)
	beforeState := armatureStateTreeSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, "HTTP")
	assert.NotContains(t, combined, "test-token")
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "fetch failure must append no op")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "fetch failure must leave on-disk state unchanged")
}

func TestArmRefReadJSON_REQ_HOSTREF_S1_T6(t *testing.T) {
	repo := setupRepoWithTask(t)
	plantHostRef(t, repo, hostRefReadURLA, "support")
	withGitHubIssueTransport(t, http.StatusOK, map[string]any{
		"title": "JSON title",
		"body":  "JSON body",
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	beforeState := armatureStateTreeSnapshot(t, repo)
	out, err := runTrls(t, repo, "ref-read", "task-01", "--format", "json")
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &got))
	assert.Equal(t, map[string]string{
		"url":   hostRefReadURLA,
		"title": "JSON title",
		"body":  "JSON body",
	}, got)
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "json ref-read must write no state files")
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

	beforeLogs := workerLogSnapshot(t, repo)
	beforeState := armatureStateTreeSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, combined, "GITHUB_TOKEN")
	assert.Equal(t, beforeLogs, workerLogSnapshot(t, repo), "missing token must append no op")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "missing token must write no state files")
}

func TestArmRefReadNoCurrentHostRef_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	repo := setupRepoWithTask(t)
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	beforeState := armatureStateTreeSnapshot(t, repo)
	_, stderr, err := runTrlsWithStderr(t, repo, "ref-read", "task-01")
	require.Error(t, err)
	combined := stderr + "\n" + err.Error()
	assert.Contains(t, strings.ToLower(combined), "host")
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "no current host ref must write no state files")
}

func TestArmRefReadHyphenAllowlist_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	root := testRootCmd(t)
	cmd, _, err := root.Find([]string{"ref-read"})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	assert.Equal(t, "ref-read", cmd.Name())
}

func TestArmRefReadJSONFailures_REQ_HOSTREF_S1_T6(t *testing.T) {
	t.Parallel()

	t.Run("missing token", func(t *testing.T) {
		t.Parallel()
		repo := setupRepoWithTask(t)
		plantHostRef(t, repo, hostRefReadURLA, "engineering")
		setCLIEnv(t, "GITHUB_TOKEN", "")
		beforeState := armatureStateTreeSnapshot(t, repo)

		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "task-01", "--format", "json")
		assert.Equal(t, 1, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, "REF-READ-1", cf.Code)
		assert.Equal(t, 1, cf.ExitCode)
		assert.Contains(t, cf.Cause, "GITHUB_TOKEN")
		require.NotEmpty(t, cf.NextActions)
		assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo))
	})

	t.Run("unknown url", func(t *testing.T) {
		t.Parallel()
		repo := setupRepoWithTask(t)
		plantHostRef(t, repo, hostRefReadURLA, "engineering")
		setCLIEnv(t, "GITHUB_TOKEN", "test-token")
		beforeState := armatureStateTreeSnapshot(t, repo)

		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "task-01", "--url", hostRefReadURLB, "--format", "json")
		assert.Equal(t, 1, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, "REF-READ-1", cf.Code)
		assert.Equal(t, 1, cf.ExitCode)
		assert.Contains(t, cf.Cause, hostRefReadURLB)
		require.NotEmpty(t, cf.NextActions)
		assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo))
	})

	t.Run("no current host ref", func(t *testing.T) {
		t.Parallel()
		repo := setupRepoWithTask(t)
		setCLIEnv(t, "GITHUB_TOKEN", "test-token")
		beforeState := armatureStateTreeSnapshot(t, repo)

		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "task-01", "--format", "agent")
		assert.Equal(t, 1, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, "REF-READ-1", cf.Code)
		assert.Equal(t, 1, cf.ExitCode)
		assert.Contains(t, strings.ToLower(cf.Cause), "host")
		require.NotEmpty(t, cf.NextActions)
		assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo))
	})

	t.Run("unknown issue", func(t *testing.T) {
		t.Parallel()
		repo := setupRepoWithTask(t)
		plantHostRef(t, repo, hostRefReadURLA, "engineering")
		setCLIEnv(t, "GITHUB_TOKEN", "test-token")
		beforeState := armatureStateTreeSnapshot(t, repo)

		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "missing-issue", "--format", "json")
		assert.Equal(t, 1, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, "REF-READ-1", cf.Code)
		assert.Equal(t, 1, cf.ExitCode)
		assert.Contains(t, cf.Cause, "missing-issue")
		require.NotEmpty(t, cf.NextActions)
		assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "unknown issue must leave on-disk state unchanged")
	})

	t.Run("fetch failure", func(t *testing.T) {
		repo := setupRepoWithTask(t)
		plantHostRef(t, repo, hostRefReadURLA, "engineering")
		withGitHubIssueTransport(t, http.StatusInternalServerError, map[string]any{"message": "boom"})
		setCLIEnv(t, "GITHUB_TOKEN", "test-token")
		beforeState := armatureStateTreeSnapshot(t, repo)

		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "task-01", "--format", "json")
		assert.Equal(t, 1, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, "REF-READ-1", cf.Code)
		assert.Equal(t, 1, cf.ExitCode)
		assert.Contains(t, cf.Cause, "HTTP")
		assert.NotContains(t, cf.Cause, "test-token")
		require.NotEmpty(t, cf.NextActions)
		assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "fetch failure must leave on-disk state unchanged")
	})

	t.Run("usage wrong arity", func(t *testing.T) {
		t.Parallel()
		repo := setupRepoWithTask(t)
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ref-read", "--repo", repo, "--format", "json")
		assert.Equal(t, 2, code)
		cf := assertAgentFailureEnvelope(t, stdout.String())
		assert.Equal(t, armerrors.CodeUSAGE, cf.Code)
		assert.Equal(t, 2, cf.ExitCode)
	})
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

	beforeState := armatureStateTreeSnapshot(t, repo)
	_, _, err = runTrlsWithStderr(t, repo, "ref-read", "task-01", "--url", hostRefReadURLA)
	require.Error(t, err)
	assert.Equal(t, beforeState, armatureStateTreeSnapshot(t, repo), "withdrawn URL reject must write no state files")
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

func armatureStateTreeSnapshot(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(repo, ".armature", "state")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "ABSENT"
		}
		require.NoError(t, err)
	}
	require.True(t, info.IsDir())

	var lines []string
	fsys := os.DirFS(root)
	err = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			lines = append(lines, "D|"+filepath.ToSlash(rel))
			return nil
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, "F|"+filepath.ToSlash(rel)+"|"+hex.EncodeToString(sum[:]))
		return nil
	})
	require.NoError(t, err)
	return strings.Join(lines, "\n")
}
