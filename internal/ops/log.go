package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppendOp appends a single op to the log file as a JSONL line.
func AppendOp(logPath string, op Op) error {
	return AppendOps(logPath, []Op{op})
}

// AppendOps appends multiple ops atomically in a single file write.
func AppendOps(logPath string, ops []Op) error {
	var buf []byte
	for _, op := range ops {
		line, err := MarshalOp(op)
		if err != nil {
			return err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("write to log %s: %w", logPath, err)
	}
	return nil
}

// ReadLog reads all ops from a log file.
func ReadLog(logPath string) ([]Op, error) {
	return ReadLogFromOffset(logPath, 0)
}

// ReadLogFromOffset reads ops starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) ([]Op, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer func() { _ = f.Close() }()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, fmt.Errorf("seek in log %s: %w", logPath, err)
		}
	}

	ops := make([]Op, 0, 64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		op, err := ParseLine(line)
		if err != nil {
			// Skip corrupt lines per spec — log warning
			continue
		}
		ops = append(ops, op)
	}
	return ops, scanner.Err()
}

// ReadLogValidated reads ops and filters out those with mismatched worker IDs.
func ReadLogValidated(logPath string, expectedWorkerID string) ([]Op, error) {
	all, err := ReadLog(logPath)
	if err != nil {
		return nil, err
	}
	var valid []Op
	for _, op := range all {
		if op.WorkerID == expectedWorkerID {
			valid = append(valid, op)
		}
	}
	return valid, nil
}

// WorkerIDFromFilename extracts the worker ID from a log filename.
// Plain log:   "3357fe85.log"   -> "3357fe85"
// Slotted log: "3357fe85~a.log" -> "3357fe85"  (slot suffix stripped)
func WorkerIDFromFilename(logPath string) string {
	base := filepath.Base(logPath)
	name := strings.TrimSuffix(base, ".log")
	if idx := strings.Index(name, "~"); idx >= 0 {
		name = name[:idx]
	}
	return name
}
