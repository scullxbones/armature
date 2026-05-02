package ops

import "testing"

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
