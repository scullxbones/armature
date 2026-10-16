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
	type scanned struct {
		entry       *fileEntry
		lines       []physicalLine
		physicalEOF int64
	}
	scans := make([]scanned, 0, len(s.files))
	quarantined := make(map[string]map[int]struct{})

	for _, entry := range s.files {
		lines, eof, err := readPhysicalLog(entry.LogPath)
		if err != nil {
			return loadResult{}, fmt.Errorf("load file %s: %w", entry.LogPath, err)
		}
		scans = append(scans, scanned{entry: entry, lines: lines, physicalEOF: eof})
		for _, rec := range lines {
			if rec.Incomplete || len(rec.Raw) == 0 {
				continue
			}
			op, err := parseLine(rec.Raw).value()
			if err != nil {
				continue
			}
			if op.Type != OpLogQuarantine {
				continue
			}
			logName := op.Payload.QuarantineLog
			if logName == "" {
				logName = op.TargetID
			}
			if quarantined[logName] == nil {
				quarantined[logName] = make(map[int]struct{})
			}
			quarantined[logName][op.Payload.QuarantineLine] = struct{}{}
		}
	}

	result := loadResult{
		PhysicalEOF: make(map[string]int64),
	}
	for _, scan := range scans {
		fileItems, fileWarnings, err := foldScannedFile(scan.entry, scan.lines, quarantined)
		if err != nil {
			return loadResult{}, err
		}
		result.Items = append(result.Items, fileItems...)
		result.Warnings = append(result.Warnings, fileWarnings...)
		result.PhysicalEOF[filepath.Base(scan.entry.LogPath)] = scan.physicalEOF
	}
	return result, nil
}

func foldScannedFile(entry *fileEntry, lines []physicalLine, quarantined map[string]map[int]struct{}) ([]OpItem, []string, error) {
	var items []OpItem
	var warnings []string
	base := filepath.Base(entry.LogPath)

	for _, rec := range lines {
		if len(rec.Raw) == 0 {
			continue
		}
		if rec.Incomplete {
			warnings = append(warnings, fmt.Sprintf(
				"ignoring torn final line in %s:%d (no trailing newline)",
				base, rec.Number,
			))
			continue
		}
		op, parseErr := parseLine(rec.Raw).value()
		if parseErr != nil {
			if isNewerSchemaVersion(parseErr) {
				return nil, nil, locateParseError(entry.LogPath, rec.Number, parseErr)
			}
			if _, skip := quarantined[base][rec.Number]; skip {
				continue
			}
			return nil, nil, locateParseError(entry.LogPath, rec.Number, parseErr)
		}
		if !WorkerOwnsLog(op.WorkerID, entry.ExpectedWorkerID) {
			warnings = append(warnings, fmt.Sprintf(
				"worker ID mismatch in %s: expected %s, got %s (target: %s)",
				base,
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
			Offset:      rec.EndOffset,
			LineNumber:  rec.Number,
		})
	}
	return items, warnings, nil
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
