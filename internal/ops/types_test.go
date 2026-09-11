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

func TestOutcomeOp_RecordsTokenCounts_REQ_TOPTIER_S11_T1(t *testing.T) {
	t.Parallel()

	op := Op{
		Type:      OpTransition,
		TargetID:  "TOPTIER-S11-T1",
		Timestamp: 1740700800,
		WorkerID:  "worker-a1",
		Payload: Payload{
			To:           StatusDone,
			Outcome:      "Recorded optional token counts on the existing outcome op",
			InputTokens:  1200,
			OutputTokens: 340,
		},
	}
	line, err := MarshalOp(op)
	if err != nil {
		t.Fatalf("marshal outcome op: %v", err)
	}
	if !bytes.Contains(line, []byte(`"input_tokens":1200`)) {
		t.Errorf("expected input_tokens in outcome op JSON, got: %s", line)
	}
	if !bytes.Contains(line, []byte(`"output_tokens":340`)) {
		t.Errorf("expected output_tokens in outcome op JSON, got: %s", line)
	}

	parsed, err := ParseLine(line)
	if err != nil {
		t.Fatalf("parse outcome op: %v", err)
	}
	if parsed.Type != OpTransition {
		t.Errorf("expected type %q, got %q", OpTransition, parsed.Type)
	}
	if parsed.Payload.InputTokens != 1200 {
		t.Errorf("expected InputTokens 1200, got %d", parsed.Payload.InputTokens)
	}
	if parsed.Payload.OutputTokens != 340 {
		t.Errorf("expected OutputTokens 340, got %d", parsed.Payload.OutputTokens)
	}

	legacy := []byte(`["transition","TOPTIER-S11-T1",1740700800,"worker-a1",{"to":"done","outcome":"legacy outcome without token fields"}]`)
	legacyOp, err := ParseLine(legacy)
	if err != nil {
		t.Fatalf("legacy outcome op must still decode: %v", err)
	}
	if legacyOp.Payload.Outcome != "legacy outcome without token fields" {
		t.Errorf("expected legacy outcome preserved, got %q", legacyOp.Payload.Outcome)
	}
	if legacyOp.Payload.InputTokens != 0 || legacyOp.Payload.OutputTokens != 0 {
		t.Errorf("expected absent token fields to decode as 0, got input=%d output=%d",
			legacyOp.Payload.InputTokens, legacyOp.Payload.OutputTokens)
	}

	omitted, err := json.Marshal(Payload{To: StatusDone, Outcome: "no usage telemetry"})
	if err != nil {
		t.Fatalf("marshal payload without token counts: %v", err)
	}
	if bytes.Contains(omitted, []byte("input_tokens")) || bytes.Contains(omitted, []byte("output_tokens")) {
		t.Errorf("expected token fields omitted when zero, got: %s", omitted)
	}
}

func TestPayloadsEqual_LegacyOmitsTokens_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	legacy := Payload{To: StatusDone, Outcome: "legacy outcome without token fields"}
	retry := Payload{To: StatusDone, Outcome: "legacy outcome without token fields"}
	if !PayloadsEqual(legacy, retry) {
		t.Fatal("absent token fields must not make a legacy retry look like a new payload")
	}
	withTokens := Payload{To: StatusDone, Outcome: "legacy outcome without token fields", InputTokens: 1200}
	if PayloadsEqual(legacy, withTokens) {
		t.Fatal("token counts already on a recorded payload must participate in equality")
	}
}

func TestRecordedTransitionPayload_UsesLastOpWhenStatusMatches_REQ_AOC_S4_T1(t *testing.T) {
	t.Parallel()
	last := Payload{To: StatusDone, Outcome: "recorded", InputTokens: 50, OutputTokens: 10}
	got := RecordedTransitionPayload(StatusDone, "materialized", "", "", last, true)
	if !PayloadsEqual(got, last) {
		t.Fatalf("expected last transition payload when status matches, got %#v", got)
	}
	synthesized := RecordedTransitionPayload(StatusBlocked, "waiting on review now", "", "", last, true)
	want := Payload{To: StatusBlocked, Outcome: "waiting on review now"}
	if !PayloadsEqual(synthesized, want) {
		t.Fatalf("expected synthesized payload without invented tokens, got %#v", synthesized)
	}
}
