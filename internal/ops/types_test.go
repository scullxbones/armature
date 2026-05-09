package ops

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestOpCitationAccepted_RegisteredInValidOpTypes(t *testing.T) {
	if !ValidOpTypes[OpCitationAccepted] {
		t.Errorf("OpCitationAccepted (%q) is not registered in ValidOpTypes", OpCitationAccepted)
	}
}

func TestOpScopeRename_RegisteredInValidOpTypes(t *testing.T) {
	if !ValidOpTypes[OpScopeRename] {
		t.Errorf("OpScopeRename (%q) is not registered in ValidOpTypes", OpScopeRename)
	}
}

func TestOpScopeDelete_RegisteredInValidOpTypes(t *testing.T) {
	if !ValidOpTypes[OpScopeDelete] {
		t.Errorf("OpScopeDelete (%q) is not registered in ValidOpTypes", OpScopeDelete)
	}
}

func TestPayload_ScopeRenameFields(t *testing.T) {
	p := Payload{OldPath: "old/path", NewPath: "new/path"}
	if p.OldPath != "old/path" {
		t.Errorf("expected OldPath %q, got %q", "old/path", p.OldPath)
	}
	if p.NewPath != "new/path" {
		t.Errorf("expected NewPath %q, got %q", "new/path", p.NewPath)
	}
}

func TestPayload_ScopeDeleteField(t *testing.T) {
	p := Payload{DeletedPath: "some/path"}
	if p.DeletedPath != "some/path" {
		t.Errorf("expected DeletedPath %q, got %q", "some/path", p.DeletedPath)
	}
}

func TestPayload_PreferredModel_RoundTripsJSON(t *testing.T) {
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

// Orchestration op type constants must all be registered in ValidOpTypes.
func TestOrchestrationOpConstants_RegisteredInValidOpTypes(t *testing.T) {
	orchOps := []struct {
		name   string
		opType string
	}{
		{"OpOrchestrateStart", OpOrchestrateStart},
		{"OpOrchestrateDispatch", OpOrchestrateDispatch},
		{"OpOrchestrateDispatchComplete", OpOrchestrateDispatchComplete},
		{"OpOrchestrateVerifyFail", OpOrchestrateVerifyFail},
		{"OpOrchestrateRetry", OpOrchestrateRetry},
		{"OpOrchestrateEscalate", OpOrchestrateEscalate},
		{"OpOrchestrateComplete", OpOrchestrateComplete},
		{"OpOrchestrateCheckResult", OpOrchestrateCheckResult},
	}
	for _, tc := range orchOps {
		if !ValidOpTypes[tc.opType] {
			t.Errorf("%s (%q) is not registered in ValidOpTypes", tc.name, tc.opType)
		}
	}
}

func TestOrchestrationOpConstants_Values(t *testing.T) {
	// Verify the string values are as expected.
	cases := []struct {
		constant string
		want     string
	}{
		{OpOrchestrateStart, "orchestrate-start"},
		{OpOrchestrateDispatch, "orchestrate-dispatch"},
		{OpOrchestrateDispatchComplete, "orchestrate-dispatch-complete"},
		{OpOrchestrateVerifyFail, "orchestrate-verify-fail"},
		{OpOrchestrateRetry, "orchestrate-retry"},
		{OpOrchestrateEscalate, "orchestrate-escalate"},
		{OpOrchestrateComplete, "orchestrate-complete"},
		{OpOrchestrateCheckResult, "orchestrate-check-result"},
	}
	for _, tc := range cases {
		if tc.constant != tc.want {
			t.Errorf("expected %q, got %q", tc.want, tc.constant)
		}
	}
}

func TestFailureRecord_Fields(t *testing.T) {
	// FailureRecord must be a typed struct with IssueID, Reason, and Timestamp fields.
	fr := FailureRecord{
		IssueID:   "E7-S1-T4",
		Reason:    "test failure",
		Timestamp: 1234567890,
	}
	if fr.IssueID != "E7-S1-T4" {
		t.Errorf("expected IssueID %q, got %q", "E7-S1-T4", fr.IssueID)
	}
	if fr.Reason != "test failure" {
		t.Errorf("expected Reason %q, got %q", "test failure", fr.Reason)
	}
	if fr.Timestamp != 1234567890 {
		t.Errorf("expected Timestamp 1234567890, got %d", fr.Timestamp)
	}
}

func TestFailureRecord_JSONRoundTrip(t *testing.T) {
	fr := FailureRecord{
		IssueID:   "E7-S1-T4",
		Reason:    "verify failed",
		Timestamp: 9999,
	}
	data, err := json.Marshal(fr)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var fr2 FailureRecord
	if err := json.Unmarshal(data, &fr2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if fr2.IssueID != fr.IssueID {
		t.Errorf("expected IssueID %q, got %q", fr.IssueID, fr2.IssueID)
	}
	if fr2.Reason != fr.Reason {
		t.Errorf("expected Reason %q, got %q", fr.Reason, fr2.Reason)
	}
	if fr2.Timestamp != fr.Timestamp {
		t.Errorf("expected Timestamp %d, got %d", fr.Timestamp, fr2.Timestamp)
	}
}
