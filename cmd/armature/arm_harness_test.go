package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

type armHarness struct {
	t      *testing.T
	repo   string
	env    map[string]string
	temp   string
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newArm(t *testing.T, repo string, env map[string]string) *armHarness {
	t.Helper()
	return &armHarness{t: t, repo: repo, env: env, temp: testTempDir(t)}
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
	root := testRoot(h.t, h.env, &h.stdout, &h.stderr)
	root.SetArgs(append(injectDoneDelivery(h.repo, enrichTestCLIArgs(args)), "--repo", h.repo))
	err := root.Execute()
	return h.stdout.String(), err
}

var cliEnvMu sync.Mutex
var cliEnv = map[*testing.T]map[string]string{}
var testTemps = map[*testing.T]string{}

func testTempDir(t *testing.T) string {
	t.Helper()
	cliEnvMu.Lock()
	defer cliEnvMu.Unlock()
	if d, ok := testTemps[t]; ok {
		return d
	}
	d := t.TempDir()
	testTemps[t] = d
	t.Cleanup(func() {
		cliEnvMu.Lock()
		delete(testTemps, t)
		cliEnvMu.Unlock()
	})
	return d
}

func setCLIEnv(t *testing.T, key, value string) {
	t.Helper()
	cliEnvMu.Lock()
	defer cliEnvMu.Unlock()
	m := cliEnv[t]
	if m == nil {
		m = map[string]string{}
		cliEnv[t] = m
		t.Cleanup(func() {
			cliEnvMu.Lock()
			delete(cliEnv, t)
			cliEnvMu.Unlock()
		})
	}
	m[key] = value
}

func unsetCLIEnv(t *testing.T, key string) {
	t.Helper()
	cliEnvMu.Lock()
	defer cliEnvMu.Unlock()
	if m := cliEnv[t]; m != nil {
		delete(m, key)
	}
}

func prependPATH(t *testing.T, wrapperDir string) {
	t.Helper()
	setCLIEnv(t, "PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func cliEnvFor(t *testing.T) map[string]string {
	t.Helper()
	cliEnvMu.Lock()
	defer cliEnvMu.Unlock()
	src := cliEnv[t]
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
	if stdout == nil {
		stdout = new(bytes.Buffer)
	}
	if stderr == nil {
		stderr = new(bytes.Buffer)
	}
	return commandFor(Invocation{
		Env:     OverlayEnv(OSEnv(), mergeCLIEnv(t, extra)),
		Stdout:  stdout,
		Stderr:  stderr,
		TempDir: testTempDir(t),
	})
}

func testRootCmd(t *testing.T) *cobra.Command {
	t.Helper()
	return testRoot(t, nil, new(bytes.Buffer), new(bytes.Buffer))
}
