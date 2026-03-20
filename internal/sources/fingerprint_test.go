package sources

import (
	"testing"
)

// TestFingerprintKnownValue asserts that a known input produces the expected
// lowercase hex SHA-256 digest.
// echo -n "hello" | sha256sum => 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
func TestFingerprintKnownValue(t *testing.T) {
	input := []byte("hello")
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got := Fingerprint(input)
	if got != expected {
		t.Errorf("Fingerprint(%q) = %q; want %q", input, got, expected)
	}
}

// TestFingerprintDifferentInputs asserts that two different inputs produce
// different fingerprints.
func TestFingerprintDifferentInputs(t *testing.T) {
	a := Fingerprint([]byte("foo"))
	b := Fingerprint([]byte("bar"))
	if a == b {
		t.Errorf("expected different fingerprints for different inputs, both returned %q", a)
	}
}
