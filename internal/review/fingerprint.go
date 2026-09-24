package review

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scullxbones/armature/internal/ops"
)

// activityScannerBufferSize is the initial buffer size handed to bufio.Scanner
// when reading the activity log. bufio.Scanner's default 64KB token limit is
// smaller than a single worst-case activity line (unbounded command up to
// maxCommandSize plus ~2KB of truncated output plus JSON overhead), so a single
// oversized line would otherwise fail the entire scan and silently drop the
// whole activity section (M9). 1MB comfortably covers the writer's cap.
const activityScannerBufferSize = 1 << 20

const activityScannerMaxTokenSize = 4 << 20

func fingerprintJSON(v any, what string) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %s: %v", what, err))
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func FingerprintContract(contract Contract) string {
	return fingerprintJSON(contract, "contract")
}

func FingerprintDelivery(delivery Delivery) string {
	return fingerprintJSON(delivery, "delivery")
}

func FingerprintResult(assessment ConformanceAssessment) string {
	return fingerprintJSON(assessment, "assessment")
}

// BundleIDError is returned when ComputeBundleID cannot hash a bundle.
// Callers treat it as a normal validation/prepare failure, not a crash.
type BundleIDError struct {
	Err error
}

func (e *BundleIDError) Error() string {
	if e == nil {
		return "compute bundle id"
	}
	if e.Err == nil {
		return "compute bundle id"
	}
	return fmt.Sprintf("compute bundle id: %v", e.Err)
}

func (e *BundleIDError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func bundleIDFromPayload(data any) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", &BundleIDError{Err: fmt.Errorf("marshal bundle data: %w", err)}
	}
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(hash[:])), nil
}

func ComputeBundleID(bundle ReviewBundle) (string, error) {
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
		GateEvidence []ops.GateEvidence
	}{
		SchemaVersion: bundle.SchemaVersion,
		Issue:         bundle.Issue,
		Contract:      bundle.Contract,
		Delivery:      bundle.Delivery,
		Activity:      activityForHash,
		GateEvidence:  bundle.GateEvidence,
	}

	return bundleIDFromPayload(data)
}

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

func parseActivityLogFile(logPath string) (map[int]ActivityLogEntry, []byte, error) {
	content, err := os.ReadFile(logPath) //nolint:gosec // G304: logPath is provided by Prepare
	if err != nil {
		return nil, nil, fmt.Errorf("read activity log: %w", err)
	}

	entries, err := parseActivityLogBytes(content)
	if err != nil {
		return nil, nil, err
	}

	return entries, content, nil
}

func parseActivityLogBytes(content []byte) (map[int]ActivityLogEntry, error) {
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
			continue
		}

		entries[id] = ActivityLogEntry(raw)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan activity log: %w", err)
	}

	return entries, nil
}

func formatActivityDigestMismatch(logPath, recordedDigest, actualDigest string) string {
	return fmt.Sprintf(
		"activity log digest mismatch: bundle recorded %s but log at %q now has digest %s "+
			"(the log changed since `arm review prepare` ran — this may be a new entry appended by "+
			"further worktree activity, a stale/rotated log, or a tampered file; re-run prepare "+
			"against the current log before recording)",
		recordedDigest, logPath, actualDigest)
}

func formatActivityLogUnreadable(logPath string, err error) string {
	return fmt.Sprintf(
		"activity log missing or unreadable at %q: %v (log must be present and unmodified since prepare)",
		logPath, err)
}

func FingerprintActivity(logContent []byte) string {
	hash := sha256.Sum256(logContent)
	return hex.EncodeToString(hash[:])
}

type ActivityEntryDetails struct {
	EntryID       int
	Command       string
	ExitCode      int
	ExitCodeKnown bool
	HeadSHA       string
}

func ValidateActivityDigestAndLoadEntries(activity *Activity) (map[int]ActivityEntryDetails, []string) {
	var errs []string

	if activity == nil {
		return make(map[int]ActivityEntryDetails), errs
	}

	content, err := os.ReadFile(activity.LogPath)
	if err != nil {
		errs = append(errs, formatActivityLogUnreadable(activity.LogPath, err))
		return make(map[int]ActivityEntryDetails), errs
	}

	actualDigest := FingerprintActivity(content)
	if actualDigest != activity.Digest {
		errs = append(errs, formatActivityDigestMismatch(activity.LogPath, activity.Digest, actualDigest))
	}

	entries, err := parseActivityLogBytes(content)
	if err != nil {
		errs = append(errs, err.Error())
	}

	result := make(map[int]ActivityEntryDetails)
	for id, entry := range entries {
		result[id] = ActivityEntryDetails{
			EntryID:       id,
			Command:       entry.Command,
			ExitCode:      entry.ExitCode,
			ExitCodeKnown: entry.ExitCodeKnown,
			HeadSHA:       entry.HeadSHA,
		}
	}

	return result, errs
}

func FormatActivityEntryDetails(details ActivityEntryDetails) string {
	if !details.ExitCodeKnown {
		return fmt.Sprintf("entry %d: command=%q exit_code=unknown", details.EntryID, details.Command)
	}
	return fmt.Sprintf("entry %d: command=%q exit_code=%d", details.EntryID, details.Command, details.ExitCode)
}
