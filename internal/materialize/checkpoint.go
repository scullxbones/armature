package materialize

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Checkpoint struct {
	LastCommitSHA string           `json:"last_materialized_commit"`
	ByteOffsets   map[string]int64 `json:"byte_offsets"`
}

func WriteCheckpoint(path string, cp Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func LoadCheckpoint(path string) (Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Checkpoint{ByteOffsets: make(map[string]int64)}, nil
		}
		return Checkpoint{}, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, fmt.Errorf("parse checkpoint: %w", err)
	}
	if cp.ByteOffsets == nil {
		cp.ByteOffsets = make(map[string]int64)
	}
	return cp, nil
}
