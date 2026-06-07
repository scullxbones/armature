package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

// OpItem represents a single op loaded from a log file, with metadata about its source.
type OpItem struct {
	Op          Op         // The parsed operation
	LogFilename string     // Full path to the log file
	Source      *FileEntry // Reference to the FileEntry that produced this op
	Offset      int64      // Byte offset where this op ends in the log file
}

// FileEntry represents a log file to be loaded with expected worker ID validation.
type FileEntry struct {
	LogPath          string
	ExpectedWorkerID string
}

// ValidatedOpStream loads operations from multiple log files, validating that
// the worker ID in the op matches the expected worker ID from the filename,
// and returns warnings for any validation failures.
type ValidatedOpStream struct {
	files []*FileEntry
}

// NewValidatedOpStream creates a new stream for loading ops from multiple files.
func NewValidatedOpStream() *ValidatedOpStream {
	return &ValidatedOpStream{
		files: make([]*FileEntry, 0),
	}
}

// AddFile registers a log file to be loaded, with the expected worker ID from the filename.
func (s *ValidatedOpStream) AddFile(logPath, expectedWorkerID string) *FileEntry {
	entry := &FileEntry{
		LogPath:          logPath,
		ExpectedWorkerID: expectedWorkerID,
	}
	s.files = append(s.files, entry)
	return entry
}

// Load reads all registered files and returns a slice of OpItems, warnings, and an error.
// Each OpItem includes the op, its source file, byte offset, and log filename.
// Ops with mismatched worker IDs are excluded and logged as warnings.
// Corrupt lines are skipped and logged as warnings.
func (s *ValidatedOpStream) Load() ([]OpItem, []string, error) {
	var items []OpItem
	var warnings []string

	for _, entry := range s.files {
		fileItems, fileWarnings, err := s.loadFile(entry)
		if err != nil {
			return nil, nil, fmt.Errorf("load file %s: %w", entry.LogPath, err)
		}
		items = append(items, fileItems...)
		warnings = append(warnings, fileWarnings...)
	}

	return items, warnings, nil
}

// loadFile loads ops from a single file, returning items, warnings, and error.
// It validates that each op's WorkerID matches the expected worker ID.
func (s *ValidatedOpStream) loadFile(entry *FileEntry) ([]OpItem, []string, error) {
	var items []OpItem
	var warnings []string

	// Read raw lines from the log file and track their byte offsets
	linesWithOffsets, err := adapters.ReadLogLinesWithOffsets(entry.LogPath, 0)
	if err != nil {
		return nil, nil, err
	}

	for _, lineInfo := range linesWithOffsets {
		// Parse the op
		op, parseErr := ParseLine(lineInfo.Line)
		if parseErr != nil {
			// Skip corrupt lines, but record warning
			warnings = append(warnings, fmt.Sprintf(
				"corrupt line in %s: %v",
				filepath.Base(entry.LogPath), parseErr,
			))
			continue
		}

		// Check worker ID match
		if op.WorkerID != entry.ExpectedWorkerID {
			warnings = append(warnings, fmt.Sprintf(
				"worker ID mismatch in %s: expected %s, got %s (target: %s)",
				filepath.Base(entry.LogPath),
				entry.ExpectedWorkerID,
				op.WorkerID,
				op.TargetID,
			))
			continue
		}

		// Add to items
		items = append(items, OpItem{
			Op:          op,
			LogFilename: entry.LogPath,
			Source:      entry,
			Offset:      lineInfo.EndOffset,
		})
	}

	return items, warnings, nil
}

// LoadFromDirValidated loads all ops from a directory of .log files, validating
// that each op's worker ID matches its filename's worker ID (including slot suffix).
// Returns items, warnings, and error.
// Each op's worker ID is compared against the full filename worker ID (with slot suffix preserved).
// Mismatched ops are excluded, and warnings are returned for them.
func LoadFromDirValidated(opsDir string) ([]OpItem, []string, error) {
	// List all log files in the directory
	logFiles, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		// If directory doesn't exist, return empty
		if logFiles == nil {
			return []OpItem{}, []string{}, nil
		}
		return nil, nil, err
	}

	stream := NewValidatedOpStream()

	// Register each log file with its expected worker ID from the filename
	// Note: we use the full filename-derived worker ID (including slot suffix)
	for _, logPath := range logFiles {
		expectedWorkerID := ExtractWorkerIDFromFilename(logPath)
		stream.AddFile(logPath, expectedWorkerID)
	}

	return stream.Load()
}

// LoadFromDirWithOffsetsValidated loads all ops from a directory of .log files,
// validating worker IDs and returning byte offsets for checkpoint tracking.
// Returns items, a map of log filename -> byte offset (end position), warnings, and error.
func LoadFromDirWithOffsetsValidated(opsDir string) ([]OpItem, map[string]int64, []string, error) {
	items, warnings, err := LoadFromDirValidated(opsDir)
	if err != nil {
		return nil, nil, nil, err
	}

	// Build offsets map from items
	offsets := make(map[string]int64)
	for _, item := range items {
		logName := filepath.Base(item.LogFilename)
		// Store the maximum offset seen for each file
		if item.Offset > offsets[logName] {
			offsets[logName] = item.Offset
		}
	}

	return items, offsets, warnings, nil
}

// ExtractOps converts a slice of OpItems to a slice of Ops for compatibility
// with existing code that expects just ops.
func ExtractOps(items []OpItem) []Op {
	ops := make([]Op, len(items))
	for i, item := range items {
		ops[i] = item.Op
	}
	return ops
}

// ExtractWorkerIDFromFilename extracts the full worker ID from a log filename,
// preserving any slot suffix (the part after ~).
// Examples:
// - "3357fe85.log" -> "3357fe85"
// - "3357fe85~a.log" -> "3357fe85~a"
// - "worker-x~slot-99.log" -> "worker-x~slot-99"
func ExtractWorkerIDFromFilename(logPath string) string {
	base := filepath.Base(logPath)
	name := strings.TrimSuffix(base, ".log")
	return name
}
