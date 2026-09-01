package materialize

import (
	"github.com/scullxbones/armature/internal/adapters"
)

// CurrentStateVersion is the schema version of the issue snapshots this build
// writes. Bump it whenever materialized state gains a field that incremental
// replay depends on but cannot reconstruct from a snapshot written by another
// build — a checkpoint whose version differs from this one forces a cold
// replay instead of trusting the cached issues (see runFullPipeline). The
// check is a mismatch, not a lower-than: a snapshot from a newer build is
// equally untrustworthy, since this decoder drops the fields it does not know.
//
// 1: Issue.RollupStatusBefore. Before it, a rollup promotion was recorded as a
// bare merged status, indistinguishable from one an op asserted, so retraction
// could not reach it and an incremental run kept a parent merged where a cold
// replay did not. See TOPTIER-B1.
const CurrentStateVersion = 1

type Checkpoint struct {
	LastCommitSHA string `json:"last_materialized_commit"`
	// StateVersion is the CurrentStateVersion of the build that wrote the issue
	// snapshots this checkpoint accompanies. Absent (0) in checkpoints written
	// before versioning existed.
	StateVersion int              `json:"state_version,omitempty"`
	ByteOffsets  map[string]int64 `json:"byte_offsets"`
}

// WriteCheckpoint stamps the checkpoint with CurrentStateVersion. Stamping here
// rather than at each call site is what keeps the version honest: it always
// describes the build that actually wrote the accompanying snapshots.
func WriteCheckpoint(path string, cp Checkpoint) error {
	cp.StateVersion = CurrentStateVersion
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
