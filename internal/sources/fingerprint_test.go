package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestFingerprintKnownValue(t *testing.T) {
	t.Parallel()
	input := []byte("hello")
	sum := sha256.Sum256(input)
	expected := hex.EncodeToString(sum[:])
	got := Fingerprint(input)
	if got != expected {
		t.Errorf("Fingerprint(%q) = %q; want %q", input, got, expected)
	}
}

func TestFingerprintDifferentInputs(t *testing.T) {
	t.Parallel()
	a := Fingerprint([]byte("foo"))
	b := Fingerprint([]byte("bar"))
	if a == b {
		t.Errorf("expected different fingerprints for different inputs, both returned %q", a)
	}
}
