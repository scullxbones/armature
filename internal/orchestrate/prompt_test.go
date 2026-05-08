package orchestrate

import (
	"strings"
	"testing"
)

// TestAssemblePrompt_ContainsContext verifies the prompt includes the provided context block.
func TestAssemblePrompt_ContainsContext(t *testing.T) {
	ctx := "You are a Go developer working on a CLI tool."
	prompt := AssemblePrompt(ctx, Feedback{})
	if !strings.Contains(prompt, ctx) {
		t.Errorf("prompt missing context block; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_ContainsDoNotCommit verifies the 'Do not commit' instruction is always present.
func TestAssemblePrompt_ContainsDoNotCommit(t *testing.T) {
	prompt := AssemblePrompt("some context", Feedback{})
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "do not commit") {
		t.Errorf("prompt missing 'Do not commit' instruction; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_ContainsScopeConstraint verifies scope constraint is rendered when present.
func TestAssemblePrompt_ContainsScopeConstraint(t *testing.T) {
	fb := Feedback{ScopeConstraint: "Only modify files in the scope list."}
	prompt := AssemblePrompt("context", fb)
	if !strings.Contains(prompt, fb.ScopeConstraint) {
		t.Errorf("prompt missing ScopeConstraint; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_NoScopeConstraint verifies no spurious scope text when ScopeConstraint is empty.
func TestAssemblePrompt_NoScopeConstraint(t *testing.T) {
	prompt := AssemblePrompt("context", Feedback{ScopeConstraint: ""})
	// The literal token "ScopeConstraint" should not appear in rendered output.
	if strings.Contains(prompt, "ScopeConstraint") {
		t.Errorf("prompt should not contain 'ScopeConstraint' label; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_ContainsNamedFiles verifies named files are listed when present.
func TestAssemblePrompt_ContainsNamedFiles(t *testing.T) {
	fb := Feedback{
		ScopeConstraint: "Only touch these files.",
		NamedFiles:      []string{"main.go", "util.go"},
	}
	prompt := AssemblePrompt("context", fb)
	for _, f := range fb.NamedFiles {
		if !strings.Contains(prompt, f) {
			t.Errorf("prompt missing named file %q; got:\n%s", f, prompt)
		}
	}
}

// TestAssemblePrompt_ContainsFailedChecks verifies failed check messages appear in prompt.
func TestAssemblePrompt_ContainsFailedChecks(t *testing.T) {
	fb := Feedback{
		FailedChecks: []CheckResult{
			{Name: "lint", Severity: SeverityError, Passed: false, Message: "unused variable x"},
		},
	}
	prompt := AssemblePrompt("context", fb)
	if !strings.Contains(prompt, "lint") {
		t.Errorf("prompt missing failed check name 'lint'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "unused variable x") {
		t.Errorf("prompt missing failed check message; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_EmptyFeedback verifies clean prompt with no feedback block.
func TestAssemblePrompt_EmptyFeedback(t *testing.T) {
	prompt := AssemblePrompt("context", Feedback{})
	if len(prompt) == 0 {
		t.Error("expected non-empty prompt")
	}
	// No failed checks block should clutter the output.
	if strings.Contains(prompt, "Failed checks:") {
		t.Errorf("empty feedback should not emit 'Failed checks:' block; got:\n%s", prompt)
	}
}

// TestAssemblePrompt_OrderContextFirst verifies context block appears before instructions.
func TestAssemblePrompt_OrderContextFirst(t *testing.T) {
	ctx := "UNIQUE_CONTEXT_STRING"
	prompt := AssemblePrompt(ctx, Feedback{})
	ctxIdx := strings.Index(prompt, ctx)
	commitIdx := strings.Index(strings.ToLower(prompt), "do not commit")
	if ctxIdx == -1 {
		t.Fatal("context not found in prompt")
	}
	if commitIdx == -1 {
		t.Fatal("'do not commit' not found in prompt")
	}
	if ctxIdx > commitIdx {
		t.Errorf("context block should appear before 'Do not commit' instruction; ctxIdx=%d commitIdx=%d", ctxIdx, commitIdx)
	}
}
