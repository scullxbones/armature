package materialize

import (
	"github.com/scullxbones/armature/internal/adapters"
)

type Checkpoint struct {
	LastCommitSHA string           `json:"last_materialized_commit"`
	ByteOffsets   map[string]int64 `json:"byte_offsets"`
}

func WriteCheckpoint(path string, cp Checkpoint) error {
	return adapters.WriteCheckpointJSON(path, cp)
}

func LoadCheckpoint(path string) (Checkpoint, error) {
	var cp Checkpoint
	if err := adapters.LoadCheckpointJSON(path, &cp); err != nil {
		return Checkpoint{}, err
	}
	if cp.ByteOffsets == nil {
		cp.ByteOffsets = make(map[string]int64)
	}
	return cp, nil
}
