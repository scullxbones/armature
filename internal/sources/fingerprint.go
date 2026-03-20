package sources

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint returns the lowercase hex-encoded SHA-256 hash of data.
func Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
