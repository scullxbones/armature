package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/config"
)

type GitAdapter interface {
	ResolveRevision(rev string) (string, error)
	DiffRange(base, head string) (string, error)
	DiffNameOnlyRange(base, head string) ([]string, error)
}

func FilterDiff(diff string, excludePrefixes []string) string {
	if diff == "" || len(excludePrefixes) == 0 {
		return diff
	}

	var out strings.Builder
	var sectionBuf strings.Builder
	excluded := false
	inSection := false

	lines := strings.SplitAfter(diff, "\n")

	for _, line := range lines {
		bare := strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(bare, "diff --git "):
			if inSection && !excluded {
				out.WriteString(sectionBuf.String())
			}
			sectionBuf.Reset()
			sectionBuf.WriteString(line)
			inSection = true

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
			out.WriteString(line)
		}
	}

	if inSection && !excluded {
		out.WriteString(sectionBuf.String())
	}

	return out.String()
}

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

func attachActivitySection(activityLogPath, headSHA string) (*Activity, error) {
	if _, err := os.Stat(activityLogPath); err != nil {
		return nil, nil
	}

	entries, logContent, err := parseActivityLogFile(activityLogPath)
	if err != nil {
		return nil, err
	}

	deliveryHeadCount := 0
	earlierCount := 0
	for _, entry := range entries {
		if entry.HeadSHA == headSHA {
			deliveryHeadCount++
		} else {
			earlierCount++
		}
	}

	digest := FingerprintActivity(logContent)

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
	}, nil
}

func Prepare(
	git GitAdapter,
	issueID, title, definitionOfDone, issueType, issueOutcome string,
	scope []string, criteria []string,
	base, head, activityLogPath string,
) (*ReviewBundle, error) {
	baseSHA, err := git.ResolveRevision(base)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base revision %s: %w", base, err)
	}

	headSHA, err := git.ResolveRevision(head)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve head revision %s: %w", head, err)
	}

	diff, err := git.DiffRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to compute diff: %w", err)
	}

	changedFiles, err := git.DiffNameOnlyRange(baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	excludePrefixes := []string{config.StateDirName + "/", ".arm/"}
	changedFiles = filterExcludedPaths(changedFiles, excludePrefixes)

	if len(changedFiles) == 0 {
		return nil, fmt.Errorf("delivery contains no changed files")
	}

	filteredDiff := FilterDiff(diff, excludePrefixes)

	contract := Contract{
		DefinitionOfDone: definitionOfDone,
		Scope:            scope,
		Acceptance:       criteria,
	}

	delivery := Delivery{
		BaseSHA:      baseSHA,
		HeadSHA:      headSHA,
		ChangedFiles: changedFiles,
		Diff:         filteredDiff,
	}

	contractFP := FingerprintContract(contract)
	deliveryFP := FingerprintDelivery(delivery)

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

	if activityLogPath != "" {
		activity, err := attachActivitySection(activityLogPath, headSHA)
		if err != nil {
			return nil, fmt.Errorf("parse activity log: %w", err)
		}
		if activity != nil {
			bundle.Activity = activity
		}
	}

	id, err := ComputeBundleID(*bundle)
	if err != nil {
		return nil, err
	}
	bundle.BundleID = id

	if err := bundle.Valid(); err != nil {
		return nil, fmt.Errorf("invalid review bundle: %w", err)
	}

	return bundle, nil
}
