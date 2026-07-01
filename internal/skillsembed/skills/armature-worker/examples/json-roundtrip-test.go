//go:build ignore
// +build ignore

package examples

import (
	"encoding/json"
	"strings"
	"testing"
)

// Example: round-trip JSON fixture test for a Go type documented in a skill or CONTEXT.md.
// Adapt the type, field, and expected JSON form to your task. This test fails immediately
// if the type uses integer marshaling while the skill/CONTEXT.md documents strings (or vice versa).
//
// DO NOT compile this file into the binary. Use as a template when your task:
// 1. Adds or modifies a Go type
// 2. Documents that type's JSON format in a skill (SKILL.md) or CONTEXT.md
//
// Copy this pattern into your actual test file and replace the type, field names,
// and expected JSON values with those from your task.
func TestCriterionStatus_JSONRoundTrip_REQ_DF_S5_T6(t *testing.T) {
	// Example type stub (replace with your actual type when adapting)
	type CriterionStatus string
	const StatusSatisfied CriterionStatus = "satisfied"

	type CriterionResult struct {
		Status CriterionStatus `json:"status"`
	}

	// Step 1: Unmarshal from JSON string
	raw := `{"status":"satisfied"}`
	var result CriterionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Step 2: Verify the Go value matches expectation
	if result.Status != StatusSatisfied {
		t.Errorf("got %v, want StatusSatisfied", result.Status)
	}

	// Step 3: Marshal back to JSON
	out, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Step 4: Verify the JSON form uses strings (not integers)
	// This catches the bug where a field is documented as a string in SKILL.md
	// but the Go code uses integer enum values or omits a custom MarshalJSON.
	if !strings.Contains(string(out), `"satisfied"`) {
		t.Errorf("marshal produced %s, want string form with \"satisfied\"", out)
	}
}
