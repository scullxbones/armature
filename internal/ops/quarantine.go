package ops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

const quarantineSuffix = ".quarantine"

func quarantinePath(logPath string) string {
	return logPath + quarantineSuffix
}

// QuarantineLogLine copies the physical line at lineNumber into a sidecar
// next to the log and appends a log-quarantine op. It never rewrites or
// deletes the original line (I2). Re-running on the same line is a no-op.
func QuarantineLogLine(logPath string, lineNumber int, ts int64) error {
	if lineNumber < 1 {
		return fmt.Errorf("quarantine line must be >= 1, got %d", lineNumber)
	}
	lines, _, err := readPhysicalLog(logPath)
	if err != nil {
		return err
	}
	if lineNumber > len(lines) {
		return fmt.Errorf("quarantine line %d is past end of %s (%d lines)", lineNumber, logPath, len(lines))
	}
	rec := lines[lineNumber-1]
	if rec.Incomplete {
		return fmt.Errorf("line %d of %s is a torn final line; load already ignores it", lineNumber, logPath)
	}
	parseErr := parseLine(rec.Raw).err
	if parseErr == nil {
		if hasQuarantineOp(lines, logPath, lineNumber) {
			return ensureQuarantineSidecar(logPath, rec.Raw)
		}
		return fmt.Errorf("line %d of %s parses as a valid op; refusing to quarantine", lineNumber, logPath)
	}
	if isNewerSchemaVersion(parseErr) {
		return parseErr
	}
	if err := ensureQuarantineSidecar(logPath, rec.Raw); err != nil {
		return err
	}
	if hasQuarantineOp(lines, logPath, lineNumber) {
		return nil
	}
	base := filepath.Base(logPath)
	workerID := strings.TrimSuffix(base, ".log")
	sum := sha256.Sum256(rec.Raw)
	op := Op{
		Type:      OpLogQuarantine,
		TargetID:  base,
		Timestamp: ts,
		WorkerID:  workerID,
		Payload: Payload{
			QuarantineLog:  base,
			QuarantineLine: lineNumber,
			SHA:            hex.EncodeToString(sum[:]),
		},
	}
	return AppendOp(logPath, op)
}

func hasQuarantineOp(lines []physicalLine, logPath string, lineNumber int) bool {
	base := filepath.Base(logPath)
	for _, rec := range lines {
		if rec.Incomplete || len(rec.Raw) == 0 {
			continue
		}
		op, err := parseLine(rec.Raw).value()
		if err != nil {
			continue
		}
		if op.Type == OpLogQuarantine && op.Payload.QuarantineLog == base && op.Payload.QuarantineLine == lineNumber {
			return true
		}
	}
	return false
}

func ensureQuarantineSidecar(logPath string, line []byte) error {
	path := quarantinePath(logPath)
	if existing, err := os.ReadFile(path); err == nil { //nolint:gosec // sidecar beside the ops log
		for _, got := range splitPhysicalLog(existing) {
			if bytes.Equal(got.Raw, line) {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	buf := append(append([]byte{}, line...), '\n')
	return adapters.NewAppendLog(path).Append(buf)
}
