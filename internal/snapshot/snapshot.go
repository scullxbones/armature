package snapshot

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/materialize"
	"github.com/scullxbones/armature/internal/ops"
)

// Snapshot is the result of loading and materializing the full repo state.
type Snapshot struct {
	State    *materialize.State
	Index    materialize.Index
	Issues   map[string]*materialize.Issue
	Warnings []string
}

// Load materializes state from opsDir and stateDir, returning a populated Snapshot.
// Returns a non-nil Snapshot with empty collections when opsDir is empty.
func Load(opsDir, stateDir string, singleBranch bool) (*Snapshot, error) {
	items, offsets, warnings, err := ops.LoadFromDirWithOffsetsValidated(opsDir)
	if err != nil {
		return nil, fmt.Errorf("load ops: %w", err)
	}

	allOps := ops.ExtractOps(items)

	state, _, err := materialize.MaterializeAndReturn(stateDir, allOps, singleBranch, offsets)
	if err != nil {
		return nil, fmt.Errorf("materialize: %w", err)
	}

	index, err := materialize.LoadIndex(filepath.Join(stateDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}

	issues := state.Issues
	if issues == nil {
		issues = make(map[string]*materialize.Issue)
	}

	return &Snapshot{
		State:    state,
		Index:    index,
		Issues:   issues,
		Warnings: warnings,
	}, nil
}
