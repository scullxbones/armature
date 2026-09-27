// Package dag builds and validates the issue dependency graph, detecting cycles and computing blocked/ready state from op-log-derived edges.
package dag

import (
	"fmt"
)

type Node struct {
	ID        string
	Title     string
	Type      string // "epic", "story", "task"
	Parent    string // parent node ID (empty for root)
	Children  []string
	BlockedBy []string
	Blocks    []string
}

// Graph is the issue dependency graph: ancestry, descendants, blockers,
// hierarchy, cycle detection, and depth. Callers build graphs with FromIndex.
type Graph struct {
	nodes map[string]*Node
}

func newGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

func (g *Graph) addNode(n *Node) error {
	if _, exists := g.nodes[n.ID]; exists {
		return fmt.Errorf("node %s already exists", n.ID)
	}
	g.nodes[n.ID] = n
	return nil
}

// HasCycle checks for cycles using DFS across both the parent-child hierarchy
// and the blocking dependency graph simultaneously.
func (g *Graph) HasCycle() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for id := range g.nodes {
		if !visited[id] {
			if g.hasCycleDFS(id, visited, recStack) {
				return true
			}
		}
	}
	return false
}

func (g *Graph) hasCycleDFS(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true

	node := g.nodes[nodeID]
	if node == nil {
		recStack[nodeID] = false
		return false
	}
	for _, childID := range node.Children {
		if !visited[childID] {
			if g.hasCycleDFS(childID, visited, recStack) {
				return true
			}
		} else if recStack[childID] {
			return true
		}
	}

	for _, blockedID := range node.BlockedBy {
		if !visited[blockedID] {
			if g.hasCycleDFS(blockedID, visited, recStack) {
				return true
			}
		} else if recStack[blockedID] {
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

// Ancestry returns the chain of hierarchical parent nodes from the given node up to the root.
func (g *Graph) Ancestry(id string) []string {
	ancestors := []string{}
	visited := map[string]bool{id: true}
	node := g.nodes[id]
	if node == nil {
		return ancestors
	}

	current := node.Parent
	for current != "" {
		if visited[current] {
			break
		}
		visited[current] = true
		ancestors = append(ancestors, current)
		next, ok := g.parentOrStop(current)
		if !ok {
			break
		}
		current = next
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

		node := g.nodes[current]
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

func (g *Graph) blockers(id string) []string {
	node := g.nodes[id]
	if node == nil {
		return nil
	}
	result := make([]string, len(node.BlockedBy))
	copy(result, node.BlockedBy)
	return result
}

// Hierarchy returns the parent and children of a node as (parentID, childIDs).
func (g *Graph) Hierarchy(id string) (string, []string) {
	node := g.nodes[id]
	if node == nil {
		return "", nil
	}
	result := make([]string, len(node.Children))
	copy(result, node.Children)
	return node.Parent, result
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
			return scope[nodeID]
		}
		if visited[nodeID] {
			return false
		}

		visited[nodeID] = true
		recStack[nodeID] = true

		if g.walkBlockedByIncludingOutOfScope(nodeID, dfs) {
			return true
		}
		if g.walkChildrenInScope(nodeID, scope, dfs) {
			return true
		}

		recStack[nodeID] = false
		return false
	}

	return dfs(id)
}

func (g *Graph) walkBlockedByIncludingOutOfScope(nodeID string, dfs func(string) bool) bool {
	for _, dep := range g.blockers(nodeID) {
		if dfs(dep) {
			return true
		}
	}
	return false
}

func (g *Graph) walkChildrenInScope(nodeID string, scope map[string]bool, dfs func(string) bool) bool {
	_, children := g.Hierarchy(nodeID)
	for _, child := range children {
		if scope[child] && dfs(child) {
			return true
		}
	}
	return false
}

// Depth returns the depth of a node from its root (node with no parent).
// A root node has depth 0, its direct children have depth 1, etc.
func (g *Graph) Depth(id string) int {
	visited := map[string]bool{}
	depth := 0
	node := g.nodes[id]
	if node == nil {
		return depth
	}

	current := node.Parent
	for current != "" {
		if visited[current] {
			break
		}
		visited[current] = true
		depth++
		next, ok := g.parentOrStop(current)
		if !ok {
			break
		}
		current = next
	}
	return depth
}

func (g *Graph) parentOrStop(id string) (string, bool) {
	parentNode := g.nodes[id]
	if parentNode == nil {
		return "", false
	}
	return parentNode.Parent, true
}

// FromIndex constructs a Graph from a map of node IDs to Node pointers.
func FromIndex(index map[string]*Node) *Graph {
	g := newGraph()
	for _, node := range index {
		g.nodes[node.ID] = node
	}
	return g
}
