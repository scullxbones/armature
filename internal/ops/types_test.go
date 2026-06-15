package ops

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestOpCitationAccepted_RegisteredInValidOpTypes(t *testing.T) {
	t.Parallel()
	if !ValidOpTypes[OpCitationAccepted] {
		t.Errorf("OpCitationAccepted (%q) is not registered in ValidOpTypes", OpCitationAccepted)
	}
}

func TestOpScopeRename_RegisteredInValidOpTypes(t *testing.T) {
	t.Parallel()
	if !ValidOpTypes[OpScopeRename] {
		t.Errorf("OpScopeRename (%q) is not registered in ValidOpTypes", OpScopeRename)
	}
}

func TestOpScopeDelete_RegisteredInValidOpTypes(t *testing.T) {
	t.Parallel()
	if !ValidOpTypes[OpScopeDelete] {
		t.Errorf("OpScopeDelete (%q) is not registered in ValidOpTypes", OpScopeDelete)
	}
}

func TestPayload_ScopeRenameFields(t *testing.T) {
	t.Parallel()
	p := Payload{OldPath: "old/path", NewPath: "new/path"}
	if p.OldPath != "old/path" {
		t.Errorf("expected OldPath %q, got %q", "old/path", p.OldPath)
	}
	if p.NewPath != "new/path" {
		t.Errorf("expected NewPath %q, got %q", "new/path", p.NewPath)
	}
}

func TestPayload_ScopeDeleteField(t *testing.T) {
	t.Parallel()
	p := Payload{DeletedPath: "some/path"}
	if p.DeletedPath != "some/path" {
		t.Errorf("expected DeletedPath %q, got %q", "some/path", p.DeletedPath)
	}
}

func TestPayload_PreferredModel_RoundTripsJSON(t *testing.T) {
	t.Parallel()
	// Payload.PreferredModel must survive JSONL encode/decode.
	p := Payload{PreferredModel: "claude-opus-4"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var p2 Payload
	if err := json.Unmarshal(data, &p2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if p2.PreferredModel != "claude-opus-4" {
		t.Errorf("expected PreferredModel %q after round-trip, got %q", "claude-opus-4", p2.PreferredModel)
	}
}

func TestPayload_PreferredModel_OmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	// When PreferredModel is empty, it must not appear in JSON (omitempty).
	p := Payload{Title: "some task"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if bytes.Contains(data, []byte("preferred_model")) {
		t.Errorf("expected preferred_model to be absent from JSON when empty, got: %s", data)
	}
}

func TestPayload_SourceEntryID_RoundTripsJSON(t *testing.T) {
	t.Parallel()
	// Payload.SourceEntryID must survive JSONL encode/decode.
	p := Payload{SourceEntryID: "entry-abc123"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var p2 Payload
	if err := json.Unmarshal(data, &p2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if p2.SourceEntryID != "entry-abc123" {
		t.Errorf("expected SourceEntryID %q after round-trip, got %q", "entry-abc123", p2.SourceEntryID)
	}
}

func TestPayload_SourceEntryID_OmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	// When SourceEntryID is empty, it must not appear in JSON (omitempty).
	p := Payload{Title: "some task"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if bytes.Contains(data, []byte("source_entry_id")) {
		t.Errorf("expected source_entry_id to be absent from JSON when empty, got: %s", data)
	}
}

// TestManagedExecutionOperationTypesAreInvalid verifies that all nine
// managed-execution op types are absent from ValidOpTypes after removal.
func TestManagedExecutionOperationTypesAreInvalid(t *testing.T) {
	t.Parallel()
	removed := []string{
		"orchestrate-start",
		"orchestrate-dispatch",
		"orchestrate-dispatch-complete",
		"orchestrate-verify-fail",
		"orchestrate-retry",
		"orchestrate-escalate",
		"orchestrate-complete",
		"orchestrate-check-result",
		"worker-runtime-decision",
	}
	for _, opType := range removed {
		if ValidOpTypes[opType] {
			t.Errorf("managed-execution op type %q must not be in ValidOpTypes", opType)
		}
	}
}
