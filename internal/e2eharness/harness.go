// Package e2eharness provides infrastructure for end-to-end lifecycle tests of the arm CLI.
//
// It creates a bare origin repository, clones, and orchestrates the complete workflow:
// bootstrap → worker-init → create → claim → in-progress → done → merge detection.
package e2eharness

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Harness manages a test repository lifecycle with a bare origin and clones.
type Harness struct {
	t          *testing.T
	TempDir    string            // Root temporary directory for all test artifacts
	OriginDir  string            // Bare git repository (origin)
	WorkDir    string            // Primary working clone (coordinator perspective)
	WorkerDirs map[string]string // Named worker directories (indexed by worker ID)
	ArmBinPath string            // Path to the built arm binary
}

// New creates a new harness with a temporary directory and bare origin repo.
func New(t *testing.T, armBinPath string) *Harness {
	t.Helper()

	tempDir := t.TempDir()
	originDir := filepath.Join(tempDir, "origin.git")

	// Initialize bare origin repository
	if err := gitInit(t, originDir, true); err != nil {
		t.Fatalf("failed to initialize bare origin repo: %v", err)
	}

	// Create initial commit in temporary clone for bootstrap
	initClone := filepath.Join(tempDir, ".init")
	if err := gitInit(t, initClone, false); err != nil {
		t.Fatalf("failed to initialize temporary clone: %v", err)
	}

	configGit(t, initClone)
	if err := gitRun(t, initClone, "commit", "--allow-empty", "-m", "init"); err != nil {
		t.Fatalf("failed to create init commit: %v", err)
	}
	if err := gitRun(t, initClone, "remote", "add", "origin", originDir); err != nil {
		t.Fatalf("failed to add origin remote: %v", err)
	}
	if err := gitRun(t, initClone, "branch", "-M", "main"); err != nil {
		t.Fatalf("failed to rename branch to main: %v", err)
	}
	if err := gitRun(t, initClone, "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("failed to push to origin: %v", err)
	}
	if err := gitRun(t, originDir, "symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
		t.Fatalf("failed to set origin HEAD: %v", err)
	}

	h := &Harness{
		t:          t,
		TempDir:    tempDir,
		OriginDir:  originDir,
		WorkDir:    filepath.Join(tempDir, "work"),
		WorkerDirs: make(map[string]string),
		ArmBinPath: armBinPath,
	}

	// Clone to work directory for coordinator
	if err := h.Clone("work", h.WorkDir); err != nil {
		t.Fatalf("failed to clone to work directory: %v", err)
	}

	return h
}

// Clone creates a new clone of the origin repository at the specified path.
func (h *Harness) Clone(name, path string) error {
	if err := gitRun(h.t, h.TempDir, "clone", h.OriginDir, path); err != nil {
		h.t.Fatalf("failed to clone: %v", err)
	}
	configGit(h.t, path)

	if name != "work" {
		h.WorkerDirs[name] = path
	}

	return nil
}

// RunArm executes an arm command in the work directory with the harness arm binary.
func (h *Harness) RunArm(args ...string) (string, error) {
	return runCmd(h.t, h.WorkDir, h.ArmBinPath, args...)
}

// RunArmIn executes an arm command in a specified clone directory.
func (h *Harness) RunArmIn(path string, args ...string) (string, error) {
	return runCmd(h.t, path, h.ArmBinPath, args...)
}

// GetWorkerDir returns the path to a named worker's clone directory.
func (h *Harness) GetWorkerDir(name string) string {
	if path, ok := h.WorkerDirs[name]; ok {
		return path
	}
	return ""
}

// helper functions

func gitInit(t *testing.T, dir string, bare bool) error {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	args := []string{"init"}
	if bare {
		args = append(args, "--bare")
	}

	return gitRun(t, dir, args...)
}

func configGit(t *testing.T, dir string) {
	t.Helper()
	if err := gitRun(t, dir, "config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("failed to configure git email: %v", err)
	}
	if err := gitRun(t, dir, "config", "user.name", "Test"); err != nil {
		t.Fatalf("failed to configure git name: %v", err)
	}
	if err := gitRun(t, dir, "config", "commit.gpgsign", "false"); err != nil {
		t.Fatalf("failed to configure gpgsign: %v", err)
	}
	if err := gitRun(t, dir, "config", "gc.auto", "0"); err != nil {
		t.Fatalf("failed to configure gc.auto: %v", err)
	}
	if err := gitRun(t, dir, "config", "maintenance.auto", "false"); err != nil {
		t.Fatalf("failed to configure maintenance.auto: %v", err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) error {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: git is a fixed constant
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v failed: %s", args, out)
		return err
	}
	return nil
}

func runCmd(t *testing.T, dir, cmdName string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), cmdName, args...) //nolint:gosec // G204: cmdName is from harness configuration
	cmd.Dir = dir
	cmd.Env = envWithoutARMLogSlot()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

// envWithoutARMLogSlot copies the process environment minus ARM_LOG_SLOT.
// Product bug, not a harness design: dag apply (internal/decompose/apply.go)
// writes workerID+".log" and ignores ARM_LOG_SLOT. An inherited slot would
// send later commands to a different file and break clone rematerialization.
// Follow-up (not T4): honor the slot on dag apply and add a slotted E2E test.
// This strip is a workaround so T4's e2e can run; it is not the fix.
func envWithoutARMLogSlot() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "ARM_LOG_SLOT=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
