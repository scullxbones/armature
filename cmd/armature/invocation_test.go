package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_VersionHuman_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := Run(context.Background(), Invocation{
		Args:    []string{"version", "--format", "human"},
		Stdout:  stdout,
		Stderr:  stderr,
		TempDir: t.TempDir(),
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "arm version")
	assert.NotContains(t, stdout.String(), `"versions"`)
}

func TestRun_NilContextKeepsSession_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	var ctx context.Context
	code := Run(ctx, Invocation{
		Args:    []string{"version", "--format", "human"},
		Stdout:  stdout,
		Stderr:  new(bytes.Buffer),
		TempDir: t.TempDir(),
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "arm version")
}

func TestRun_IsolatesFormatBetweenInvocations_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	var humanOut, jsonOut string
	wg.Add(2)
	go func() {
		defer wg.Done()
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), Invocation{
			Args:    []string{"version", "--format", "human"},
			Stdout:  stdout,
			Stderr:  new(bytes.Buffer),
			TempDir: t.TempDir(),
			TTY:     true,
		})
		require.Equal(t, 0, code)
		humanOut = stdout.String()
	}()
	go func() {
		defer wg.Done()
		stdout := new(bytes.Buffer)
		code := Run(context.Background(), Invocation{
			Args:    []string{"version", "--format", "json"},
			Stdout:  stdout,
			Stderr:  new(bytes.Buffer),
			TempDir: t.TempDir(),
		})
		require.Equal(t, 0, code)
		jsonOut = stdout.String()
	}()
	wg.Wait()
	assert.Contains(t, humanOut, "arm version")
	assert.True(t, strings.Contains(jsonOut, `"versions"`) || strings.Contains(jsonOut, `"version"`), jsonOut)
}

func TestApplyTTYPolicyUsesInvocationEnvNotProcess_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	stdout := new(bytes.Buffer)
	root := commandFor(Invocation{
		Stdout:  stdout,
		Stderr:  new(bytes.Buffer),
		TempDir: t.TempDir(),
		TTY:     true,
		Env: OverlayEnv(OSEnv(), map[string]string{
			"GEMINI_CLI": "1",
			"TERM":       "xterm",
		}),
	})
	format, nonInteractive := applyTTYDetectionPolicy(root)
	assert.Equal(t, "agent", format)
	assert.True(t, nonInteractive)
	assert.False(t, uiFrom(root).IsInteractive())
}

func TestNewRootCmdDoesNotUsePackageTUISetters_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	empty := mapEnv{}
	a := commandFor(Invocation{Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), TTY: true, TempDir: t.TempDir(), Env: empty})
	b := commandFor(Invocation{Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), TTY: false, TempDir: t.TempDir(), Env: empty})
	uiFrom(a).Apply("human", false)
	uiFrom(b).Apply("json", true)
	require.True(t, uiFrom(a).IsInteractive())
	require.False(t, uiFrom(b).IsInteractive())
	require.False(t, uiFrom(a).IsNonInteractive())
	require.True(t, uiFrom(b).IsNonInteractive())
}

func TestOverlayEnvPrefersTop_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	env := OverlayEnv(mapEnv{"ARM_LOG_SLOT": "base", "TERM": "xterm"}, map[string]string{"ARM_LOG_SLOT": "top"})
	v, ok := env.Lookup("ARM_LOG_SLOT")
	require.True(t, ok)
	assert.Equal(t, "top", v)
	term, ok := env.Lookup("TERM")
	require.True(t, ok)
	assert.Equal(t, "xterm", term)
}

type mapEnv map[string]string

func (m mapEnv) Lookup(key string) (string, bool) {
	v, ok := m[key]
	return v, ok
}

func TestSlottedWorkerIDUsesInvocationEnv_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	id := slottedWorkerIDEnv("worker-123", OverlayEnv(nil, map[string]string{"ARM_LOG_SLOT": "lane-a"}))
	assert.Equal(t, "worker-123~lane-a", id)
	plain := slottedWorkerIDEnv("worker-123", OverlayEnv(nil, map[string]string{"ARM_LOG_SLOT": ""}))
	assert.Equal(t, "worker-123", plain)
}

func TestHeartbeatStateIsScopedToTempDir_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	a := t.TempDir()
	b := t.TempDir()
	now := time.Now().Truncate(time.Second)
	require.NoError(t, writeHeartbeatRateLimitStateAt(a, "w", "i", now))
	assert.True(t, readHeartbeatRateLimitStateAt(b, "w", "i").IsZero())
	got := readHeartbeatRateLimitStateAt(a, "w", "i")
	assert.Equal(t, now.Unix(), got.Unix())
}

func TestRepoPathJoinsInvocationDir_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	root := commandFor(Invocation{Dir: "/tmp/arm-inv-dir", Stdout: new(bytes.Buffer), Stderr: new(bytes.Buffer), TempDir: t.TempDir()})
	assert.Equal(t, "/tmp/arm-inv-dir", repoPathFromCmd(root))
	require.NoError(t, root.PersistentFlags().Set("repo", "."))
	assert.Equal(t, "/tmp/arm-inv-dir", repoPathFromCmd(root))
}

func TestRendererLookupMatchesEnvInterface_REQ_PARCLI(t *testing.T) {
	t.Parallel()
	r := tui.NewRenderer(true, OverlayEnv(nil, map[string]string{"TERM": "dumb"}).Lookup)
	r.Apply("human", false)
	assert.False(t, r.IsInteractive())
}
