package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type armHarness struct {
	t                 *testing.T
	repo              string
	env               map[string]string
	temp              string
	stdout            bytes.Buffer
	stderr            bytes.Buffer
	openTTY           func() (*os.File, error)
	gitNetworkTimeout time.Duration
}

func newArm(t *testing.T, repo string, env map[string]string) *armHarness {
	t.Helper()
	return &armHarness{t: t, repo: repo, env: env, temp: testTempDir(t)}
}

func (h *armHarness) WithOpenTTY(open func() (*os.File, error)) *armHarness {
	h.openTTY = open
	return h
}

func (h *armHarness) WithGitNetworkTimeout(d time.Duration) *armHarness {
	h.gitNetworkTimeout = d
	return h
}

func (h *armHarness) MustRun(args ...string) string {
	h.t.Helper()
	out, err := h.Run(args...)
	if err != nil {
		h.t.Fatalf("arm %v: %v\nstderr: %s", args, err, h.stderr.String())
	}
	return out
}

func (h *armHarness) Run(args ...string) (string, error) {
	h.t.Helper()
	h.stdout.Reset()
	h.stderr.Reset()
	root := commandFor(Invocation{
		Env:                OverlayEnv(OSEnv(), mergeCLIEnv(h.t, h.env)),
		Stdout:             &h.stdout,
		Stderr:             &h.stderr,
		TempDir:            h.temp,
		OpenControllingTTY: h.openTTY,
		GitNetworkTimeout:  h.gitNetworkTimeout,
	})
	root.SetArgs(append(injectDoneDelivery(h.repo, enrichTestCLIArgs(args)), "--repo", h.repo))
	err := root.Execute()
	return h.stdout.String(), err
}

// testSlots holds per-*testing.T CLI env overlays and shared temp dirs so
// parallel tests stay isolated without os.Setenv. Tests never reassign this
// pointer; they only mutate maps through helpers.
type testSlots struct {
	mu    sync.Mutex
	env   map[*testing.T]map[string]string
	temps map[*testing.T]string
}

var slots = &testSlots{ //nolint:gochecknoglobals // per-*testing.T CLI isolation; tests never reassign the pointer
	env:   map[*testing.T]map[string]string{},
	temps: map[*testing.T]string{},
}

func testTempDir(t *testing.T) string {
	t.Helper()
	slots.mu.Lock()
	defer slots.mu.Unlock()
	if d, ok := slots.temps[t]; ok {
		return d
	}
	d := t.TempDir()
	slots.temps[t] = d
	t.Cleanup(func() {
		slots.mu.Lock()
		delete(slots.temps, t)
		slots.mu.Unlock()
	})
	return d
}

func setCLIEnv(t *testing.T, key, value string) {
	t.Helper()
	slots.mu.Lock()
	defer slots.mu.Unlock()
	m := slots.env[t]
	if m == nil {
		m = map[string]string{}
		slots.env[t] = m
		t.Cleanup(func() {
			slots.mu.Lock()
			delete(slots.env, t)
			slots.mu.Unlock()
		})
	}
	m[key] = value
}

func unsetCLIEnv(t *testing.T, key string) {
	t.Helper()
	slots.mu.Lock()
	defer slots.mu.Unlock()
	if m := slots.env[t]; m != nil {
		delete(m, key)
	}
}

func prependPATH(t *testing.T, wrapperDir string) {
	t.Helper()
	setCLIEnv(t, "PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func cliEnvFor(t *testing.T) map[string]string {
	t.Helper()
	slots.mu.Lock()
	defer slots.mu.Unlock()
	src := slots.env[t]
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func testEnv(t *testing.T) Env {
	t.Helper()
	return OverlayEnv(OSEnv(), cliEnvFor(t))
}

func mergeCLIEnv(t *testing.T, extra map[string]string) map[string]string {
	t.Helper()
	out := cliEnvFor(t)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func testRoot(t *testing.T, extra map[string]string, stdout, stderr io.Writer) *cobra.Command {
	t.Helper()
	return testRootInv(t, Invocation{
		Env:     OverlayEnv(OSEnv(), mergeCLIEnv(t, extra)),
		Stdout:  stdout,
		Stderr:  stderr,
		TempDir: testTempDir(t),
	})
}

func testRootInv(t *testing.T, inv Invocation) *cobra.Command {
	t.Helper()
	if inv.Stdout == nil {
		inv.Stdout = new(bytes.Buffer)
	}
	if inv.Stderr == nil {
		inv.Stderr = new(bytes.Buffer)
	}
	if inv.Env == nil {
		inv.Env = OverlayEnv(OSEnv(), cliEnvFor(t))
	}
	if inv.TempDir == "" {
		inv.TempDir = testTempDir(t)
	}
	return commandFor(inv)
}

func testRootCmd(t *testing.T) *cobra.Command {
	t.Helper()
	return testRoot(t, nil, new(bytes.Buffer), new(bytes.Buffer))
}

func testRootCmdWith(t *testing.T, inv Invocation) *cobra.Command {
	t.Helper()
	return testRootInv(t, inv)
}
