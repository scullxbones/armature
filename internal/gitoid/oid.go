// Package gitoid is the single helper for full git object-id hex strings.
// The published review-bundle schema and every Go caller share one predicate:
// lowercase hex of length 40 (sha1) or 64 (sha256). Valid compiles PatternLower.
package gitoid

import "regexp"

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
