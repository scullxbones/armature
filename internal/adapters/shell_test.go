package adapters

import (
	"context"
	"encoding/json"
	"os/exec"
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
	t.Parallel()
	cmd := NonInteractiveGitCommand("/tmp", "version")
	found := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_TERMINAL_PROMPT=") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected GIT_TERMINAL_PROMPT in command env")
	}
}

func TestGitLog_InvalidRepo(t *testing.T) {
	t.Parallel()
	out, err := GitLog("/nonexistent/path")
	if err != nil {
		t.Fatal("expected nil error for git log on invalid repo, got", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for invalid repo, got %q", out)
	}
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
	// init a real git repo
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
