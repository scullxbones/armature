package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// FingerprintContract computes a canonical SHA-256 fingerprint of a contract.
// The fingerprint is deterministic across identical contracts.
func FingerprintContract(contract Contract) string {
	// Serialize contract to canonical JSON for deterministic hashing
	data, err := json.Marshal(contract)
	if err != nil {
		// Should never happen for well-formed contracts
		panic(fmt.Sprintf("failed to marshal contract: %v", err))
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// FingerprintDelivery computes a canonical SHA-256 fingerprint of delivery metadata.
// The fingerprint is deterministic across identical delivery ranges.
func FingerprintDelivery(delivery Delivery) string {
	// Serialize delivery to canonical JSON for deterministic hashing
	data, err := json.Marshal(delivery)
	if err != nil {
		// Should never happen for well-formed deliveries
		panic(fmt.Sprintf("failed to marshal delivery: %v", err))
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// FingerprintResult computes a canonical SHA-256 fingerprint of a conformance assessment result.
// This fingerprint is used for idempotence detection and result validation.
func FingerprintResult(assessment ConformanceAssessment) string {
	// Serialize the entire assessment to canonical JSON
	data, err := json.Marshal(assessment)
	if err != nil {
		// Should never happen for well-formed assessments
		panic(fmt.Sprintf("failed to marshal assessment: %v", err))
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ComputeBundleID computes a canonical bundle identifier from review data.
// The format is "sha256:<64-char-hex-string>".
func ComputeBundleID(bundle ReviewBundle) string {
	// Serialize the bundle (without bundleID itself) to canonical JSON
	// We hash the core review data
	data := struct {
		SchemaVersion int
		Issue         IssueInfo
		Contract      Contract
		Delivery      Delivery
	}{
		SchemaVersion: bundle.SchemaVersion,
		Issue:         bundle.Issue,
		Contract:      bundle.Contract,
		Delivery:      bundle.Delivery,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		// Should never happen for well-formed bundles
		panic(fmt.Sprintf("failed to marshal bundle data: %v", err))
	}

	hash := sha256.Sum256(jsonData)
	hashStr := hex.EncodeToString(hash[:])
	return fmt.Sprintf("sha256:%s", hashStr)
}
