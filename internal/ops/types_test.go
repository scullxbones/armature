package ops

import "testing"

func TestOpCitationAccepted_RegisteredInValidOpTypes(t *testing.T) {
	if !ValidOpTypes[OpCitationAccepted] {
		t.Errorf("OpCitationAccepted (%q) is not registered in ValidOpTypes", OpCitationAccepted)
	}
}
