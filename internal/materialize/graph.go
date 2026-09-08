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
		nodeIndex[id] = graphNode(id, issue.Title, issue.Type, issue.Parent, issue.Children, issue.BlockedBy, issue.Blocks)
	}
	return dag.FromIndex(nodeIndex)
}

// GraphFromIndex constructs a dag.Graph from a denormalized Index.
// All slices are defensively copied so callers can safely mutate index
// entries without corrupting the returned graph.
func GraphFromIndex(index Index) *dag.Graph {
	nodeIndex := make(map[string]*dag.Node, len(index))
	for id, entry := range index {
		nodeIndex[id] = graphNode(id, entry.Title, entry.Type, entry.Parent, entry.Children, entry.BlockedBy, entry.Blocks)
	}
	return dag.FromIndex(nodeIndex)
}

func graphNode(id, title, typ, parent string, children, blockedBy, blocks []string) *dag.Node {
	return &dag.Node{
		ID:        id,
		Title:     title,
		Type:      typ,
		Parent:    parent,
		Children:  append([]string(nil), children...),
		BlockedBy: append([]string(nil), blockedBy...),
		Blocks:    append([]string(nil), blocks...),
	}
}
