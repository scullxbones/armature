package adapters

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunProcessWithEnvInjectsEnvironment(t *testing.T) {
	t.Parallel()
	var stdout strings.Builder
	var stderr strings.Builder

	status, err := RunProcessWithEnv(
		context.Background(),
		t.TempDir(),
		[]string{"sh", "-c", "printf %s \"$ARMATURE_TEST_ENV\""},
		map[string]string{"ARMATURE_TEST_ENV": "TEST-VALUE"},
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v stderr=%s", err, stderr.String())
	}
	if status != ProcessClean {
		t.Fatalf("expected clean status, got %v", status)
	}
	if stdout.String() != "TEST-VALUE" {
		t.Fatalf("expected injected env value, got %q", stdout.String())
	}
}

func TestNonInteractiveGitCommand(t *testing.T) {
	t.Setenv("GIT_DIR", "/tmp/other.git")
	t.Setenv("GIT_WORK_TREE", "/tmp/other")
	t.Setenv("GIT_COMMON_DIR", "/tmp/other.git")
	cmd := NonInteractiveGitCommand("/tmp", "version")
	found := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_TERMINAL_PROMPT=") {
			found = true
		}
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") || strings.HasPrefix(e, "GIT_COMMON_DIR=") {
			t.Fatalf("repo-selection env should be stripped, got %q", e)
		}
	}
	if !found {
		t.Fatal("expected GIT_TERMINAL_PROMPT in command env")
	}
}

func TestGitInitMainIgnoresInheritedGITDir(t *testing.T) {
	other := t.TempDir()
	otherInit := exec.CommandContext(context.Background(), "git", "init", other)
	otherInit.Env = overlayEnv(stripGitOverrideEnv(os.Environ()), []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	})
	if out, err := otherInit.CombinedOutput(); err != nil {
		t.Fatalf("git init other: %v: %s", err, out)
	}
	marker := filepath.Join(other, "KEEP")
	if err := os.WriteFile(marker, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	headBefore, err := os.ReadFile(filepath.Join(other, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	dir := t.TempDir()
	if err := GitInitMain(dir); err != nil {
		t.Fatalf("GitInitMain: %v", err)
	}

	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("expected %s to be initialized: %v", dir, err)
	}
	if !strings.Contains(string(head), "refs/heads/main") {
		t.Fatalf("initialized HEAD = %q, want refs/heads/main", head)
	}

	headAfter, err := os.ReadFile(filepath.Join(other, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	if string(headAfter) != string(headBefore) {
		t.Fatalf("other repo HEAD changed: %q -> %q", headBefore, headAfter)
	}
	keep, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(keep) != "untouched" {
		t.Fatalf("other worktree changed: %q", keep)
	}
	if _, err := os.Stat(filepath.Join(dir, "KEEP")); !os.IsNotExist(err) {
		t.Fatal("GitInitMain used GIT_DIR's worktree instead of dir")
	}
}

func TestGitInitMainIgnoresGlobalInitTemplateDir(t *testing.T) {
	template := t.TempDir()
	hooks := filepath.Join(template, "hooks")
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		t.Fatal(err)
	}
	hook := []byte("#!/bin/sh\necho template-hook-ran >&2\nexit 1\n")
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), hook, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	set := exec.CommandContext(context.Background(), "git", "config", "-f", cfg, "init.templateDir", template)
	if err := set.Run(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	dir := t.TempDir()
	if err := GitInitMain(dir); err != nil {
		t.Fatalf("GitInitMain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("template pre-commit should not be installed, err=%v", err)
	}
}

func TestGitLog_Success_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	dir := initGitRepoWithCommit(t, "feat(TASK-1): hello")
	out, err := GitLog(dir, "-n", "1", "--pretty=%s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "feat(TASK-1): hello") {
		t.Fatalf("expected commit subject in git log, got %q", out)
	}
}

func TestGitLog_EmptyButSuccessful_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	dir := initGitRepoWithCommit(t, "init")
	out, err := GitLog(dir, "-n", "0")
	if err != nil {
		t.Fatalf("empty successful git log must not error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got %q", out)
	}
}

func TestGitLog_InvalidRepo_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()
	out, err := GitLog("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for git log on invalid repo")
	}
	if !strings.Contains(err.Error(), "git log") {
		t.Fatalf("error must name the command, got %q", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for invalid repo, got %q", out)
	}
}

func initGitRepoWithCommit(t *testing.T, message string) string {
	t.Helper()
	dir := t.TempDir()
	if err := GitInitMain(dir); err != nil {
		t.Fatal(err)
	}
	run := exec.CommandContext(context.Background(), "git", "-C", dir, "config", "user.email", "test@test.com")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git config email: %v: %s", err, out)
	}
	run = exec.CommandContext(context.Background(), "git", "-C", dir, "config", "user.name", "Test")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git config name: %v: %s", err, out)
	}
	run = exec.CommandContext(context.Background(), "git", "-C", dir, "config", "commit.gpgsign", "false")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git config gpgsign: %v: %s", err, out)
	}
	run = exec.CommandContext(context.Background(), "git", "-C", dir, "commit", "--allow-empty", "-m", message)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return dir
}

func TestExecuteHook_Allow(t *testing.T) {
	t.Parallel()
	cmd := []string{"echo", `{"allowed":true,"message":""}`}
	input := HookInput{IssueID: "T1", FromStatus: "open", ToStatus: "done", WorkerID: "w1"}
	if err := ExecuteHook("test-hook", cmd, input); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteHook_Reject(t *testing.T) {
	t.Parallel()
	cmd := []string{"echo", `{"allowed":false,"message":"blocked"}`}
	input := HookInput{IssueID: "T1", FromStatus: "open", ToStatus: "done", WorkerID: "w1"}
	err := ExecuteHook("test-hook", cmd, input)
	if err == nil {
		t.Fatal("expected error for rejected hook")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected 'blocked' in error, got %q", err)
	}
}

func TestExecuteHook_BadOutput(t *testing.T) {
	t.Parallel()
	cmd := []string{"echo", "not-json"}
	input := HookInput{}
	err := ExecuteHook("test-hook", cmd, input)
	if err == nil {
		t.Fatal("expected error for invalid JSON output")
	}
}

func TestGitConfig_Unset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := exec.CommandContext(context.Background(), "git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}
	_, err := GitConfig(dir, "armature.nonexistent-key")
	if err == nil {
		t.Fatal("expected error for unset git config key")
	}
}

func TestHookInput_MarshalRoundtrip(t *testing.T) {
	t.Parallel()
	input := HookInput{IssueID: "T1", FromStatus: "open", ToStatus: "done", WorkerID: "w"}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var got HookInput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != input {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}
