package materialize

import (
	"github.com/scullxbones/armature/internal/adapters"
)

const currentStateVersion = 2

type Checkpoint struct {
	LastCommitSHA string `json:"last_materialized_commit"`
	// StateVersion is the currentStateVersion of the build that wrote the issue
	// snapshots this checkpoint accompanies. Absent (0) in checkpoints written
	// before versioning existed.
	StateVersion int              `json:"state_version,omitempty"`
	ByteOffsets  map[string]int64 `json:"byte_offsets"`
}

func writeCheckpoint(path string, cp Checkpoint) error {
	cp.StateVersion = currentStateVersion
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
