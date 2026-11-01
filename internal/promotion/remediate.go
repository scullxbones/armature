package promotion

import (
	"slices"
	"strings"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/ops"
)

// Node is the minimal issue-graph fact RemediationsForIssue needs.
type Node struct {
	Status   string
	Children []string
}

// Graph maps issue ID → status/children. Callers project from index/issues;
// promotion never imports materialize.
type Graph map[string]Node

const (
	// ChildrenMergedReopen is the honest next step when a non-leaf's
	// descendants are all merged (rollup/reopen — never arm merged on parent).
	ChildrenMergedReopenPrefix = "arm reopen --issue "
	// FallbackGuidance is shape-agnostic text when no graph is available.
	FallbackGuidance = "promote unmerged descendants first; reserve leaf remediations for leaves only"
	// ChildrenMergedGuidance explains the non-leaf all-merged case for --explain.
	ChildrenMergedGuidance = "children are merged; reopen or investigate the non-leaf rollup — do not run arm merged on the non-leaf"
)

// RemediationsForIssue walks the issue tree under id and returns one honest
// next-step command per pending descendant (and for a leaf, for the leaf
// itself). Rules:
//   - merged (and cancelled) nodes are skipped
//   - non-leaves are never suggested as arm merged; the walk recurses to leaves
//   - done unmerged leaf → arm delivery record (same truth as sync/legacy)
//   - open / other unfinished leaf → arm claim (honest non-merged next step)
//
// Empty result means every descendant is merged/cancelled (or id unknown).
func RemediationsForIssue(id string, g Graph) []string {
	if id == "" || g == nil {
		return nil
	}
	node, ok := g[id]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	if len(node.Children) > 0 {
		collectFromChildren(node.Children, g, seen, &out)
		return out
	}
	if cmd := leafRemediation(id, node); cmd != "" {
		return []string{cmd}
	}
	return nil
}

func collectFromChildren(children []string, g Graph, seen map[string]bool, out *[]string) {
	sorted := append([]string(nil), children...)
	slices.Sort(sorted)
	for _, childID := range sorted {
		collectNode(childID, g, seen, out)
	}
}

func collectNode(id string, g Graph, seen map[string]bool, out *[]string) {
	if id == "" || seen[id] {
		return
	}
	seen[id] = true
	node, ok := g[id]
	if !ok {
		return
	}
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return
	}
	if len(node.Children) > 0 {
		// Non-leaf: never arm merged here — recurse to pending leaves.
		collectFromChildren(node.Children, g, seen, out)
		return
	}
	if cmd := leafRemediation(id, node); cmd != "" {
		*out = append(*out, cmd)
	}
}

func leafRemediation(id string, node Node) string {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return ""
	case ops.StatusDone:
		// Sync/legacy truth: done leaf without a proven delivery path starts
		// at delivery record. arm merged fails without worktree/hook-log target.
		return delivery.RecordArgv(id)
	default:
		// open / claimed / in-progress / blocked — not arm merged.
		return "arm claim --issue " + id
	}
}

// ChildrenMergedReopenArgv is the actionable recovery when RemediationsForIssue
// returns empty for a known non-leaf (all descendants terminal-merged).
func ChildrenMergedReopenArgv(issueID string) string {
	return ChildrenMergedReopenPrefix + issueID
}

// FirstRemediation picks sync's singular next_action from a list.
func FirstRemediation(actions []string) string {
	if len(actions) == 0 {
		return "arm doctor"
	}
	return actions[0]
}

// IsRunnableArmCommand reports whether s looks like an arm argv (not prose).
func IsRunnableArmCommand(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "arm ")
}
