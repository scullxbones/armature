package dag

import (
	"fmt"
	"slices"
)

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

// HasCycle checks for cycles using DFS across both the parent-child hierarchy
// and the blocking dependency graph simultaneously.
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
	if node == nil {
		recStack[nodeID] = false
		return false
	}
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
			if !slices.Contains(parent.Children, id) {
				return fmt.Errorf("node %s lists parent %s, but parent doesn't list it as child", id, node.Parent)
			}
		}
	}
	return nil
}

// Graph is a read-only projection of the DAG that provides a unified interface
// for querying ancestry, descendants, blockers, blocks, hierarchy, cycle detection,
// and depth of nodes.
type Graph struct {
	dag *DAG
}

// NewGraph creates a new Graph projection from the given DAG.
func NewGraph(d *DAG) *Graph {
	if d == nil {
		panic("NewGraph: DAG must not be nil")
	}
	return &Graph{dag: d}
}

// Ancestry returns the chain of hierarchical parent nodes from the given node up to the root.
func (g *Graph) Ancestry(id string) []string {
	ancestors := []string{}
	visited := map[string]bool{id: true}
	node := g.dag.nodes[id]
	if node == nil {
		return ancestors
	}

	current := node.Parent
	for current != "" {
		if visited[current] {
			break // cycle guard
		}
		visited[current] = true
		ancestors = append(ancestors, current)
		parentNode := g.dag.nodes[current]
		if parentNode == nil {
			break
		}
		current = parentNode.Parent
	}
	return ancestors
}

// Descendants returns all downstream descendants of a node (all nodes that
// depend on this node being completed, following child links).
func (g *Graph) Descendants(id string) []string {
	var descendants []string
	queue := []string{id}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		node := g.dag.nodes[current]
		if node == nil {
			continue
		}

		for _, childID := range node.Children {
			if !visited[childID] {
				descendants = append(descendants, childID)
				queue = append(queue, childID)
			}
		}
	}

	return descendants
}

// Blockers returns all direct blocked_by dependencies of a node.
func (g *Graph) Blockers(id string) []string {
	node := g.dag.nodes[id]
	if node == nil {
		return nil
	}
	result := make([]string, len(node.BlockedBy))
	copy(result, node.BlockedBy)
	return result
}

// Blocks returns all nodes that this node directly blocks.
func (g *Graph) Blocks(id string) []string {
	node := g.dag.nodes[id]
	if node == nil {
		return nil
	}
	result := make([]string, len(node.Blocks))
	copy(result, node.Blocks)
	return result
}

// Hierarchy returns the parent and children of a node as (parentID, childIDs).
func (g *Graph) Hierarchy(id string) (string, []string) {
	node := g.dag.nodes[id]
	if node == nil {
		return "", nil
	}
	result := make([]string, len(node.Children))
	copy(result, node.Children)
	return node.Parent, result
}

// HasCycle returns true if the graph contains a cycle.
func (g *Graph) HasCycle() bool {
	return g.dag.HasCycle()
}

// Depth returns the depth of a node from its root (node with no parent).
// A root node has depth 0, its direct children have depth 1, etc.
func (g *Graph) Depth(id string) int {
	visited := map[string]bool{}
	depth := 0
	node := g.dag.nodes[id]
	if node == nil {
		return depth
	}

	current := node.Parent
	for current != "" {
		if visited[current] {
			break // cycle guard
		}
		visited[current] = true
		depth++
		parentNode := g.dag.nodes[current]
		if parentNode == nil {
			break
		}
		current = parentNode.Parent
	}
	return depth
}
