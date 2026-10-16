package ops

import (
	"github.com/scullxbones/armature/internal/adapters"
)

// AppendOp appends a single op to the log file as a JSONL line.
func AppendOp(logPath string, op Op) error {
	_, err := appendOpIf(logPath, op, nil)
	return err
}

func appendOpIf(logPath string, op Op, proceed func() (bool, error)) (bool, error) {
	line, err := MarshalOp(op)
	if err != nil {
		return false, err
	}
	return adapters.NewAppendLog(logPath).AppendIf(append(line, '\n'), proceed)
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

	if len(lines) > 0 {
		complete, err := fileEndsWithNewline(logPath)
		if err != nil {
			return nil, err
		}
		if !complete {
			lines = lines[:len(lines)-1]
		}
	}

	ops := make([]Op, 0, len(lines))
	for i, line := range lines {
		op, err := parseLine(line).value()
		if err != nil {
			return nil, locateParseError(logPath, i+1, err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}
