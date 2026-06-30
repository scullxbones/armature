package materialize

import (
	"github.com/scullxbones/armature/internal/dag"
)

// GraphFromState constructs a dag.Graph from a materialize.State.
// All slices are defensively copied so callers can safely mutate state
// without corrupting the returned graph.
func GraphFromState(state *State) *dag.Graph {
	nodeIndex := make(map[string]*dag.Node, len(state.Issues))
	for id, issue := range state.Issues {
		nodeIndex[id] = &dag.Node{
			ID:        id,
			Title:     issue.Title,
			Type:      issue.Type,
			Parent:    issue.Parent,
			Children:  append([]string(nil), issue.Children...),
			BlockedBy: append([]string(nil), issue.BlockedBy...),
			Blocks:    append([]string(nil), issue.Blocks...),
		}
	}
	return dag.FromIndex(nodeIndex)
}
