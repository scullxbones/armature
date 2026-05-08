package orchestrate

import (
	"strings"
	"testing"
)

// TestAssembleFeedback_RetryZero verifies that retry 0 returns empty feedback.
func TestAssembleFeedback_RetryZero(t *testing.T) {
	fb := AssembleFeedback(0, []string{"main.go"}, []CheckResult{})
	if fb.ScopeConstraint != "" {
		t.Errorf("retry 0: expected empty ScopeConstraint, got %q", fb.ScopeConstraint)
	}
	if len(fb.NamedFiles) != 0 {
		t.Errorf("retry 0: expected empty NamedFiles, got %v", fb.NamedFiles)
	}
}

// TestAssembleFeedback_RetryOne verifies that retry 1 returns empty feedback.
func TestAssembleFeedback_RetryOne(t *testing.T) {
	fb := AssembleFeedback(1, []string{"main.go"}, []CheckResult{})
	if fb.ScopeConstraint != "" {
		t.Errorf("retry 1: expected empty ScopeConstraint, got %q", fb.ScopeConstraint)
	}
	if len(fb.NamedFiles) != 0 {
		t.Errorf("retry 1: expected empty NamedFiles, got %v", fb.NamedFiles)
	}
}

// TestAssembleFeedback_RetryTwo verifies that retry 2 adds a negative scope constraint.
func TestAssembleFeedback_RetryTwo(t *testing.T) {
	fb := AssembleFeedback(2, []string{"main.go", "util.go"}, []CheckResult{})
	if fb.ScopeConstraint == "" {
		t.Error("retry 2: expected non-empty ScopeConstraint")
	}
	// Should be negative — tell the agent NOT to touch files outside scope.
	if !strings.Contains(strings.ToLower(fb.ScopeConstraint), "do not") &&
		!strings.Contains(strings.ToLower(fb.ScopeConstraint), "only") {
		t.Errorf("retry 2: ScopeConstraint should be a negative constraint, got %q", fb.ScopeConstraint)
	}
	// No named files yet on retry 2.
	if len(fb.NamedFiles) != 0 {
		t.Errorf("retry 2: expected empty NamedFiles, got %v", fb.NamedFiles)
	}
}

// TestAssembleFeedback_RetryThree verifies that retry 3 adds both scope constraint and named file list.
func TestAssembleFeedback_RetryThree(t *testing.T) {
	files := []string{"main.go", "util.go"}
	fb := AssembleFeedback(3, files, []CheckResult{})
	if fb.ScopeConstraint == "" {
		t.Error("retry 3: expected non-empty ScopeConstraint")
	}
	if len(fb.NamedFiles) == 0 {
		t.Error("retry 3: expected non-empty NamedFiles")
	}
	for _, f := range files {
		found := false
		for _, nf := range fb.NamedFiles {
			if nf == f {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("retry 3: file %q not found in NamedFiles %v", f, fb.NamedFiles)
		}
	}
}

// TestAssembleFeedback_RetryFourPlus verifies that retry 4+ behaves like retry 3.
func TestAssembleFeedback_RetryFourPlus(t *testing.T) {
	files := []string{"a.go", "b.go"}
	for _, retry := range []int{4, 5, 10} {
		fb := AssembleFeedback(retry, files, []CheckResult{})
		if fb.ScopeConstraint == "" {
			t.Errorf("retry %d: expected non-empty ScopeConstraint", retry)
		}
		if len(fb.NamedFiles) == 0 {
			t.Errorf("retry %d: expected non-empty NamedFiles", retry)
		}
	}
}

// TestAssembleFeedback_CheckResults verifies that failed check results appear in feedback.
func TestAssembleFeedback_CheckResults(t *testing.T) {
	checks := []CheckResult{
		{Name: "lint", Severity: SeverityError, Passed: false, Message: "unused variable x"},
		{Name: "test", Severity: SeverityError, Passed: false, Message: "FAIL: TestFoo"},
	}
	fb := AssembleFeedback(1, []string{"main.go"}, checks)
	if len(fb.FailedChecks) != 2 {
		t.Errorf("expected 2 failed checks, got %d", len(fb.FailedChecks))
	}
}

// TestAssembleFeedback_CheckResultsOnlyFailed verifies that passing checks are excluded.
func TestAssembleFeedback_CheckResultsOnlyFailed(t *testing.T) {
	checks := []CheckResult{
		{Name: "build", Severity: SeverityInfo, Passed: true, Message: "ok"},
		{Name: "lint", Severity: SeverityError, Passed: false, Message: "unused variable"},
	}
	fb := AssembleFeedback(1, []string{"main.go"}, checks)
	if len(fb.FailedChecks) != 1 {
		t.Errorf("expected 1 failed check, got %d", len(fb.FailedChecks))
	}
	if fb.FailedChecks[0].Name != "lint" {
		t.Errorf("expected failed check 'lint', got %q", fb.FailedChecks[0].Name)
	}
}
