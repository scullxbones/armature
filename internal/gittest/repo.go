package gittest

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

type Fixture struct {
	Dir    string
	Origin string
}

func Init(t *testing.T) Fixture {
	t.Helper()
	if err := IsolateGit(); err != nil {
		t.Fatalf("IsolateGit: %v", err)
	}
	dir := t.TempDir()
	if err := adapters.GitInitMain(dir); err != nil {
		t.Fatalf("GitInitMain: %v", err)
	}
	configureRepo(t, dir)

	originParent := t.TempDir()
	origin := filepath.Join(originParent, "origin.git")
	if err := adapters.GitInitBareMain(origin); err != nil {
		t.Fatalf("GitInitBareMain: %v", err)
	}
	configureRepo(t, origin)
	Git(t, dir, "remote", "add", "origin", origin)

	t.Cleanup(func() {
		waitGitIdle(dir)
		waitGitIdle(origin)
	})
	return Fixture{Dir: dir, Origin: origin}
}

func InitRepo(t *testing.T) string {
	t.Helper()
	return Init(t).Dir
}

func Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := gitCombined(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func gitCombined(dir string, args ...string) (string, error) {
	cmd := adapters.NonInteractiveGitCommand(dir, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func configureRepo(t *testing.T, dir string) {
	t.Helper()
	Git(t, dir, "config", "user.email", "test@test.com")
	Git(t, dir, "config", "user.name", "Test")
	Git(t, dir, "config", "commit.gpgsign", "false")
	Git(t, dir, "config", "tag.gpgsign", "false")
	Git(t, dir, "config", "gc.auto", "0")
	Git(t, dir, "config", "gc.autoDetach", "false")
	Git(t, dir, "config", "maintenance.auto", "false")
	Git(t, dir, "config", "receive.autogc", "false")
	Git(t, dir, "config", "credential.helper", "")
}

func waitGitIdle(dir string) {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "gc", "--auto")
	cmd.Env = os.Environ()
	_ = cmd.Run()
}
