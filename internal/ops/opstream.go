package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

type OpItem struct {
	Op          Op
	LogFilename string
	source      *fileEntry
	Offset      int64
	LineNumber  int
}

type fileEntry struct {
	LogPath          string
	ExpectedWorkerID string
}

type validatedOpStream struct {
	files []*fileEntry
}

type loadResult struct {
	Items       []OpItem
	PhysicalEOF map[string]int64
	Warnings    []string
}

func newValidatedOpStream() *validatedOpStream {
	return &validatedOpStream{
		files: make([]*fileEntry, 0),
	}
}

func (s *validatedOpStream) addFile(logPath, expectedWorkerID string) *fileEntry {
	entry := &fileEntry{
		LogPath:          logPath,
		ExpectedWorkerID: expectedWorkerID,
	}
	s.files = append(s.files, entry)
	return entry
}

func (s *validatedOpStream) loadAll() (loadResult, error) {
	result := loadResult{
		PhysicalEOF: make(map[string]int64),
	}

	for _, entry := range s.files {
		fileItems, physicalEOF, fileWarnings, err := s.loadFile(entry)
		if err != nil {
			return loadResult{}, fmt.Errorf("load file %s: %w", entry.LogPath, err)
		}
		result.Items = append(result.Items, fileItems...)
		result.Warnings = append(result.Warnings, fileWarnings...)
		result.PhysicalEOF[filepath.Base(entry.LogPath)] = physicalEOF
	}

	return result, nil
}

func (s *validatedOpStream) loadFile(entry *fileEntry) ([]OpItem, int64, []string, error) {
	var items []OpItem
	var warnings []string
	var physicalEOF int64

	linesWithOffsets, err := adapters.ReadLogLinesWithOffsets(entry.LogPath, 0)
	if err != nil {
		return nil, 0, nil, err
	}

	for i, lineInfo := range linesWithOffsets {
		physicalEOF = lineInfo.EndOffset

		op, parseErr := ParseLine(lineInfo.Line)
		if parseErr != nil {
			if isNewerSchemaVersion(parseErr) {
				return nil, 0, nil, parseErr
			}
			warnings = append(warnings, fmt.Sprintf(
				"corrupt line in %s: %v",
				filepath.Base(entry.LogPath), parseErr,
			))
			continue
		}

		if !WorkerOwnsLog(op.WorkerID, entry.ExpectedWorkerID) {
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
			source:      entry,
			Offset:      lineInfo.EndOffset,
			LineNumber:  i + 1,
		})
	}

	return items, physicalEOF, warnings, nil
}

func LoadFromDirValidated(opsDir string) (loadResult, error) {
	empty := loadResult{
		Items:       []OpItem{},
		PhysicalEOF: map[string]int64{},
		Warnings:    []string{},
	}
	logFiles, err := adapters.ListLogFiles(opsDir)
	if err != nil {
		return loadResult{}, err
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

// WorkerOwnsLog reports whether workerID may appear in the named log.
// The name is `<workerID>.log` or `<workerID>~<slot>.log` (a path is ok).
//
// Exact match of the log stem is always accepted. Slotted logs also accept the
// unslotted base worker ID (`<base>` for `<base>~<slot>.log`): those lines are
// a permanent read grandfather for append-only history (Constitution I2) written
// before writers stamped the full slotted stem. New writes still emit the exact
// slotted ID via worker.ResolveIdentity; this carve-out is load-only.
func WorkerOwnsLog(workerID, logPathOrName string) bool {
	expected := strings.TrimSuffix(filepath.Base(logPathOrName), ".log")
	if workerID == expected {
		return true
	}
	base, _, found := strings.Cut(expected, "~")
	return found && workerID == base
}

func ExtractOps(items []OpItem) []Op {
	ops := make([]Op, len(items))
	for i, item := range items {
		ops[i] = item.Op
	}
	return ops
}
