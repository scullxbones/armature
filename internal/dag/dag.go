// Package dag builds and validates the issue dependency graph, detecting cycles and computing blocked/ready state from op-log-derived edges.
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

// ScopedHasCycle checks for cycles within a restricted set of node IDs.
// It performs DFS starting from id, following all BlockedBy edges (including cross-scope ones)
// but only reporting a cycle if the closing node (already in recStack) is in scope.
// This prevents false positives from unrelated cycles entirely outside the scope.
func (g *Graph) ScopedHasCycle(id string, scope map[string]bool) bool {
	visited := map[string]bool{}
	recStack := map[string]bool{}

	var dfs func(string) bool
	dfs = func(nodeID string) bool {
		if recStack[nodeID] {
			// Cycle detected: only report true if the closing node is in scope.
			// This prevents false positives from unrelated cycles outside the scope.
			return scope[nodeID]
		}
		if visited[nodeID] {
			return false
		}

		visited[nodeID] = true
		recStack[nodeID] = true

		// Walk BlockedBy edges: follow ALL blockers (including out-of-scope ones)
		// to detect cross-scope cycles that affect scoped nodes.
		for _, dep := range g.Blockers(nodeID) {
			if dfs(dep) {
				return true
			}
		}

		// Walk Children edges within scope only. Parent-child cycles are structurally
		// impossible once parent-link validation passes, so cross-scope child traversal
		// adds no cycle-detection value and risks false positives.
		_, children := g.Hierarchy(nodeID)
		for _, child := range children {
			if scope[child] {
				if dfs(child) {
					return true
				}
			}
		}

		recStack[nodeID] = false
		return false
	}

	return dfs(id)
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

// FromIndex constructs a Graph from a map of node IDs to Node pointers.
// It creates a new DAG with all nodes from the index and returns a Graph projection.
func FromIndex(index map[string]*Node) *Graph {
	d := New()
	for _, node := range index {
		// We don't check for errors here since we own the nodes from the index
		_ = d.AddNode(node) //nolint:errcheck // AddNode only errors on duplicate IDs; ID uniqueness is enforced by caller
	}
	return NewGraph(d)
}

// BuildGraph constructs a Graph from a node index map.
// This is a convenience helper that converts a map of Node pointers into a Graph suitable
// for context assembly and ready-queue computation.
// All slices are defensively copied.
// Previously named GraphFromState — renamed to avoid ambiguity with materialize.GraphFromState,
// which takes a *materialize.State rather than a map[string]*dag.Node.
func BuildGraph(index map[string]*Node) *Graph {
	nodeIndex := make(map[string]*Node)
	for id, node := range index {
		copiedNode := &Node{
			ID:        node.ID,
			Title:     node.Title,
			Type:      node.Type,
			Parent:    node.Parent,
			Children:  make([]string, len(node.Children)),
			BlockedBy: make([]string, len(node.BlockedBy)),
			Blocks:    make([]string, len(node.Blocks)),
		}
		copy(copiedNode.Children, node.Children)
		copy(copiedNode.BlockedBy, node.BlockedBy)
		copy(copiedNode.Blocks, node.Blocks)
		nodeIndex[id] = copiedNode
	}
	return FromIndex(nodeIndex)
}

// isLegalHierarchy validates that a node index has consistent parent-child relationships.
// It returns true if every node's parent reference is satisfied and every parent's
// Children list contains consistent entries. An empty index is considered valid.
func isLegalHierarchy(index map[string]*Node) bool {
	// Empty index is valid
	if len(index) == 0 {
		return true
	}

	// Check parent-child consistency
	for id, node := range index {
		// Check that if a node has a parent, the parent exists
		if node.Parent != "" {
			parent, exists := index[node.Parent]
			if !exists {
				return false
			}
			// Check that the parent actually lists this node as a child
			if !slices.Contains(parent.Children, id) {
				return false
			}
		}
	}

	return true
}
