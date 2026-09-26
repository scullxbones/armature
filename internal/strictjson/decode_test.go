package strictjson

import "testing"

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
