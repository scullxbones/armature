package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/gittest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOpsBackupRepo(t *testing.T) string {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := writeArmatureLayout(repo, "ops-backup-worker")
	require.NoError(t, err)
	gittest.Git(t, repo, "checkout", "--orphan", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	gittest.Git(t, repo, "checkout", "main")
	return repo
}

func setupOpsBackupFreshClone(t *testing.T) string {
	t.Helper()
	fx := gittest.InitWithOrigin(t)
	repo := fx.Dir
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	gittest.Git(t, repo, "checkout", "--orphan", "_armature")
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "ops tip")
	gittest.Git(t, repo, "push", "-u", "origin", "_armature")
	gittest.Git(t, repo, "checkout", "main")
	return repo
}

func TestOpsBackupSnapshotVerifyRestorePlan_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupRepo(t)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	var snapEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &snapEnv))
	require.Contains(t, snapEnv, "count")
	require.Contains(t, snapEnv, "snapshots")
	require.Contains(t, snapEnv, "help")
	require.FileExists(t, bundle)

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "verify", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	var verifyEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &verifyEnv))
	require.Contains(t, verifyEnv, "verifications")
	require.Contains(t, verifyEnv, "help")

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "restore-plan", "--from", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute())
	var planEnv map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &planEnv))
	require.Contains(t, planEnv, "plans")
	require.Contains(t, planEnv, "help")
	assert.Contains(t, out.String(), "fast-forward")
}

func TestOpsBackupVerifyRestorePlan_FreshCloneNoLayout_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupFreshClone(t)
	bundle := filepath.Join(t.TempDir(), "armature-ops.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute(), "snapshot must work without armature layout")
	require.FileExists(t, bundle)

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "verify", bundle, "--repo", repo, "--format", "human", "--non-interactive"})
	require.NoError(t, root.Execute(), "verify must work on a fresh clone")
	assert.Contains(t, out.String(), "verify ok")

	root = testRootCmd(t)
	out.Reset()
	root.SetOut(out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"ops-backup", "restore-plan", "--from", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	require.NoError(t, root.Execute(), "restore-plan must work on a fresh clone")
	assert.Contains(t, out.String(), `"plans"`)
}

func TestOpsBackupSnapshot_RefreshFail_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := gittest.InitRepo(t)
	gittest.Git(t, repo, "commit", "--allow-empty", "-m", "init")
	_, err := writeArmatureLayout(repo, "ops-backup-worker")
	require.NoError(t, err)
	bundle := filepath.Join(t.TempDir(), "missing.bundle")

	root := testRootCmd(t)
	out := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errBuf)
	root.SetArgs([]string{"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive"})
	err = root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPS-BACKUP-1")
	assert.NoFileExists(t, bundle)
}

func TestOpsBackupVerify_BadBundle_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupRepo(t)
	bad := filepath.Join(t.TempDir(), "not-a-bundle")
	require.NoError(t, os.WriteFile(bad, []byte("nope"), 0o644))

	root := testRootCmd(t)
	root.SetArgs([]string{"ops-backup", "verify", bad, "--repo", repo, "--non-interactive"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPS-BACKUP-1")
}

func assertExactlyOneJSONValue(t *testing.T, stdout string) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(stdout)))
	var first any
	require.NoError(t, dec.Decode(&first), "stdout must decode as JSON, got %q", stdout)
	require.False(t, dec.More(), "stdout must be exactly one JSON value")
	var extra any
	err := dec.Decode(&extra)
	require.ErrorIs(t, err, io.EOF, "expected EOF after one JSON value, got extra %v err=%v", extra, err)
}

func TestOpsBackup_StructuredStdoutExactlyOneJSON_REQ_TOPTIER_S12_T1(t *testing.T) {
	t.Parallel()
	repo := setupOpsBackupRepo(t)
	dir := t.TempDir()
	okBundle := filepath.Join(dir, "ok.bundle")
	divergeBundle := filepath.Join(dir, "diverge.bundle")

	// Seed an ok tip bundle, then rewrite origin to an unrelated tip so
	// restore-plan --from divergeBundle yields diverge→_armature-recovered.
	{
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ops-backup", "snapshot", "--output", divergeBundle, "--repo", repo, "--format", "json", "--non-interactive")
		require.Equal(t, 0, code, stdout.String())
		gittest.Git(t, repo, "checkout", "--orphan", "unrelated-ops")
		gittest.Git(t, repo, "commit", "--allow-empty", "-m", "unrelated tip")
		gittest.Git(t, repo, "push", "--force", "origin", "HEAD:refs/heads/_armature")
		gittest.Git(t, repo, "checkout", "main")
	}

	type want struct {
		exit       int
		payloadKey string // empty → expect error envelope
		substr     string
	}
	cases := []struct {
		name string
		args []string
		want want
	}{
		{
			name: "snapshot_ok_json",
			args: []string{"ops-backup", "snapshot", "--output", okBundle, "--repo", repo, "--format", "json", "--non-interactive"},
			want: want{exit: 0, payloadKey: "snapshots", substr: `"status"`},
		},
		{
			name: "snapshot_ok_agent",
			args: []string{"ops-backup", "snapshot", "--output", filepath.Join(dir, "ok-agent.bundle"), "--repo", repo, "--format", "agent", "--non-interactive"},
			want: want{exit: 0, payloadKey: "snapshots", substr: `"status"`},
		},
		{
			name: "snapshot_refresh_fail",
			args: func() []string {
				bare := gittest.InitRepo(t)
				gittest.Git(t, bare, "commit", "--allow-empty", "-m", "init")
				return []string{"ops-backup", "snapshot", "--output", filepath.Join(dir, "fail.bundle"), "--repo", bare, "--format", "json", "--non-interactive"}
			}(),
			want: want{exit: 1, substr: `"OPS-BACKUP-1"`},
		},
		{
			name: "verify_ok_json",
			args: []string{"ops-backup", "verify", okBundle, "--repo", repo, "--format", "json", "--non-interactive"},
			want: want{exit: 0, payloadKey: "verifications", substr: `"ok"`},
		},
		{
			name: "verify_ok_agent",
			args: []string{"ops-backup", "verify", okBundle, "--repo", repo, "--format", "agent", "--non-interactive"},
			want: want{exit: 0, payloadKey: "verifications", substr: `"ok"`},
		},
		{
			name: "verify_bad_bundle",
			args: func() []string {
				bad := filepath.Join(dir, "not-a-bundle")
				require.NoError(t, os.WriteFile(bad, []byte("nope"), 0o644))
				return []string{"ops-backup", "verify", bad, "--repo", repo, "--format", "json", "--non-interactive"}
			}(),
			want: want{exit: 1, substr: `"OPS-BACKUP-1"`},
		},
		{
			name: "restore_plan_ff_json",
			args: []string{"ops-backup", "restore-plan", "--from", okBundle, "--repo", repo, "--format", "json", "--non-interactive"},
			want: want{exit: 0, payloadKey: "plans", substr: "fast-forward"},
		},
		{
			name: "restore_plan_diverge_json",
			args: []string{"ops-backup", "restore-plan", "--from", divergeBundle, "--repo", repo, "--format", "json", "--non-interactive"},
			want: want{exit: 1, payloadKey: "plans", substr: "diverge→_armature-recovered"},
		},
		{
			name: "restore_plan_diverge_agent",
			args: []string{"ops-backup", "restore-plan", "--from", divergeBundle, "--repo", repo, "--format", "agent", "--non-interactive"},
			want: want{exit: 1, payloadKey: "plans", substr: "diverge→_armature-recovered"},
		},
		{
			name: "restore_plan_bad_bundle",
			args: func() []string {
				bad := filepath.Join(dir, "plan-bad.bundle")
				require.NoError(t, os.WriteFile(bad, []byte("nope"), 0o644))
				return []string{"ops-backup", "restore-plan", "--from", bad, "--repo", repo, "--format", "json", "--non-interactive"}
			}(),
			want: want{exit: 1, substr: `"OPS-BACKUP-1"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			code := executeThenHandleRootError(t, stdout, stderr, tc.args...)
			assert.Equal(t, tc.want.exit, code, "stderr=%s stdout=%s", stderr.String(), stdout.String())
			assertExactlyOneJSONValue(t, stdout.String())
			assert.Contains(t, stdout.String(), tc.want.substr)
			if tc.want.payloadKey != "" {
				var env map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &env))
				require.Contains(t, env, tc.want.payloadKey)
				require.Contains(t, env, "help")
				require.NotContains(t, env, "error", "protocol report must not be a Command Failure envelope")
				if tc.want.exit != 0 {
					assert.Contains(t, string(env["help"]), "_armature-recovered")
				}
			} else {
				var env map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &env))
				require.Contains(t, env, "error")
			}
		})
	}
}

// TCP accept-never-reply stalls git-remote-http. networkCmd must kill the
// process group so snapshot / restore-plan return a fail-closed error envelope
// within timeout+WaitDelay slack (not hang the CLI).
func TestOpsBackup_StalledHTTPRemote_FailClosedEnvelope_REQ_TOPTIER_S12_T1G(t *testing.T) {
	t.Parallel()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close() //nolint:errcheck
	go func() {
		for {
			conn, accErr := ln.Accept()
			if accErr != nil {
				return
			}
			// Accept then never reply — holds git-remote-http until group kill.
			go func(c net.Conn) {
				defer c.Close() //nolint:errcheck
				select {}
			}(conn)
		}
	}()
	stalledURL := "http://" + ln.Addr().String() + "/armature-ops-backup.git"

	repo := setupOpsBackupFreshClone(t)
	bundle := filepath.Join(t.TempDir(), "pre.bundle")
	{
		stdout := new(bytes.Buffer)
		code := executeThenHandleRootError(t, stdout, new(bytes.Buffer),
			"ops-backup", "snapshot", "--output", bundle, "--repo", repo, "--format", "json", "--non-interactive")
		require.Equal(t, 0, code, stdout.String())
	}
	gittest.Git(t, repo, "remote", "set-url", "origin", stalledURL)

	slack := 3 * time.Second
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "snapshot",
			args: []string{"ops-backup", "snapshot", "--output", filepath.Join(t.TempDir(), "stall.bundle"),
				"--repo", repo, "--format", "json", "--non-interactive"},
		},
		{
			name: "restore-plan",
			args: []string{"ops-backup", "restore-plan", "--from", bundle,
				"--repo", repo, "--format", "json", "--non-interactive"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			start := time.Now()
			code := Run(t.Context(), Invocation{
				Args:              tc.args,
				Env:               OverlayEnv(OSEnv(), cliEnvFor(t)),
				Stdout:            stdout,
				Stderr:            stderr,
				TempDir:           testTempDir(t),
				GitNetworkTimeout: 200 * time.Millisecond,
			})
			elapsed := time.Since(start)
			assert.Equal(t, 1, code, "stderr=%s stdout=%s", stderr.String(), stdout.String())
			assert.Less(t, elapsed, slack, "%s must fail-closed near network timeout, got %v", tc.name, elapsed)
			assertExactlyOneJSONValue(t, stdout.String())
			var env map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &env))
			require.Contains(t, env, "error", "fail-closed must be an error envelope")
			assert.Contains(t, stdout.String(), "OPS-BACKUP-1")
		})
	}
}
