package ready

import (
	"slices"
	"sort"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
)

// PartitionWaves partitions ready entries into scope-disjoint waves using greedy first-fit.
// Priority tier is a hard boundary between waves (process tier by tier).
// Within a tier, items are ordered by scope-conflict degree (how many other ready items
// in the tier share scope with them) rather than depth/blocks/ID tie-break,
// so items likely to conflict are considered first for placement.
// Ancestor/descendant pairs are excluded from being placed in the same wave.
// Returns a slice of waves, where each wave is a slice of ReadyEntry.
func PartitionWaves(entries []ReadyEntry, index materialize.Index) [][]ReadyEntry {
	if len(entries) == 0 {
		return [][]ReadyEntry{}
	}
	graph := graphFromIndex(index)

	// Group entries by priority tier
	tierMap := make(map[string][]ReadyEntry)
	priorityOrder := []string{"critical", "high", "medium", "low", ""}

	for _, entry := range entries {
		tier := entry.Priority
		tierMap[tier] = append(tierMap[tier], entry)
	}

	var allWaves [][]ReadyEntry

	// Process known priority tiers first, then all custom tiers in lexical order.
	// This preserves hard boundaries while ensuring every input entry is emitted.
	orderedTiers := append([]string(nil), priorityOrder...)
	knownTiers := make(map[string]bool, len(priorityOrder))
	for _, tier := range priorityOrder {
		knownTiers[tier] = true
	}
	var customTiers []string
	for tier := range tierMap {
		if !knownTiers[tier] {
			customTiers = append(customTiers, tier)
		}
	}
	sort.Strings(customTiers)
	orderedTiers = append(orderedTiers, customTiers...)

	// Process each priority tier in order, ensuring hard boundaries between tiers.
	for _, tier := range orderedTiers {
		tierEntries, ok := tierMap[tier]
		if !ok || len(tierEntries) == 0 {
			continue
		}

		// Sort entries in this tier by scope-conflict degree (descending)
		// Items with higher conflict degree are considered first for placement
		sortByConflictDegree(tierEntries)

		// Greedy first-fit: for each candidate, try to place it into the first existing
		// wave where it has no scope overlap and no ancestor/descendant relationship
		var tierWaves [][]ReadyEntry

		for _, candidate := range tierEntries {
			placed := false

			// Try to place the candidate into an existing wave
			for waveIdx := range tierWaves {
				if canAddToWave(candidate, tierWaves[waveIdx], graph) {
					tierWaves[waveIdx] = append(tierWaves[waveIdx], candidate)
					placed = true
					break
				}
			}

			// If not placed in any existing wave, start a new wave
			if !placed {
				tierWaves = append(tierWaves, []ReadyEntry{candidate})
			}
		}

		// Add all waves from this tier to the overall waves
		allWaves = append(allWaves, tierWaves...)
	}

	return allWaves
}

// sortByConflictDegree sorts entries by their scope-conflict degree in descending order.
// Items with more conflicts (shared scopes with other ready items) are sorted first.
func sortByConflictDegree(entries []ReadyEntry) {
	// Compute conflict degree for each entry
	conflictDegrees := make(map[string]int)
	for _, entry := range entries {
		degree := 0
		for _, other := range entries {
			if entry.Issue != other.Issue {
				if claim.ScopesOverlap(entry.Scope, other.Scope) {
					degree++
				}
			}
		}
		conflictDegrees[entry.Issue] = degree
	}

	// Sort by conflict degree (descending), then by ID for determinism
	orderByConflictDegree := func(i, j int) bool {
		di := conflictDegrees[entries[i].Issue]
		dj := conflictDegrees[entries[j].Issue]
		if di != dj {
			return di > dj // higher degree first (descending)
		}
		return entries[i].Issue < entries[j].Issue // tie-break by ID
	}

	// Use insertion sort to maintain relative order for deterministic results
	for i := 1; i < len(entries); i++ {
		key := entries[i]
		j := i - 1
		for j >= 0 && orderByConflictDegree(j+1, j) {
			entries[j+1] = entries[j]
			j--
		}
		entries[j+1] = key
	}
}

// canAddToWave checks if a candidate entry can be added to a wave without:
// 1. Scope overlap with any existing member of the wave
// 2. Ancestor/descendant relationship with any existing member of the wave
func canAddToWave(candidate ReadyEntry, wave []ReadyEntry, graph *dag.Graph) bool {
	for _, existing := range wave {
		// Check scope overlap
		if claim.ScopesOverlap(candidate.Scope, existing.Scope) {
			return false
		}

		// Also check if they are direct ancestors/descendants (even if scopes don't overlap)
		if isAncestorOrDescendant(candidate.Issue, existing.Issue, graph) {
			return false
		}
	}
	return true
}

// isAncestorOrDescendant checks if one issue is an ancestor or descendant of another
func isAncestorOrDescendant(issueA, issueB string, graph *dag.Graph) bool {
	if graph == nil {
		return false
	}

	// Check if issueA is an ancestor of issueB (issueB is a descendant of issueA)
	if slices.Contains(graph.Descendants(issueA), issueB) {
		return true
	}

	// Check if issueB is an ancestor of issueA (issueA is a descendant of issueB)
	if slices.Contains(graph.Descendants(issueB), issueA) {
		return true
	}

	return false
}
