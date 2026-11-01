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
	Parent   string
	Children []string
	Base     string // delivery base SHA when known
	Tip      string // delivery tip SHA when known
}

// Graph maps issue ID → status/children/parent/delivery. Callers project from
// index/issues; promotion never imports materialize.
type Graph map[string]Node

const (
	// FallbackGuidance is shape-agnostic text when no graph is available.
	FallbackGuidance = "promote unmerged descendants first; reserve leaf remediations for leaves only"
	// TreeRepairGuidance explains open/unfinished work under a merged ancestor.
	TreeRepairGuidance = "tree needs repair: open work under a merged ancestor — reopen the merged ancestor or cancel/reparent the open child"
)

// RemediationsForIssue walks the issue tree under id and returns one honest
// next-step command per pending descendant (and for a leaf, for the leaf
// itself). Rules:
//   - merged (and cancelled) nodes are skipped
//   - non-leaves are never suggested as arm merged; the walk recurses to leaves
//   - done unmerged leaf → arm delivery record (filled SHAs when known; else template)
//   - open leaf under a merged ancestor → repair (reopen ancestor / cancel child);
//     never arm claim (inconsistent tree)
//   - open leaf with no merged ancestor → arm claim --issue … --worktree
//
// Empty result means every descendant is merged/cancelled (or id unknown).
// Callers must not invent reopen-for-all-merged-children: materialize rollup
// already promotes that shape to merged.
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
	return leafRemediations(id, node, g)
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
		collectFromChildren(node.Children, g, seen, out)
		return
	}
	*out = append(*out, leafRemediations(id, node, g)...)
}

func leafRemediations(id string, node Node, g Graph) []string {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	case ops.StatusDone:
		return []string{doneLeafDeliveryRecord(id, node)}
	default:
		// open / claimed / in-progress / blocked
		if anc := nearestMergedAncestor(id, g); anc != "" {
			return []string{
				"arm reopen --issue " + anc,
				"arm transition --issue " + id + " --to cancelled --force",
			}
		}
		return []string{"arm claim --issue " + id + " --worktree"}
	}
}

func doneLeafDeliveryRecord(id string, node Node) string {
	base := strings.TrimSpace(node.Base)
	tip := strings.TrimSpace(node.Tip)
	if base != "" && tip != "" {
		return delivery.RecordArgvFilled(id, base, tip)
	}
	// Template: legacy done with no claim/delivery provenance.
	return delivery.RecordArgv(id)
}

func nearestMergedAncestor(id string, g Graph) string {
	seen := map[string]bool{}
	for cur := g[id].Parent; cur != ""; cur = g[cur].Parent {
		if seen[cur] {
			return ""
		}
		seen[cur] = true
		node, ok := g[cur]
		if !ok {
			return ""
		}
		if node.Status == ops.StatusMerged {
			return cur
		}
	}
	return ""
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

// IsDeliveryRecordTemplate reports a delivery-record argv that still has SHA placeholders.
func IsDeliveryRecordTemplate(cmd string) bool {
	return strings.HasPrefix(cmd, "arm delivery record") && strings.Contains(cmd, "<sha>")
}
