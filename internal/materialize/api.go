package materialize

import (
	"fmt"

	"github.com/scullxbones/armature/internal/ops"
)

// MaterializeCold replays the complete op log onto a fresh State.
func MaterializeCold(allOps []ops.Op) (*State, error) {
	state := NewState()
	if err := ApplyOpsSorted(state, allOps); err != nil {
		return nil, err
	}
	return state, nil
}

// MaterializeIncremental retracts derived rollup promotions, then full-replays
// allOps onto state. It is not a delta of new ops: callers pass the complete log.
// Asserted issue state is reinitialized first so non-idempotent handlers
// (PriorOutcomes on reopen, notes, and similar appends) do not re-apply on a
// snapshot that already absorbed the same ops.
func MaterializeIncremental(state *State, allOps []ops.Op) error {
	if state == nil {
		return fmt.Errorf("MaterializeIncremental: state is nil")
	}
	*state = *NewState()
	return ApplyOpsSorted(state, allOps)
}
