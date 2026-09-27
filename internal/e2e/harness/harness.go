// Package harness is the test-only lifecycle harness for arm CLI end-to-end
// tests (make test-e2eharness).
package harness

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
)

type Harness struct {
	t          *testing.T
	TempDir    string
	OriginDir  string
	WorkDir    string
	WorkerDirs map[string]string
	ArmBinPath string
}

func New(t *testing.T, armBinPath string) *Harness {
	t.Helper()

	tempDir := t.TempDir()
	fx := gittest.InitWithOrigin(t)
	gittest.Git(t, fx.Dir, "commit", "--allow-empty", "-m", "init")
	if err := gitRun(t, fx.Dir, "push", "-u", "origin", "main"); err != nil {
		t.Fatalf("failed to push to origin: %v", err)
	}

	h := &Harness{
		t:          t,
		TempDir:    tempDir,
		OriginDir:  fx.Origin,
		WorkDir:    filepath.Join(tempDir, "work"),
		WorkerDirs: make(map[string]string),
		ArmBinPath: armBinPath,
	}

	if err := h.Clone("work", h.WorkDir); err != nil {
		t.Fatalf("failed to clone to work directory: %v", err)
	}

	return h
}

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

func (h *Harness) RunArm(args ...string) (string, error) {
	return runCmd(h.t, h.WorkDir, h.ArmBinPath, args...)
}

func (h *Harness) RunArmIn(path string, args ...string) (string, error) {
	return runCmd(h.t, path, h.ArmBinPath, args...)
}

func (h *Harness) GetWorkerDir(name string) string {
	if path, ok := h.WorkerDirs[name]; ok {
		return path
	}
	return ""
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
	cmd.Env = stripInheritedARMLogSlot()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func stripInheritedARMLogSlot() []string {
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
