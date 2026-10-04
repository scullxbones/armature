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
	Source      *FileEntry
	Offset      int64
	LineNumber  int
}

type FileEntry struct {
	LogPath          string
	ExpectedWorkerID string
}

type ValidatedOpStream struct {
	files []*FileEntry
}

type LoadResult struct {
	Items       []OpItem
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

	linesWithOffsets, err := adapters.ReadLogLinesWithOffsets(entry.LogPath, 0)
	if err != nil {
		return nil, 0, nil, err
	}

	for i, lineInfo := range linesWithOffsets {
		physicalEOF = lineInfo.EndOffset

		op, parseErr := ParseLine(lineInfo.Line)
		if parseErr != nil {
			if IsNewerSchemaVersion(parseErr) {
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
			Source:      entry,
			Offset:      lineInfo.EndOffset,
			LineNumber:  i + 1,
		})
	}

	return items, physicalEOF, warnings, nil
}

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

// WorkerOwnsLog reports whether workerID may appear in the named log.
// The name is `<workerID>.log` or `<workerID>~<slot>.log` (a path is ok).
// The WorkerID on each line must match the log name exactly, including any slot.
func WorkerOwnsLog(workerID, logPathOrName string) bool {
	expected := strings.TrimSuffix(filepath.Base(logPathOrName), ".log")
	return workerID == expected
}

func ExtractOps(items []OpItem) []Op {
	ops := make([]Op, len(items))
	for i, item := range items {
		ops[i] = item.Op
	}
	return ops
}
