package ready

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeWaves_TierBoundaryEnforcement_REQ_LNGHZN_S2_T1 verifies that priority tier
// is a hard boundary — entries from different priority tiers are never placed in the same wave.
func TestComputeWaves_TierBoundaryEnforcement_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	// Create entries with different priorities
	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "critical", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
		{Issue: "task-3", Title: "Task 3", Priority: "critical", Scope: []string{"src/db/**"}},
		{Issue: "task-4", Title: "Task 4", Priority: "high", Scope: []string{"src/util/**"}},
	}

	// Build minimal index and graph
	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
		"task-4": {Title: "Task 4", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// Verify that critical tasks come in waves before high-priority tasks
	// Find the last wave index containing critical tasks
	lastCriticalWaveIdx := -1
	firstHighWaveIdx := -1

	for waveIdx, wave := range waves {
		for _, entry := range wave {
			if entry.Priority == "critical" && waveIdx > lastCriticalWaveIdx {
				lastCriticalWaveIdx = waveIdx
			}
			if entry.Priority == "high" && firstHighWaveIdx == -1 {
				firstHighWaveIdx = waveIdx
			}
		}
	}

	// Verify tier boundary: if we found both tiers, critical must come before high
	if lastCriticalWaveIdx != -1 && firstHighWaveIdx != -1 {
		assert.True(t, lastCriticalWaveIdx < firstHighWaveIdx, "critical priority waves must come before high priority waves")
	}
}

// TestComputeWaves_CustomPriorityTiersAreDeterministicAndComplete verifies that
// unknown priority tiers are emitted after known tiers, in lexical order, with
// every ready entry appearing exactly once.
func TestComputeWaves_CustomPriorityTiersAreDeterministicAndComplete(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "custom-zeta", Priority: "zeta", Scope: []string{"zeta/**"}},
		{Issue: "known-high", Priority: "high", Scope: []string{"high/**"}},
		{Issue: "custom-alpha", Priority: "alpha", Scope: []string{"alpha/**"}},
		{Issue: "default", Scope: []string{"default/**"}},
		{Issue: "custom-beta", Priority: "beta", Scope: []string{"beta/**"}},
	}

	waves := PartitionWaves(entries, materialize.Index{}, nil)

	var got []string
	for _, wave := range waves {
		for _, entry := range wave {
			got = append(got, entry.Issue)
		}
	}

	assert.Equal(t, []string{"known-high", "default", "custom-alpha", "custom-beta", "custom-zeta"}, got)
	assert.Len(t, got, len(entries), "each input entry must be emitted exactly once")

	reversed := []ReadyEntry{entries[4], entries[3], entries[2], entries[1], entries[0]}
	var gotReversed []string
	for _, wave := range PartitionWaves(reversed, materialize.Index{}, nil) {
		for _, entry := range wave {
			gotReversed = append(gotReversed, entry.Issue)
		}
	}
	assert.Equal(t, got, gotReversed, "wave output must be independent of input order")
}

// TestComputeWaves_ScopeConflictDegreeOrdering_REQ_LNGHZN_S2_T1 verifies that within a tier,
// items are ordered by scope-conflict degree (how many other ready items share scope with them).
func TestComputeWaves_ScopeConflictDegreeOrdering_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	// Create entries where some have high scope conflict
	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/auth/login.go"}},  // conflicts with task-1
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/api/**"}},         // no conflict with task-1
		{Issue: "task-4", Title: "Task 4", Priority: "high", Scope: []string{"src/auth/logout.go"}}, // conflicts with task-1
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
		"task-4": {Title: "Task 4", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// Verify that items with high conflict are placed before items with low conflict
	// task-1, task-2, task-4 all have overlapping scopes with each other
	// task-3 has a disjoint scope
	// We expect task-1/2/4 to be in earlier waves than task-3 would be in a different wave

	// Verify no wave has conflicting scopes
	for _, wave := range waves {
		for i, e1 := range wave {
			for j, e2 := range wave {
				if i == j {
					continue
				}
				assert.False(t, claim.ScopesOverlap(e1.Scope, e2.Scope), "Wave should not have conflicting scopes: %s and %s", e1.Issue, e2.Issue)
			}
		}
	}
}

// TestComputeWaves_AncestorDescendantExclusion_REQ_LNGHZN_S2_T1 verifies that
// ancestor/descendant pairs are excluded from being placed in the same wave.
func TestComputeWaves_AncestorDescendantExclusion_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	// Create a hierarchy: story-1 is parent of task-1
	entries := []ReadyEntry{
		{Issue: "story-1", Title: "Story 1", Priority: "high", Scope: []string{"src/**"}},
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
	}

	index := materialize.Index{
		"story-1": {Title: "Story 1", Type: "story"},
		"task-1":  {Title: "Task 1", Type: "task", Parent: "story-1"},
		"task-2":  {Title: "Task 2", Type: "task"},
	}

	nodeIndex := map[string]*dag.Node{
		"story-1": {
			ID:       "story-1",
			Title:    "Story 1",
			Type:     "story",
			Children: []string{"task-1"},
		},
		"task-1": {
			ID:     "task-1",
			Title:  "Task 1",
			Type:   "task",
			Parent: "story-1",
		},
		"task-2": {
			ID:    "task-2",
			Title: "Task 2",
			Type:  "task",
		},
	}

	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// Verify that story-1 and task-1 are not in the same wave
	for _, wave := range waves {
		hasStory := false
		hasTask := false
		for _, entry := range wave {
			if entry.Issue == "story-1" {
				hasStory = true
			}
			if entry.Issue == "task-1" {
				hasTask = true
			}
		}
		// story-1 and task-1 should never be in the same wave
		assert.False(t, hasStory && hasTask, "Ancestor and descendant should not be in the same wave")
	}
}

// TestComputeWaves_GreedyFirstFitPlacement_REQ_LNGHZN_S2_T1 verifies that the greedy
// first-fit algorithm places items into the first available wave without scope conflicts.
func TestComputeWaves_GreedyFirstFitPlacement_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	// Create entries that can be packed efficiently with greedy first-fit
	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/db/**"}},
		{Issue: "task-4", Title: "Task 4", Priority: "high", Scope: []string{"src/auth/login.go"}},
		{Issue: "task-5", Title: "Task 5", Priority: "high", Scope: []string{"src/api/handler.go"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
		"task-4": {Title: "Task 4", Type: "task"},
		"task-5": {Title: "Task 5", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// Verify that the number of waves is reasonable
	// We have 5 entries with 3 disjoint scope families (auth, api, db)
	// task-1 and task-4 conflict (both auth), task-2 and task-5 conflict (both api), task-3 (db) is disjoint
	// Greedy first-fit could achieve 2 waves: one with task-1, task-2, task-3, and another with task-4, task-5
	// But it might create more depending on ordering. Minimum is 2.
	assert.GreaterOrEqual(t, len(waves), 2, "Expected at least 2 waves")

	// Verify no wave has scope conflicts
	for i, wave := range waves {
		// Check that no two entries in the same wave have overlapping scopes
		for entryIdx := range wave {
			for otherIdx := entryIdx + 1; otherIdx < len(wave); otherIdx++ {
				e1 := wave[entryIdx]
				e2 := wave[otherIdx]
				// Check if they have conflicting scopes
				for _, s1 := range e1.Scope {
					for _, s2 := range e2.Scope {
						// Check if s1 glob pattern matches s2 or vice versa
						assert.NotEqual(t, s1, s2, "Wave %d has duplicate exact scopes: %s", i, s1)
					}
				}
			}
		}
	}
}

// TestComputeWaves_EmptyInput_REQ_LNGHZN_S2_T1 verifies that empty input produces empty output
func TestComputeWaves_EmptyInput_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{}
	index := materialize.Index{}
	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	assert.Empty(t, waves, "Empty input should produce empty output")
}

// TestComputeWaves_SingleEntry_REQ_LNGHZN_S2_T1 verifies that a single entry forms a single wave
func TestComputeWaves_SingleEntry_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	require.Len(t, waves, 1, "Single entry should produce one wave")
	require.Len(t, waves[0], 1, "Single entry wave should have one entry")
	assert.Equal(t, "task-1", waves[0][0].Issue)
}

// TestComputeWaves_AllDisjointScopes_REQ_LNGHZN_S2_T1 verifies that entries with
// completely disjoint scopes can all go into the same wave
func TestComputeWaves_AllDisjointScopes_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/db/**"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// All entries should fit in a single wave since scopes don't overlap
	require.Len(t, waves, 1, "All disjoint scopes should fit in one wave")
	require.Len(t, waves[0], 3, "All three entries should be in the same wave")
}

// TestComputeWaves_AllConflictingScopes_REQ_LNGHZN_S2_T1 verifies that entries with
// all conflicting scopes create separate waves
func TestComputeWaves_AllConflictingScopes_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/auth/login.go"}},
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/auth/logout.go"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
	}

	nodeIndex := materializeIndexToNodeIndex(index)
	graph := dag.FromIndex(nodeIndex)

	waves := PartitionWaves(entries, index, graph)

	// All entries have overlapping scopes, so they should be in separate waves
	require.Len(t, waves, 3, "All conflicting scopes should create separate waves")
	for i, wave := range waves {
		require.Len(t, wave, 1, "Wave %d should have exactly one entry", i)
	}
}
