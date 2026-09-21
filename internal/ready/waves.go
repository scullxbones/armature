package ready

import (
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
	graph := materialize.GraphFromIndex(index)

	tierMap := make(map[string][]ReadyEntry)
	priorityOrder := []string{"critical", "high", "medium", "low", ""}

	for _, entry := range entries {
		tier := entry.Priority
		tierMap[tier] = append(tierMap[tier], entry)
	}

	var allWaves [][]ReadyEntry

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

	for _, tier := range orderedTiers {
		tierEntries, ok := tierMap[tier]
		if !ok || len(tierEntries) == 0 {
			continue
		}

		sortByConflictDegree(tierEntries)

		var tierWaves [][]ReadyEntry

		for _, candidate := range tierEntries {
			placed := false

			for waveIdx := range tierWaves {
				if canAddToWave(candidate, tierWaves[waveIdx], graph) {
					tierWaves[waveIdx] = append(tierWaves[waveIdx], candidate)
					placed = true
					break
				}
			}

			if !placed {
				tierWaves = append(tierWaves, []ReadyEntry{candidate})
			}
		}

		allWaves = append(allWaves, tierWaves...)
	}

	return allWaves
}

func sortByConflictDegree(entries []ReadyEntry) {
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

	orderByConflictDegree := func(i, j int) bool {
		di := conflictDegrees[entries[i].Issue]
		dj := conflictDegrees[entries[j].Issue]
		if di != dj {
			return di > dj
		}
		return entries[i].Issue < entries[j].Issue
	}

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

func canAddToWave(candidate ReadyEntry, wave []ReadyEntry, graph *dag.Graph) bool {
	for _, existing := range wave {
		if claim.ScopesOverlap(candidate.Scope, existing.Scope) {
			return false
		}

		if claim.IsAncestorOrDescendant(graph, candidate.Issue, existing.Issue) {
			return false
		}
	}
	return true
}
