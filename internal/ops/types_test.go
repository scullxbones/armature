package ops

import (
	"bytes"
	"encoding/json"
	"testing"
)

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
