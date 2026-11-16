package promotion

import (
	"fmt"
	"sort"

	"github.com/scullxbones/armature/internal/ops"
)

// ActiveStop is a tree-scoped jidoka stop that currently blocks promotion-ready.
type ActiveStop struct {
	TreeRoot string
	IssueID  string
	PR       string
	Step     string
	Kind     string
	AndonURL string
}

// TreeRoot walks Parent links to the issue-tree root (id itself when parentless).
func TreeRoot(g Graph, id string) string {
	if id == "" || g == nil {
		return id
	}
	seen := map[string]bool{}
	cur := id
	for {
		if seen[cur] {
			return cur
		}
		seen[cur] = true
		n, ok := g[cur]
		if !ok || n.Parent == "" {
			return cur
		}
		if _, pok := g[n.Parent]; !pok {
			return cur
		}
		cur = n.Parent
	}
}

// TreeMemberIDs returns the root plus every descendant under root (pre-order).
func TreeMemberIDs(g Graph, root string) []string {
	if root == "" || g == nil {
		return nil
	}
	if _, ok := g[root]; !ok {
		return []string{root}
	}
	seen := map[string]bool{}
	var out []string
	var walk func(string)
	walk = func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
		n, ok := g[id]
		if !ok {
			return
		}
		kids := append([]string(nil), n.Children...)
		sort.Strings(kids)
		for _, ch := range kids {
			walk(ch)
		}
	}
	walk(root)
	return out
}

// ActiveStops folds trip/clear history into currently armed stops (one per tree root).
// History is applied in (At, Seq) order so later clears/trips win regardless of
// which worker log file ReadDir returned first.
func ActiveStops(history []ops.PromotionStop) []ActiveStop {
	sorted := append([]ops.PromotionStop(nil), history...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].At != sorted[j].At {
			return sorted[i].At < sorted[j].At
		}
		return sorted[i].Seq < sorted[j].Seq
	})

	type state struct {
		active ActiveStop
		armed  bool
	}
	byRoot := map[string]*state{}
	order := make([]string, 0)
	for _, rec := range sorted {
		root := rec.TreeRoot
		if root == "" {
			continue
		}
		st, ok := byRoot[root]
		if !ok {
			st = &state{}
			byRoot[root] = st
			order = append(order, root)
		}
		switch rec.Action {
		case ops.PromotionStopTrip:
			st.armed = true
			st.active = ActiveStop{
				TreeRoot: root,
				IssueID:  rec.IssueID,
				PR:       rec.PR,
				Step:     rec.Step,
				Kind:     rec.Kind,
				AndonURL: rec.AndonURL,
			}
		case ops.PromotionStopClear:
			st.armed = false
			st.active = ActiveStop{}
		}
	}
	out := make([]ActiveStop, 0)
	for _, root := range order {
		st := byRoot[root]
		if st != nil && st.armed {
			out = append(out, st.active)
		}
	}
	return out
}

// StopCovering returns the active stop whose tree covers any of issueIDs.
func StopCovering(stops []ActiveStop, g Graph, issueIDs []string) *ActiveStop {
	if len(stops) == 0 || len(issueIDs) == 0 {
		return nil
	}
	covered := map[string]ActiveStop{}
	for _, stop := range stops {
		for _, id := range TreeMemberIDs(g, stop.TreeRoot) {
			covered[id] = stop
		}
		// Always cover the recorded root even if graph lacks it.
		covered[stop.TreeRoot] = stop
		if stop.IssueID != "" {
			covered[stop.IssueID] = stop
		}
	}
	for _, id := range issueIDs {
		if stop, ok := covered[id]; ok {
			s := stop
			return &s
		}
		if g != nil {
			root := TreeRoot(g, id)
			if stop, ok := covered[root]; ok {
				s := stop
				return &s
			}
		}
	}
	return nil
}

// KindSeen reports whether history already recorded a trip of the same kind for treeRoot.
func KindSeen(history []ops.PromotionStop, treeRoot, kind string) bool {
	if treeRoot == "" || kind == "" {
		return false
	}
	for _, rec := range history {
		if rec.Action == ops.PromotionStopTrip && rec.TreeRoot == treeRoot && rec.Kind == kind {
			return true
		}
	}
	return false
}

// TripPlan is the jidoka action for one land/sweep failure.
type TripPlan struct {
	TreeRoot    string
	OpenAndon   bool
	OpenDogfood bool
}

// PlanTrip decides andon vs dogfood for a failure on issueID within g.
// First trip of a kind opens andon; a repeated kind opens a dogfood finding instead.
func PlanTrip(g Graph, history []ops.PromotionStop, issueID, kind string) TripPlan {
	root := TreeRoot(g, issueID)
	if root == "" {
		root = issueID
	}
	plan := TripPlan{TreeRoot: root}
	if KindSeen(history, root, kind) {
		plan.OpenDogfood = true
		return plan
	}
	plan.OpenAndon = true
	return plan
}

func checkJidokaStops(in ReadyInput) error {
	if len(in.Stops) == 0 {
		return nil
	}
	stop := StopCovering(in.Stops, in.Graph, in.IssueIDs)
	if stop == nil {
		return nil
	}
	return fmt.Errorf(
		"promotion-ready: jidoka stop on tree %s (PR %s, issue %s, step %s)",
		stop.TreeRoot, stop.PR, stop.IssueID, stop.Step,
	)
}
