package review

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// activityScannerBufferSize is the initial buffer size handed to bufio.Scanner
// when reading the activity log. bufio.Scanner's default 64KB token limit is
// smaller than a single worst-case activity line (unbounded command up to
// maxCommandSize plus ~2KB of truncated output plus JSON overhead), so a single
// oversized line would otherwise fail the entire scan and silently drop the
// whole activity section (M9). 1MB comfortably covers the writer's cap.
const activityScannerBufferSize = 1 << 20

// activityScannerMaxTokenSize is the hard ceiling passed to scanner.Buffer,
// bounding how large a single line is allowed to grow before scanning fails.
const activityScannerMaxTokenSize = 4 << 20

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
// Note: LogPath is excluded from Activity when hashing because it is worktree-local
// and differs across clones/worktrees even when the activity facts are identical.
func ComputeBundleID(bundle ReviewBundle) string {
	// Serialize the bundle (without bundleID itself) to canonical JSON
	// We hash the core review data

	// Prepare a normalized Activity without LogPath for hashing
	var activityForHash *struct {
		Digest            string
		EntryCount        int
		DeliveryHeadCount int
		EarlierCount      int
	}
	if bundle.Activity != nil {
		activityForHash = &struct {
			Digest            string
			EntryCount        int
			DeliveryHeadCount int
			EarlierCount      int
		}{
			Digest:            bundle.Activity.Digest,
			EntryCount:        bundle.Activity.EntryCount,
			DeliveryHeadCount: bundle.Activity.DeliveryHeadCount,
			EarlierCount:      bundle.Activity.EarlierCount,
		}
	}

	data := struct {
		SchemaVersion int
		Issue         IssueInfo
		Contract      Contract
		Delivery      Delivery
		Activity      *struct {
			Digest            string
			EntryCount        int
			DeliveryHeadCount int
			EarlierCount      int
		}
	}{
		SchemaVersion: bundle.SchemaVersion,
		Issue:         bundle.Issue,
		Contract:      bundle.Contract,
		Delivery:      bundle.Delivery,
		Activity:      activityForHash,
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

// ActivityLogEntry represents a parsed line from the activity log (JSONL format:
// one JSON object per line, written by internal/harnesshook.AppendActivity).
type ActivityLogEntry struct {
	Timestamp     string
	Command       string
	ExitCode      int
	ExitCodeKnown bool
	HeadSHA       string
	OutputHash    string
	OutputHead    string
	OutputTail    string
}

// activityLogLine mirrors the JSON shape written by
// internal/harnesshook.activityLogLine. Kept as a separate type (rather than a
// shared import) since harnesshook and review intentionally don't depend on
// each other; the two must be kept in sync (a round-trip test enforces this).
type activityLogLine struct {
	Timestamp     string `json:"timestamp"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	ExitCodeKnown bool   `json:"exit_code_known"`
	HeadSHA       string `json:"head_sha"`
	OutputHash    string `json:"output_hash"`
	OutputHead    string `json:"output_head"`
	OutputTail    string `json:"output_tail"`
}

// parseActivityLogFile reads the activity log file (JSONL: one JSON object per
// line) and returns parsed entries keyed by physical line number (0-based),
// plus the raw file content for digest computation.
//
// Entry IDs are the 0-based physical line number, not a sequential count of
// successfully parsed entries: a malformed or blank line consumes its line
// number but produces no entry, so IDs never shift when such lines are
// skipped (m1). Malformed lines are logged nowhere and simply skipped —
// the digest already pins the exact file content, so a parse failure on one
// line does not need to fail the whole bundle preparation.
func parseActivityLogFile(logPath string) (map[int]ActivityLogEntry, []byte, error) {
	content, err := os.ReadFile(logPath) //nolint:gosec // G304: logPath is provided by Prepare
	if err != nil {
		return nil, nil, fmt.Errorf("read activity log: %w", err)
	}

	entries := make(map[int]ActivityLogEntry)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 0, activityScannerBufferSize), activityScannerMaxTokenSize)

	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		id := lineNum
		lineNum++

		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw activityLogLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			// Malformed line: skip, but the line number is still consumed above.
			continue
		}

		entries[id] = ActivityLogEntry(raw)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan activity log: %w", err)
	}

	return entries, content, nil
}

// FingerprintActivity computes a SHA-256 digest of the activity log file content.
func FingerprintActivity(logContent []byte) string {
	hash := sha256.Sum256(logContent)
	return hex.EncodeToString(hash[:])
}

// ActivityEntryDetails holds extracted details from an activity log entry for rendering.
type ActivityEntryDetails struct {
	EntryID       int    // The 0-based physical-line entry ID
	Command       string // The command that was executed
	ExitCode      int    // The exit status (only meaningful when ExitCodeKnown is true)
	ExitCodeKnown bool   // Whether the harness reported an exit code for this entry
}

// LoadActivityEntries reads an activity log file and returns a map of entry ID to entry details.
// Entry IDs are the 0-based physical line number in the log file (see parseActivityLogFile).
// If the file cannot be read, returns an empty map.
func LoadActivityEntries(logPath string) map[int]ActivityEntryDetails {
	entries, _, err := parseActivityLogFile(logPath)
	if err != nil {
		return make(map[int]ActivityEntryDetails)
	}

	result := make(map[int]ActivityEntryDetails)
	for id, entry := range entries {
		result[id] = ActivityEntryDetails{
			EntryID:       id,
			Command:       entry.Command,
			ExitCode:      entry.ExitCode,
			ExitCodeKnown: entry.ExitCodeKnown,
		}
	}
	return result
}

// FormatActivityEntryDetails formats activity entry details for rendering.
// Returns a string like "entry 0: command="make build" exit_code=0", or
// "entry 0: command="make build" exit_code=unknown" when the harness did not
// report an exit code for this entry.
func FormatActivityEntryDetails(details ActivityEntryDetails) string {
	if !details.ExitCodeKnown {
		return fmt.Sprintf("entry %d: command=%q exit_code=unknown", details.EntryID, details.Command)
	}
	return fmt.Sprintf("entry %d: command=%q exit_code=%d", details.EntryID, details.Command, details.ExitCode)
}
