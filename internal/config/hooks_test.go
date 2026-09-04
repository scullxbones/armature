package config

import (
	"os"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
)

func TestRunPreTransition_NoHooks(t *testing.T) {
	t.Parallel()
	cfg := &Config{Hooks: nil}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	if err := RunPreTransition(cfg, input); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunPreTransition_AllowingHook(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Hooks: []HookConfig{
			{Name: "allow-hook", Command: []string{"sh", "-c", `echo '{"allowed":true}'`}},
		},
	}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	if err := RunPreTransition(cfg, input); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunPreTransition_RejectingHook(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Hooks: []HookConfig{
			{Name: "reject-hook", Command: []string{"sh", "-c", `echo '{"allowed":false,"message":"not ready"}'`}},
		},
	}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	err := RunPreTransition(cfg, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("expected error to contain 'not ready', got: %v", err)
	}
}

func TestRunPreTransition_FailingHook(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Hooks: []HookConfig{
			{Name: "fail-hook", Command: []string{"sh", "-c", `exit 1`}},
		},
	}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	err := RunPreTransition(cfg, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRunPreTransition_CommandInjectionMitigated(t *testing.T) {
	t.Parallel()
	tempFile := "vulnerable_marker_mitigated"
	defer func() {
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to clean up temp file %q: %v", tempFile, err)
		}
	}()

	cfg := &Config{
		Hooks: []HookConfig{
			{
				Name: "mitigated-hook",
				// ';' is passed as an argument to echo, not executed by a shell.
				Command: []string{"echo", `{"allowed":true}`, ";", "touch", tempFile},
			},
		},
	}

	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	err := RunPreTransition(cfg, input)
	if err == nil {
		t.Fatal("expected error due to invalid JSON output from echo, got nil")
	}

	if _, err := os.Stat(tempFile); err == nil {
		t.Errorf("vulnerability still present: %s was created", tempFile)
	}
}
