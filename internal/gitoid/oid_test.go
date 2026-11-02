package gitoid

import (
	"regexp"
	"strings"
	"testing"
)

func TestValid_SHA1AndSHA256_REQ_PV70(t *testing.T) {
	t.Parallel()

	if !Valid(strings.Repeat("a", LenSHA1)) {
		t.Fatal("40-hex lowercase must be accepted")
	}
	if !Valid(strings.Repeat("b", LenSHA256)) {
		t.Fatal("64-hex lowercase must be accepted (mutation to 40-only must fail here)")
	}

	bads := []string{
		"", "abc",
		strings.Repeat("0", LenSHA1-1), strings.Repeat("0", LenSHA1+1),
		strings.Repeat("0", LenSHA256-1), strings.Repeat("0", LenSHA256+1),
		strings.Repeat("g", LenSHA1),
		strings.Repeat("A", LenSHA1), // uppercase rejected (schema Exact)
		strings.Repeat("B", LenSHA256),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaA", // mixed case
	}
	for _, bad := range bads {
		if Valid(bad) {
			t.Fatalf("accepted invalid oid %q", bad)
		}
	}
}

func TestPadSHA1OwnsWidth_REQ_419(t *testing.T) {
	t.Parallel()
	got := PadSHA1("ab")
	if len(got) != LenSHA1 {
		t.Fatalf("PadSHA1 len=%d want %d", len(got), LenSHA1)
	}
	if !Valid(got) {
		t.Fatalf("PadSHA1 must yield Valid OID, got %q", got)
	}
	got64 := PadSHA256("cd")
	if len(got64) != LenSHA256 || !Valid(got64) {
		t.Fatalf("PadSHA256 invalid: %q", got64)
	}
}

func TestValidEqualsPatternLower_REQ_PV70(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(PatternLower)
	cases := []string{
		"",
		strings.Repeat("a", LenSHA1),
		strings.Repeat("f", LenSHA256),
		strings.Repeat("A", LenSHA1),
		strings.Repeat("0", LenSHA1-1),
		strings.Repeat("0", LenSHA1+1),
		strings.Repeat("0", LenSHA256),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaG",
		"not-a-git-sha",
	}
	for _, s := range cases {
		if got, want := Valid(s), re.MatchString(s); got != want {
			t.Fatalf("Valid(%q)=%v PatternLower=%v (drift / mutate-to-A-F must fail)", s, got, want)
		}
	}
	if !strings.Contains(PatternLower, "{40}") || !strings.Contains(PatternLower, "{64}") {
		t.Fatalf("PatternLower must encode both widths: %s", PatternLower)
	}
	if strings.Contains(PatternLower, "A-F") || strings.Contains(PatternLower, "a-fA-F") {
		t.Fatal("PatternLower must stay lowercase-only; mutate to accept A-F must fail")
	}
}
