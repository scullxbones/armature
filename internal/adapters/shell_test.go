package adapters

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestRunCommand_Success(t *testing.T) {
	out, err := RunCommand("echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("expected 'hello', got %q", out)
	}
}

func TestRunCommand_Error(t *testing.T) {
	_, err := RunCommand("false")
	if err == nil {
		t.Fatal("expected error from false")
	}
}

func TestRunCommandOutput_Success(t *testing.T) {
	out, err := RunCommandOutput("echo", "world")
	if err != nil {
		t.Fatal(err)
	}
	if out != "world" {
		t.Fatalf("expected 'world', got %q", out)
	}
}

func TestRunCommandOutput_Error(t *testing.T) {
	_, err := RunCommandOutput("false")
	if err == nil {
		t.Fatal("expected error from false")
	}
}

func TestRunShellScript_Success(t *testing.T) {
	out, err := RunShellScript("cat", []byte("test input"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "test input" {
		t.Fatalf("expected 'test input', got %q", out)
	}
}

func TestRunShellScript_Error(t *testing.T) {
	_, err := RunShellScript("exit 1", nil)
	if err == nil {
		t.Fatal("expected error from failing script")
	}
}

func TestNonInteractiveGitCommand(t *testing.T) {
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
	out, err := GitLog("/nonexistent/path")
	if err != nil {
		t.Fatal("expected nil error for git log on invalid repo, got", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for invalid repo, got %q", out)
	}
}

func TestExecuteHook_Allow(t *testing.T) {
	script := `echo '{"allowed":true,"message":""}'`
	input := HookInput{IssueID: "T1", FromStatus: "open", ToStatus: "done", WorkerID: "w1"}
	if err := ExecuteHook("test-hook", script, input); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteHook_Reject(t *testing.T) {
	script := `echo '{"allowed":false,"message":"blocked"}'`
	input := HookInput{IssueID: "T1", FromStatus: "open", ToStatus: "done", WorkerID: "w1"}
	err := ExecuteHook("test-hook", script, input)
	if err == nil {
		t.Fatal("expected error for rejected hook")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected 'blocked' in error, got %q", err)
	}
}

func TestExecuteHook_BadOutput(t *testing.T) {
	script := `echo 'not-json'`
	input := HookInput{}
	err := ExecuteHook("test-hook", script, input)
	if err == nil {
		t.Fatal("expected error for invalid JSON output")
	}
}

func TestGitConfig_Unset(t *testing.T) {
	dir := t.TempDir()
	// init a real git repo
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}
	_, err := GitConfig(dir, "armature.nonexistent-key")
	if err == nil {
		t.Fatal("expected error for unset git config key")
	}
}

func TestGitConfigMode_Default(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}
	mode, err := GitConfigMode(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "single-branch" {
		t.Fatalf("expected default 'single-branch', got %q", mode)
	}
}

func TestHookInput_MarshalRoundtrip(t *testing.T) {
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
