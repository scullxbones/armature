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
	LineNumber  int        // Physical line number in the source log file (1-indexed)
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

// LoadResult is one validated load of worker log files.
type LoadResult struct {
	Items []OpItem
	// PhysicalEOF maps each log basename to the byte offset of the last line
	// observed (accepted, rejected, or corrupt), or 0 if the file is empty.
	PhysicalEOF map[string]int64
	Warnings    []string
}

func newValidatedOpStream() *ValidatedOpStream {
	return &ValidatedOpStream{
		files: make([]*FileEntry, 0),
	}
}

func (s *ValidatedOpStream) addFile(logPath, expectedWorkerID string) *FileEntry {
	entry := &FileEntry{
		LogPath:          logPath,
		ExpectedWorkerID: expectedWorkerID,
	}
	s.files = append(s.files, entry)
	return entry
}

func (s *ValidatedOpStream) loadAll() (LoadResult, error) {
	result := LoadResult{
		PhysicalEOF: make(map[string]int64),
	}

	for _, entry := range s.files {
		fileItems, physicalEOF, fileWarnings, err := s.loadFile(entry)
		if err != nil {
			return LoadResult{}, fmt.Errorf("load file %s: %w", entry.LogPath, err)
		}
		result.Items = append(result.Items, fileItems...)
		result.Warnings = append(result.Warnings, fileWarnings...)
		result.PhysicalEOF[filepath.Base(entry.LogPath)] = physicalEOF
	}

	return result, nil
}

func (s *ValidatedOpStream) loadFile(entry *FileEntry) ([]OpItem, int64, []string, error) {
	var items []OpItem
	var warnings []string
	var physicalEOF int64

	legacyWorkerID := entry.ExpectedWorkerID
	if i := strings.Index(entry.ExpectedWorkerID, "~"); i >= 0 {
		legacyWorkerID = entry.ExpectedWorkerID[:i]
	}

	linesWithOffsets, err := adapters.ReadLogLinesWithOffsets(entry.LogPath, 0)
	if err != nil {
		return nil, 0, nil, err
	}

	for i, lineInfo := range linesWithOffsets {
		physicalEOF = lineInfo.EndOffset

		op, parseErr := ParseLine(lineInfo.Line)
		if parseErr != nil {
			warnings = append(warnings, fmt.Sprintf(
				"corrupt line in %s: %v",
				filepath.Base(entry.LogPath), parseErr,
			))
			continue
		}

		if op.WorkerID != entry.ExpectedWorkerID && op.WorkerID != legacyWorkerID {
			warnings = append(warnings, fmt.Sprintf(
				"worker ID mismatch in %s: expected %s, got %s (target: %s)",
				filepath.Base(entry.LogPath),
				entry.ExpectedWorkerID,
				op.WorkerID,
				op.TargetID,
			))
			continue
		}

		items = append(items, OpItem{
			Op:          op,
			LogFilename: entry.LogPath,
			Source:      entry,
			Offset:      lineInfo.EndOffset,
			LineNumber:  i + 1,
		})
	}

	return items, physicalEOF, warnings, nil
}

// LoadFromDirValidated loads every .log file under opsDir with worker-ID checks.
func LoadFromDirValidated(opsDir string) (LoadResult, error) {
	empty := LoadResult{
		Items:       []OpItem{},
		PhysicalEOF: map[string]int64{},
		Warnings:    []string{},
	}
	logFiles, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		return LoadResult{}, err
	}
	if logFiles == nil {
		return empty, nil
	}

	stream := newValidatedOpStream()
	for _, logPath := range logFiles {
		stream.addFile(logPath, strings.TrimSuffix(filepath.Base(logPath), ".log"))
	}
	return stream.loadAll()
}

// LoadFromDirWithOffsetsValidated loads all ops from a directory of .log files,
// validating worker IDs and returning byte offsets for checkpoint tracking.
// Returns items, a map of log filename -> byte offset (end position), warnings, and error.
// Checkpoint offset for every file must equal its physical EOF after each load.
func LoadFromDirWithOffsetsValidated(opsDir string) ([]OpItem, map[string]int64, []string, error) {
	result, err := LoadFromDirValidated(opsDir)
	if err != nil {
		return nil, nil, nil, err
	}
	return result.Items, result.PhysicalEOF, result.Warnings, nil
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
