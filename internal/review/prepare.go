package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitAdapter is an interface for git operations required by the Prepare function.
type GitAdapter interface {
	// ResolveRevision resolves a git revision to its full commit SHA.
	ResolveRevision(rev string) (string, error)
	// DiffRange returns the unified diff between two revisions.
	DiffRange(base, head string) (string, error)
	// DiffNameOnlyRange returns the list of changed file names between two revisions.
	DiffNameOnlyRange(base, head string) ([]string, error)
}

// FilterDiff strips file sections from a unified diff whose paths match any of
// the given excludePrefixes. A "file section" begins at a "diff --git a/..." line
// and extends to the next such line or end of input. The returned string contains
// only the sections whose paths do not match any excluded prefix.
func FilterDiff(diff string, excludePrefixes []string) string {
	if diff == "" || len(excludePrefixes) == 0 {
		return diff
	}

	var out strings.Builder
	var sectionBuf strings.Builder
	excluded := false
	inSection := false

	// SplitAfter preserves the trailing newline on each line.
	lines := strings.SplitAfter(diff, "\n")

	for _, line := range lines {
		bare := strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(bare, "diff --git "):
			// Flush previous section if it was not excluded.
			if inSection && !excluded {
				out.WriteString(sectionBuf.String())
			}
			sectionBuf.Reset()
			sectionBuf.WriteString(line)
			inSection = true

			// Extract the file path from "diff --git a/<path> b/<path>".
			rest := strings.TrimPrefix(bare, "diff --git a/")
			path := rest
			if idx := strings.Index(rest, " b/"); idx >= 0 {
				path = rest[:idx]
			}

			excluded = false
			for _, prefix := range excludePrefixes {
				if strings.HasPrefix(path, prefix) {
					excluded = true
					break
				}
			}
		case inSection:
			sectionBuf.WriteString(line)
		default:
			// Content before the first diff header (rare in practice).
			out.WriteString(line)
		}
	}

	// Flush the last section.
	if inSection && !excluded {
		out.WriteString(sectionBuf.String())
	}

	return out.String()
}

// filterExcludedPaths removes files matching exclusion prefixes from the list.
func filterExcludedPaths(files []string, excludePrefixes []string) []string {
	if len(files) == 0 || len(excludePrefixes) == 0 {
		return files
	}
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		excluded := false
		for _, prefix := range excludePrefixes {
			if strings.HasPrefix(file, prefix) {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// attachActivitySection attempts to read the activity log and create an Activity section.
// If the log does not exist or cannot be parsed, returns nil silently.
// headSHA is the delivery commit SHA, used to count entries at delivery head vs earlier.
func attachActivitySection(activityLogPath, headSHA string) *Activity {
	// Check if the file exists
	if _, err := os.Stat(activityLogPath); err != nil {
		// File does not exist or is inaccessible — silently return nil
		return nil
	}

	// Parse the activity log
	entries, logContent, err := parseActivityLogFile(activityLogPath)
	if err != nil {
		// Parsing failed — silently return nil (activity is optional)
		return nil
	}

	// Count entries at delivery head vs earlier
	deliveryHeadCount := 0
	earlierCount := 0
	for _, entry := range entries {
		if entry.HeadSHA == headSHA {
			deliveryHeadCount++
		} else {
			earlierCount++
		}
	}

	// Compute activity digest
	digest := FingerprintActivity(logContent)

	// Store an absolute path so ValidateActivityDigest (record time, potentially
	// a different working directory) resolves the same file that was hashed
	// here, rather than a path relative to whatever directory happened to be
	// current during prepare (m3).
	logAbsPath := activityLogPath
	if abs, err := filepath.Abs(activityLogPath); err == nil {
		logAbsPath = abs
	}

	return &Activity{
		Digest:            digest,
		EntryCount:        len(entries),
		DeliveryHeadCount: deliveryHeadCount,
		EarlierCount:      earlierCount,
		LogPath:           logAbsPath,
	}
}

// Prepare builds a ReviewBundle for an issue given its contract metadata and a git range.
// It resolves the git revisions, computes the diff and changed files, and constructs
// a complete ReviewBundle with fingerprints and bundle ID.
//
// issueType and issueOutcome are the issue's type (e.g. "task") and recorded outcome.
// definitionOfDone is the primary contract criterion; scope is the file/area scope list.
// activityLogPath is an optional path to the worktree's activity log file. If provided and
// the file exists, an Activity section is attached to the bundle with digest, entry count,
// and HEAD-anchored summary. If empty or the file does not exist, the Activity section is omitted.
func Prepare(
	git GitAdapter,
	issueID, title, definitionOfDone, issueType, issueOutcome string,
	scope []string, criteria []string,
	base, head, activityLogPath string,
) (*ReviewBundle, error) {
	// Resolve both revisions to full SHAs
	baseSHA, err := git.ResolveRevision(base)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base revision %s: %w", base, err)
	}

	headSHA, err := git.ResolveRevision(head)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve head revision %s: %w", head, err)
	}

	// Get the unified diff
	diff, err := git.DiffRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to compute diff: %w", err)
	}

	// Get the list of changed files
	changedFiles, err := git.DiffNameOnlyRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	// Filter out armature coordination paths (.armature/** and .arm/**)
	excludePrefixes := []string{".armature/", ".arm/"}
	changedFiles = filterExcludedPaths(changedFiles, excludePrefixes)

	// A delivery with no changed files is not reviewable — reject it regardless of
	// whether the empty result came from filtering or from the git range itself.
	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("delivery contains no changed files")
	}

	// Strip coordination-path sections from the unified diff so the reviewer
	// never sees coordination hunks and DeliveryFingerprint stays stable across
	// coordination-path churn.
	filteredDiff := FilterDiff(diff, excludePrefixes)

	// Build the contract
	contract := Contract{
		DefinitionOfDone: definitionOfDone,
		Scope:            scope,
		Acceptance:       criteria,
	}

	// Build the delivery
	delivery := Delivery{
		BaseSHA:      baseSHA,
		HeadSHA:      headSHA,
		ChangedFiles: changedFiles,
		Diff:         filteredDiff,
	}

	// Compute fingerprints
	contractFP := FingerprintContract(contract)
	deliveryFP := FingerprintDelivery(delivery)

	// Build the bundle (without bundleID initially)
	bundle := &ReviewBundle{
		SchemaVersion: SchemaVersion,
		Issue: IssueInfo{
			ID:      issueID,
			Type:    issueType,
			Title:   title,
			Outcome: issueOutcome,
		},
		Contract: contract,
		Delivery: delivery,
		Fingerprints: Fingerprints{
			Contract: contractFP,
			Delivery: deliveryFP,
		},
	}

	// Attempt to attach activity section if activity log path is provided
	if activityLogPath != "" {
		activity := attachActivitySection(activityLogPath, headSHA)
		if activity != nil {
			bundle.Activity = activity
		}
		// If activity log does not exist or cannot be parsed, silently continue
		// (activity is optional per the spec)
	}

	// Compute and set the bundle ID
	bundle.BundleID = ComputeBundleID(*bundle)

	// Validate the bundle
	if err := bundle.Valid(); err != nil {
		return nil, fmt.Errorf("invalid review bundle: %w", err)
	}

	return bundle, nil
}
