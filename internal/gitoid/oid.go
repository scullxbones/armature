// Package gitoid is the single helper for full git object-id hex strings.
// The published review-bundle schema and every Go caller share one predicate:
// lowercase hex of length 40 (sha1) or 64 (sha256). Valid compiles PatternLower.
// This package is the only place that may encode SHA-1/SHA-256 hash widths.
package gitoid

import (
	"regexp"
	"strings"
)

// Hex widths git emits for each object format.
const (
	LenSHA1   = 40
	LenSHA256 = 64
)

// PatternLower is the canonical object-id pattern: lowercase 40-hex or 64-hex.
// docs/schemas/review-bundle.schema.json delivery SHAs must equal this string.
const PatternLower = `^[a-f0-9]{40}$|^[a-f0-9]{64}$`

var patternLower = regexp.MustCompile(PatternLower)

// Valid reports whether s matches PatternLower exactly (lowercase 40|64 hex).
func Valid(s string) bool {
	return patternLower.MatchString(s)
}

// PadSHA1 returns a LenSHA1 lowercase-hex string by repeating prefix and
// truncating. Callers that need a distinctive fixture OID must go through
// here (or LenSHA1 + Repeat) so hex-width knowledge stays in this package.
func PadSHA1(prefix string) string {
	return padHex(prefix, LenSHA1)
}

// PadSHA256 is the LenSHA256 counterpart of PadSHA1.
func PadSHA256(prefix string) string {
	return padHex(prefix, LenSHA256)
}

func padHex(prefix string, n int) string {
	if prefix == "" {
		prefix = "a"
	}
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(prefix)
	}
	out := strings.ToLower(b.String()[:n])
	return out
}
