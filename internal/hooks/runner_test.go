package hooks

import (
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
)

func TestRunPreTransition_NoHooks(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Hooks: nil}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	if err := RunPreTransition(cfg, input); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRunPreTransition_AllowingHook(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Hooks: []config.HookConfig{
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
	cfg := &config.Config{
		Hooks: []config.HookConfig{
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
	cfg := &config.Config{
		Hooks: []config.HookConfig{
			{Name: "fail-hook", Command: []string{"sh", "-c", `exit 1`}},
		},
	}
	input := adapters.HookInput{IssueID: "1", FromStatus: "open", ToStatus: "in-progress", WorkerID: "w1"}
	err := RunPreTransition(cfg, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
