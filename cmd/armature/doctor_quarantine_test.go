package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorCorruptLogSuggestsQuarantine_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	logPath := firstOpsLog(t, repo)
	base := filepath.Base(logPath)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	line := strings.Count(string(raw), "\n")
	repair := "arm doctor quarantine --log " + base + " --line " + strconv.Itoa(line)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, stderr, "doctor", "--format", "agent", "--repo", repo)
	assert.NotEqual(t, 0, code)
	joined := stdout.String() + stderr.String()
	assert.Contains(t, joined, base)
	assert.Contains(t, joined, ":"+strconv.Itoa(line))
	assert.Contains(t, joined, repair)

	envelope := assertSingleJSONObject(t, stdout.String())
	errObj, _ := envelope["error"].(map[string]any)
	require.NotNil(t, errObj)
	assert.Equal(t, "DOCTOR-1", errObj["code"])
	actions, _ := errObj["next_actions"].([]any)
	require.Len(t, actions, 1)
	assert.Equal(t, repair, actions[0])
}

func TestDoctorQuarantineThenLoadSucceeds_REQ_AGTFND(t *testing.T) {
	t.Parallel()
	repo := setupRepoWithTask(t)
	logPath := firstOpsLog(t, repo)
	base := filepath.Base(logPath)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	line := strings.Count(string(raw), "\n")

	out, errOut, err := runTrlsWithStderr(t, repo, "doctor", "quarantine", "--log", base, "--line", strconv.Itoa(line))
	require.NoError(t, err, "stdout=%s stderr=%s", out, errOut)

	sidecar, err := os.ReadFile(logPath + ".quarantine")
	require.NoError(t, err)
	assert.Contains(t, string(sidecar), "not json")
	after, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(after), string(raw)))
	assert.Contains(t, string(after), "not json")

	_, _, err = runTrlsWithStderr(t, repo, "doctor", "quarantine", "--log", base, "--line", strconv.Itoa(line))
	require.NoError(t, err, "second quarantine must be idempotent")

	stdout := new(bytes.Buffer)
	code := executeThenHandleRootError(t, stdout, new(bytes.Buffer), "doctor", "--format", "agent", "--repo", repo)
	assert.Equal(t, 0, code, stdout.String())
	assert.NotContains(t, stdout.String(), `"error"`)
}

func firstOpsLog(t *testing.T, repo string) string {
	t.Helper()
	opsDir := filepath.Join(repo, ".armature", "ops")
	entries, err := os.ReadDir(opsDir)
	require.NoError(t, err)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			return filepath.Join(opsDir, e.Name())
		}
	}
	t.Fatal("no ops log in fixture")
	return ""
}
