package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	probeHookVar   = "requireOwnerHook"
	probeHookPkg   = "hookprobe"
	probeHookSrc   = "package " + probeHookPkg + "\n\nvar " + probeHookVar + " func(string) error\n"
	probeGoMod     = "module " + probeHookPkg + "\n\ngo 1.22\n"
	gocheckLinter  = "gochecknoglobals"
	golangciBinary = "golangci-lint"
)

// checkGochecknoglobalsProbe proves .golangci.yml enables gochecknoglobals and
// that a reintroduced package-level mutable func hook fails that linter. The
// probe module is written outside the source tree so make lint / go test never
// shell golangci-lint from a *_test.go file.
func checkGochecknoglobalsProbe(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	cfg, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: read .golangci.yml: %v\n", err)
		return err
	}
	if !strings.Contains(string(cfg), "- "+gocheckLinter) {
		fmt.Fprintf(stderr, "FAIL: .golangci.yml must enable %s\n", gocheckLinter)
		return fmt.Errorf("%s not enabled", gocheckLinter)
	}

	if _, err := exec.LookPath(golangciBinary); err != nil {
		fmt.Fprintf(stderr, "FAIL: %s not found on PATH (install via make install-check-tools)\n", golangciBinary)
		return err
	}

	dir, err := os.MkdirTemp("", "armature-gochecknoglobals-probe-*")
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: mkdir probe: %v\n", err)
		return err
	}
	defer func() {
		if remErr := os.RemoveAll(dir); remErr != nil {
			fmt.Fprintf(stderr, "WARN: cleanup probe dir: %v\n", remErr)
		}
	}()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(probeGoMod), 0o600); err != nil {
		fmt.Fprintf(stderr, "FAIL: write probe go.mod: %v\n", err)
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "hook.go"), []byte(probeHookSrc), 0o600); err != nil {
		fmt.Fprintf(stderr, "FAIL: write probe hook.go: %v\n", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, golangciBinary, "run",
		"--enable-only="+gocheckLinter,
		"--max-issues-per-linter=0",
		"--max-same-issues=0",
		"./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME=/tmp/golangci-lint-cache")
	out, err := cmd.CombinedOutput()
	joined := string(out)
	if err == nil {
		fmt.Fprintf(stderr, "FAIL: mutable %s must fail %s; out=\n%s\n", probeHookVar, gocheckLinter, joined)
		return fmt.Errorf("%s probe did not fail", gocheckLinter)
	}
	if !strings.Contains(joined, gocheckLinter) || !strings.Contains(joined, probeHookVar) {
		fmt.Fprintf(stderr, "FAIL: expected %s hit for %s; out=\n%s\n", gocheckLinter, probeHookVar, joined)
		return fmt.Errorf("%s probe missing findings", gocheckLinter)
	}
	fmt.Fprintf(stdout, "OK: %s rejects reintroduced mutable %s\n", gocheckLinter, probeHookVar)
	return nil
}
