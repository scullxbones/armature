package dag

import "fmt"

// Node represents a work item in the DAG.
type Node struct {
	ID        string
	Title     string
	Type      string // "epic", "story", "task"
	Parent    string // parent node ID (empty for root)
	Children  []string
	BlockedBy []string
	Blocks    []string
}

// DAG represents the directed acyclic graph of work items.
type DAG struct {
	nodes map[string]*Node
}

// New creates an empty DAG.
func New() *DAG {
	return &DAG{
		nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the DAG.
func (d *DAG) AddNode(n *Node) error {
	if _, exists := d.nodes[n.ID]; exists {
		return fmt.Errorf("node %s already exists", n.ID)
	}
	d.nodes[n.ID] = n
	return nil
}

// Node retrieves a node by ID.
func (d *DAG) Node(id string) *Node {
	return d.nodes[id]
}

// HasCycle checks for cycles using DFS.
func (d *DAG) HasCycle() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for id := range d.nodes {
		if !visited[id] {
			if d.hasCycleDFS(id, visited, recStack) {
				return true
			}
		}
	}
	return false
}

func (d *DAG) hasCycleDFS(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true

	node := d.nodes[nodeID]
	for _, childID := range node.Children {
		if !visited[childID] {
			if d.hasCycleDFS(childID, visited, recStack) {
				return true
			}
		} else if recStack[childID] {
			return true
		}
	}

	for _, blockedID := range node.BlockedBy {
		if !visited[blockedID] {
			if d.hasCycleDFS(blockedID, visited, recStack) {
				return true
			}
		} else if recStack[blockedID] {
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

// ValidateParentChild checks that parent-child relationships are consistent.
func (d *DAG) ValidateParentChild() error {
	for id, node := range d.nodes {
		if node.Parent != "" {
			parent := d.nodes[node.Parent]
			if parent == nil {
				return fmt.Errorf("node %s has unknown parent %s", id, node.Parent)
			}
			// Check that parent actually lists this as a child
			found := false
			for _, childID := range parent.Children {
				if childID == id {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("node %s lists parent %s, but parent doesn't list it as child", id, node.Parent)
			}
		}
	}
	return nil
}
