package orchestrate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scullxbones/armature/internal/orchestrate"
)

// --- ExitStatus.String() ---

func TestExitStatusString(t *testing.T) {
	cases := []struct {
		status orchestrate.ExitStatus
		want   string
	}{
		{orchestrate.ExitSuccess, "success"},
		{orchestrate.ExitFailure, "failure"},
		{orchestrate.ExitTimeout, "timeout"},
		{orchestrate.ExitCancelled, "cancelled"},
		{orchestrate.ExitStatus(99), "unknown"},
	}
	for _, tc := range cases {
		got := tc.status.String()
		if got != tc.want {
			t.Errorf("ExitStatus(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// --- CheckSeverity.String() ---

func TestCheckSeverityString(t *testing.T) {
	cases := []struct {
		severity orchestrate.CheckSeverity
		want     string
	}{
		{orchestrate.SeverityInfo, "info"},
		{orchestrate.SeverityWarning, "warning"},
		{orchestrate.SeverityError, "error"},
		{orchestrate.CheckSeverity(99), "unknown"},
	}
	for _, tc := range cases {
		got := tc.severity.String()
		if got != tc.want {
			t.Errorf("CheckSeverity(%d).String() = %q, want %q", tc.severity, got, tc.want)
		}
	}
}

// --- HarnessConfig round-trip JSON ---

func TestHarnessConfigRoundTrip(t *testing.T) {
	original := orchestrate.HarnessConfig{
		BuildCmd:       "make build",
		LintCmd:        "make lint",
		TestCmd:        "make test",
		CoverageCmd:    "make coverage",
		MutateCmd:      "make mutate",
		WorkDir:        "/tmp/project",
		TimeoutSeconds: 120,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded orchestrate.HarnessConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.BuildCmd != original.BuildCmd ||
		decoded.LintCmd != original.LintCmd ||
		decoded.TestCmd != original.TestCmd ||
		decoded.CoverageCmd != original.CoverageCmd ||
		decoded.MutateCmd != original.MutateCmd ||
		decoded.WorkDir != original.WorkDir ||
		decoded.TimeoutSeconds != original.TimeoutSeconds {
		t.Errorf("round-trip mismatch\n got:  %+v\n want: %+v", decoded, original)
	}
}

func TestHarnessConfigOmitemptyAbsent(t *testing.T) {
	// A zero-value HarnessConfig should marshal with no keys.
	var cfg orchestrate.HarnessConfig
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("expected empty JSON object, got %s", data)
	}
}

// --- InvocationResult round-trip ---

func TestInvocationResultRoundTrip(t *testing.T) {
	original := orchestrate.InvocationResult{
		Status:     orchestrate.ExitFailure,
		ExitCode:   1,
		Stdout:     "stdout text",
		Stderr:     "stderr text",
		DurationMs: 42,
	}

	// Verify field values survive a copy (value semantics).
	copy := original
	if copy.Status != original.Status || copy.ExitCode != original.ExitCode ||
		copy.Stdout != original.Stdout || copy.Stderr != original.Stderr ||
		copy.DurationMs != original.DurationMs {
		t.Errorf("InvocationResult copy mismatch: got %+v, want %+v", copy, original)
	}
}

// --- CheckResult field integrity ---

func TestCheckResultFields(t *testing.T) {
	inv := orchestrate.InvocationResult{Status: orchestrate.ExitSuccess, ExitCode: 0}
	cr := orchestrate.CheckResult{
		Name:       "build",
		Severity:   orchestrate.SeverityError,
		Passed:     true,
		Message:    "build passed",
		Invocation: inv,
	}

	if cr.Name != "build" {
		t.Errorf("Name: got %q, want %q", cr.Name, "build")
	}
	if cr.Severity != orchestrate.SeverityError {
		t.Errorf("Severity: got %v, want %v", cr.Severity, orchestrate.SeverityError)
	}
	if !cr.Passed {
		t.Error("Passed: expected true")
	}
	if cr.Invocation.Status != orchestrate.ExitSuccess {
		t.Errorf("Invocation.Status: got %v, want %v", cr.Invocation.Status, orchestrate.ExitSuccess)
	}
}

// --- RunOptions defaults ---

func TestRunOptionsZeroValue(t *testing.T) {
	var opts orchestrate.RunOptions
	if opts.DryRun {
		t.Error("DryRun: expected false by default")
	}
	if opts.MaxParallel != 0 {
		t.Errorf("MaxParallel: expected 0, got %d", opts.MaxParallel)
	}
	if opts.Env != nil {
		t.Errorf("Env: expected nil, got %v", opts.Env)
	}
}

// --- OrchestrateState accumulation ---

func TestOrchestrateStateAccumulation(t *testing.T) {
	state := orchestrate.OrchestrateState{
		RunID:     "run-001",
		Phase:     "build",
		StartedAt: 1000,
	}

	check := orchestrate.CheckResult{
		Name:     "build",
		Severity: orchestrate.SeverityError,
		Passed:   false,
		Message:  "compile error",
	}
	state.Checks = append(state.Checks, check)
	state.Failed = true
	state.FinishedAt = 1005

	if len(state.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(state.Checks))
	}
	if !state.Failed {
		t.Error("expected Failed to be true")
	}
	if state.FinishedAt != 1005 {
		t.Errorf("FinishedAt: got %d, want 1005", state.FinishedAt)
	}
}

// --- HarnessAdapter interface satisfaction ---

// stubAdapter is a local no-op adapter used to verify the interface.
type stubAdapter struct{ name string }

func (s *stubAdapter) Name() string { return s.name }
func (s *stubAdapter) Run(_ context.Context, _ orchestrate.HarnessConfig, _ orchestrate.RunOptions) (orchestrate.CheckResult, error) {
	return orchestrate.CheckResult{Name: s.name, Passed: true}, nil
}

// Compile-time assertion: *stubAdapter implements HarnessAdapter.
var _ orchestrate.HarnessAdapter = (*stubAdapter)(nil)

func TestHarnessAdapterInterface(t *testing.T) {
	var adapter orchestrate.HarnessAdapter = &stubAdapter{name: "stub"}
	if adapter.Name() != "stub" {
		t.Errorf("Name(): got %q, want %q", adapter.Name(), "stub")
	}

	result, err := adapter.Run(context.Background(), orchestrate.HarnessConfig{}, orchestrate.RunOptions{})
	if err != nil {
		t.Fatalf("Run(): unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("Run(): expected Passed = true")
	}
}
