package ops

import (
	"github.com/scullxbones/armature/internal/adapters"
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
	return adapters.NewAppendLog(logPath).Append(buf)
}

// ReadLog reads all ops from a log file.
func ReadLog(logPath string) ([]Op, error) {
	return ReadLogFromOffset(logPath, 0)
}

// ReadLogFromOffset reads ops starting from a byte offset.
func ReadLogFromOffset(logPath string, offset int64) ([]Op, error) {
	lines, err := adapters.ReadLogFromOffset(logPath, offset)
	if err != nil {
		return nil, err
	}

	ops := make([]Op, 0, len(lines))
	for _, line := range lines {
		op, err := ParseLine(line)
		if err != nil {
			// Skip corrupt lines per spec — log warning
			continue
		}
		ops = append(ops, op)
	}
	return ops, nil
}
