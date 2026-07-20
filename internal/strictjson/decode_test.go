package strictjson

import "testing"

// TestDecode_AllowsUnknownFields guards against rejecting schema-valid
// artifacts. The published schemas under docs/schemas/*.schema.json do not
// set additionalProperties: false, so a plan, review bundle, or conformance
// assessment carrying an extension/metadata field is schema-valid and must
// decode successfully rather than fail at runtime.
func TestDecode_AllowsUnknownFields(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `json:"name"`
	}
	var v target
	if err := Decode([]byte(`{"name":"a","extra_metadata":"x"}`), &v); err != nil {
		t.Fatalf("expected unknown field to be accepted, got error: %v", err)
	}
	if v.Name != "a" {
		t.Fatalf("expected name to decode, got %q", v.Name)
	}
}

func TestDecode_RejectsTrailingData(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `json:"name"`
	}
	var v target
	if err := Decode([]byte(`{"name":"a"}{"name":"b"}`), &v); err == nil {
		t.Fatalf("expected trailing JSON data to be rejected")
	}
}
