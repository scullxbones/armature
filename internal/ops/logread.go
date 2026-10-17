package ops

import (
	"bytes"
	"fmt"
	"os"
)

type physicalLine struct {
	Raw        []byte
	Number     int
	EndOffset  int64
	Incomplete bool
}

func readPhysicalLog(path string) ([]physicalLine, int64, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, 0, fmt.Errorf("open log %s: %w", path, err)
	}
	return splitPhysicalLog(data), int64(len(data)), nil
}

func splitPhysicalLog(data []byte) []physicalLine {
	if len(data) == 0 {
		return nil
	}
	incomplete := data[len(data)-1] != '\n'
	payload := data
	if !incomplete {
		payload = data[:len(data)-1]
	}
	parts := bytes.Split(payload, []byte{'\n'})
	out := make([]physicalLine, 0, len(parts))
	var offset int64
	last := len(parts) - 1
	for i, part := range parts {
		part = bytes.TrimSuffix(part, []byte{'\r'})
		offset += int64(len(part))
		if i < last || !incomplete {
			offset++
		}
		out = append(out, physicalLine{
			Raw:        part,
			Number:     i + 1,
			EndOffset:  offset,
			Incomplete: incomplete && i == last,
		})
	}
	return out
}

func fileEndsWithNewline(path string) (ok bool, err error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return true, nil
	}
	var b [1]byte
	if _, err := f.ReadAt(b[:], info.Size()-1); err != nil {
		return false, err
	}
	return b[0] == '\n', nil
}
