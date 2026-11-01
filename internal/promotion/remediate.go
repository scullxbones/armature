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
	// MoreStepsFollowGuidance marks descendant promotion steps that do not by
	// themselves clear a done-under-merged (D14) finding.
	MoreStepsFollowGuidance = "more steps follow after tree repair to promote remaining descendants"
	// LegacyTemplateGuidance labels <sha>/<assessment.json> placeholders as
	// legacy templates (same honesty as promotion kind "legacy").
	LegacyTemplateGuidance = "legacy: template argv — substitute real values; placeholders fail closed"
)

// RemediationsForIssue walks the issue tree under id and returns honest next
// steps. A remediation counts only if running it repairs the relevant finding.
//
// Rules:
//   - pending node under a merged ancestor → complete tree repair first
//     (reopen ancestor OR cancel/reparent this node); never arm claim
//   - further descendant promotion steps are listed after MoreStepsFollowGuidance
//   - done unmerged leaf (healthy tree) → delivery record (filled SHAs when
//     known, plus review-record assessment step; else legacy <sha> template)
//   - open leaf (healthy tree) → arm claim --issue … --worktree
//   - non-leaf with only-cancelled descendants → cancel/reopen (RunRollup will
//     not promote: needs ≥1 merged child)
//   - rollup-eligible all-merged descendants → empty (materialize owns it)
//
// Merged/cancelled nodes are skipped as walk targets. Non-leaves are never
// suggested as arm merged.
func RemediationsForIssue(id string, g Graph) []string {
	if id == "" || g == nil {
		return nil
	}
	node, ok := g[id]
	if !ok {
		return nil
	}
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	}

	repair := treeRepairFor(id, node, g)
	more := descendantOrLeafSteps(id, node, g)
	more = subtractCmds(more, repair)

	if len(repair) > 0 && len(more) > 0 {
		out := append([]string{}, repair...)
		out = append(out, MoreStepsFollowGuidance)
		out = append(out, more...)
		return out
	}
	if len(repair) > 0 {
		return repair
	}
	return more
}

func descendantOrLeafSteps(id string, node Node, g Graph) []string {
	seen := map[string]bool{}
	var out []string
	if len(node.Children) > 0 {
		collectFromChildren(node.Children, g, seen, &out)
		if len(out) == 0 {
			return strandedNonLeafRemediations(id, node, g)
		}
		return out
	}
	return leafPromotionRemediations(id, node, g)
}

// treeRepairFor is the complete D14/inconsistent-tree repair for a pending
// node under a merged ancestor. Running either command clears done-under-merged
// for this node without requiring descendant promotion.
func treeRepairFor(id string, node Node, g Graph) []string {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	}
	anc := nearestMergedAncestor(id, g)
	if anc == "" {
		return nil
	}
	return []string{
		"arm reopen --issue " + anc,
		"arm transition --issue " + id + " --to cancelled --force",
	}
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
	// Nested pending under merged ancestor: emit that child's complete repair
	// (and any further promotion steps), not a claim.
	if repair := treeRepairFor(id, node, g); len(repair) > 0 {
		*out = append(*out, repair...)
		if len(node.Children) > 0 {
			var nested []string
			collectFromChildren(node.Children, g, seen, &nested)
			if len(nested) == 0 {
				nested = strandedNonLeafRemediations(id, node, g)
			}
			nested = subtractCmds(nested, repair)
			if len(nested) > 0 {
				*out = append(*out, MoreStepsFollowGuidance)
				*out = append(*out, nested...)
			}
		} else if node.Status == ops.StatusDone {
			promo := leafPromotionRemediations(id, node, g)
			promo = subtractCmds(promo, repair)
			if len(promo) > 0 {
				*out = append(*out, MoreStepsFollowGuidance)
				*out = append(*out, promo...)
			}
		}
		return
	}
	if len(node.Children) > 0 {
		before := len(*out)
		collectFromChildren(node.Children, g, seen, out)
		if len(*out) == before {
			*out = append(*out, strandedNonLeafRemediations(id, node, g)...)
		}
		return
	}
	*out = append(*out, leafPromotionRemediations(id, node, g)...)
}

func leafPromotionRemediations(id string, node Node, g Graph) []string {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	case ops.StatusDone:
		return doneLeafPromotion(id, node)
	default:
		// Healthy open leaf (no merged ancestor — callers gate tree repair first).
		if nearestMergedAncestor(id, g) != "" {
			return nil // tree repair owns this shape
		}
		return []string{"arm claim --issue " + id + " --worktree"}
	}
}

// strandedNonLeafRemediations covers a pending non-leaf whose walk found no
// pending leaf work. All-cancelled descendants will not roll up (RunRollup
// requires ≥1 merged child); emit cancel/reopen. All-merged (rollup-eligible)
// returns nil — materialize owns that shape.
func strandedNonLeafRemediations(id string, node Node, g Graph) []string {
	switch node.Status {
	case ops.StatusMerged, ops.StatusCancelled:
		return nil
	}
	if !onlyCancelledDescendants(id, g) {
		return nil
	}
	cancel := "arm transition --issue " + id + " --to cancelled --force"
	switch node.Status {
	case ops.StatusDone, ops.StatusBlocked:
		return []string{cancel, "arm reopen --issue " + id}
	default:
		return []string{cancel}
	}
}

// onlyCancelledDescendants reports that id has children and every descendant
// path ends in cancelled work — no merged child and no pending leaf. Unknown
// nodes fail closed (not only-cancelled).
func onlyCancelledDescendants(id string, g Graph) bool {
	node, ok := g[id]
	if !ok || len(node.Children) == 0 {
		return false
	}
	seen := map[string]bool{}
	sawCancelled := false
	var walk func(string) bool
	walk = func(cid string) bool {
		if cid == "" || seen[cid] {
			return true
		}
		seen[cid] = true
		n, ok := g[cid]
		if !ok {
			return false
		}
		switch n.Status {
		case ops.StatusMerged:
			return false
		case ops.StatusCancelled:
			sawCancelled = true
			return true
		}
		if len(n.Children) == 0 {
			return false
		}
		for _, ch := range n.Children {
			if !walk(ch) {
				return false
			}
		}
		return true
	}
	for _, ch := range node.Children {
		if !walk(ch) {
			return false
		}
	}
	return sawCancelled
}

func doneLeafPromotion(id string, node Node) []string {
	base := strings.TrimSpace(node.Base)
	tip := strings.TrimSpace(node.Tip)
	if base != "" && tip != "" {
		return []string{
			delivery.RecordArgvFilled(id, base, tip),
			"arm review record --issue " + id + " --assessment <assessment.json>",
		}
	}
	// Legacy template: no claim/delivery provenance — labeled like kind "legacy".
	return []string{LegacyTemplateGuidance, delivery.RecordArgv(id)}
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

func subtractCmds(from, remove []string) []string {
	if len(from) == 0 || len(remove) == 0 {
		return from
	}
	drop := map[string]bool{}
	for _, r := range remove {
		drop[r] = true
	}
	var out []string
	for _, c := range from {
		if drop[c] || c == MoreStepsFollowGuidance {
			continue
		}
		out = append(out, c)
	}
	return out
}

// FirstRemediation picks sync's singular next_action from a list.
func FirstRemediation(actions []string) string {
	for _, a := range actions {
		if a == MoreStepsFollowGuidance || a == LegacyTemplateGuidance || a == TreeRepairGuidance || a == FallbackGuidance {
			continue
		}
		return a
	}
	return "arm doctor"
}

// IsRunnableArmCommand reports whether s looks like an arm argv (not prose).
func IsRunnableArmCommand(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "arm ")
}

// IsDeliveryRecordTemplate reports a delivery-record argv that still has SHA placeholders.
func IsDeliveryRecordTemplate(cmd string) bool {
	return strings.HasPrefix(cmd, "arm delivery record") && strings.Contains(cmd, "<sha>")
}

// IsLegacyTemplateRemediation reports placeholder recovery labeled like kind "legacy".
func IsLegacyTemplateRemediation(cmd string) bool {
	if cmd == LegacyTemplateGuidance {
		return true
	}
	if IsDeliveryRecordTemplate(cmd) {
		return true
	}
	return strings.HasPrefix(cmd, "arm review record") && strings.Contains(cmd, "<assessment.json>")
}

// IsGuidanceRemediation reports non-argv prose entries in a remediation list.
func IsGuidanceRemediation(cmd string) bool {
	switch cmd {
	case FallbackGuidance, TreeRepairGuidance, MoreStepsFollowGuidance, LegacyTemplateGuidance:
		return true
	default:
		return false
	}
}
