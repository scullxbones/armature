package review

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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

// ActivityLogEntry represents a parsed line from the activity log (key=value format).
type ActivityLogEntry struct {
	Timestamp   string
	Command     string
	ExitCode    int
	HeadSHA     string
	OutputHash  string
	OutputTrunc string // full output or truncated form
}

// parseActivityLogFile reads the activity log file and returns parsed entries and the raw file content.
// The file is in a custom key=value format (one entry per line).
// Returns the list of entries, raw file content for digest computation, and any error.
func parseActivityLogFile(logPath string) ([]ActivityLogEntry, []byte, error) {
	content, err := os.ReadFile(logPath) //nolint:gosec // G304: logPath is provided by Prepare
	if err != nil {
		return nil, nil, fmt.Errorf("read activity log: %w", err)
	}

	var entries []ActivityLogEntry
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry, err := parseActivityLogLine(line)
		if err != nil {
			// Log malformed lines gracefully — do not fail the entire bundle preparation
			// but continue parsing remaining entries
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan activity log: %w", err)
	}

	return entries, content, nil
}

// parseActivityLogLine parses a single activity log line in key=value format.
// Example: 2026-01-15T10:30:45Z activity: command="make build" exit_code=0 head_sha=abc123...
func parseActivityLogLine(line string) (ActivityLogEntry, error) {
	entry := ActivityLogEntry{}

	// Extract timestamp (everything before " activity:")
	parts := strings.SplitN(line, " activity: ", 2)
	if len(parts) != 2 {
		return entry, fmt.Errorf("malformed activity log line: missing 'activity:' marker")
	}

	entry.Timestamp = parts[0]
	kvPart := parts[1]

	// Parse key=value pairs
	// Simple parser: split by space, but be careful with quoted strings
	fields := parseKeyValuePairs(kvPart)

	for key, value := range fields {
		switch key {
		case "command":
			entry.Command = value
		case "exit_code":
			// Parse as int; silently use default (0) if parsing fails
			if exitCode, err := strconv.Atoi(value); err == nil {
				entry.ExitCode = exitCode
			}
		case "head_sha":
			entry.HeadSHA = value
		case "output_hash":
			entry.OutputHash = value
		case "output":
			entry.OutputTrunc = value
		case "output_truncated":
			entry.OutputTrunc = value
		}
	}

	return entry, nil
}

// parseKeyValuePairs parses simple key=value pairs from a string.
// Handles quoted values properly.
func parseKeyValuePairs(input string) map[string]string {
	result := make(map[string]string)
	var i int
	for i < len(input) {
		// Skip whitespace
		for i < len(input) && input[i] == ' ' {
			i++
		}
		if i >= len(input) {
			break
		}

		// Extract key
		keyStart := i
		for i < len(input) && input[i] != '=' {
			i++
		}
		key := input[keyStart:i]

		if i >= len(input) || input[i] != '=' {
			break
		}
		i++ // skip '='

		// Extract value (handle quoted strings)
		var value string
		if i < len(input) && input[i] == '"' {
			i++ // skip opening quote
			valueStart := i
			for i < len(input) && input[i] != '"' {
				if input[i] == '\\' && i+1 < len(input) {
					i += 2 // skip escape sequence
				} else {
					i++
				}
			}
			value = input[valueStart:i]
			if i < len(input) {
				i++ // skip closing quote
			}
		} else {
			// Unquoted value (read until space)
			valueStart := i
			for i < len(input) && input[i] != ' ' {
				i++
			}
			value = input[valueStart:i]
		}

		result[key] = value
	}

	return result
}

// FingerprintActivity computes a SHA-256 digest of the activity log file content.
func FingerprintActivity(logContent []byte) string {
	hash := sha256.Sum256(logContent)
	return hex.EncodeToString(hash[:])
}

// ActivityEntryDetails holds extracted details from an activity log entry for rendering.
type ActivityEntryDetails struct {
	EntryID  int    // The 0-based entry ID
	Command  string // The command that was executed
	ExitCode int    // The exit status
}

// LoadActivityEntries reads an activity log file and returns a map of entry ID to entry details.
// Entry IDs are 0-based. If the file cannot be read or parsed, returns an empty map.
func LoadActivityEntries(logPath string) map[int]ActivityEntryDetails {
	entries, _, err := parseActivityLogFile(logPath)
	if err != nil {
		return make(map[int]ActivityEntryDetails)
	}

	result := make(map[int]ActivityEntryDetails)
	for i, entry := range entries {
		result[i] = ActivityEntryDetails{
			EntryID:  i,
			Command:  entry.Command,
			ExitCode: entry.ExitCode,
		}
	}
	return result
}

// FormatActivityEntryDetails formats activity entry details for rendering.
// Returns a string like "entry 0: command="make build" exit_code=0"
func FormatActivityEntryDetails(details ActivityEntryDetails) string {
	return fmt.Sprintf("entry %d: command=%q exit_code=%d", details.EntryID, details.Command, details.ExitCode)
}
