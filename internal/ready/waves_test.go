package ready

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeWaves_TierBoundaryEnforcement_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "critical", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
		{Issue: "task-3", Title: "Task 3", Priority: "critical", Scope: []string{"src/db/**"}},
		{Issue: "task-4", Title: "Task 4", Priority: "high", Scope: []string{"src/util/**"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
		"task-4": {Title: "Task 4", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

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

	if lastCriticalWaveIdx != -1 && firstHighWaveIdx != -1 {
		assert.True(t, lastCriticalWaveIdx < firstHighWaveIdx, "critical priority waves must come before high priority waves")
	}
}

func TestComputeWaves_CustomPriorityTiersAreDeterministicAndComplete(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "custom-zeta", Priority: "zeta", Scope: []string{"zeta/**"}},
		{Issue: "known-high", Priority: "high", Scope: []string{"high/**"}},
		{Issue: "custom-alpha", Priority: "alpha", Scope: []string{"alpha/**"}},
		{Issue: "default", Scope: []string{"default/**"}},
		{Issue: "custom-beta", Priority: "beta", Scope: []string{"beta/**"}},
	}

	waves := PartitionWaves(entries, materialize.Index{})

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
	for _, wave := range PartitionWaves(reversed, materialize.Index{}) {
		for _, entry := range wave {
			gotReversed = append(gotReversed, entry.Issue)
		}
	}
	assert.Equal(t, got, gotReversed, "wave output must be independent of input order")
}

func TestComputeWaves_ScopeConflictDegreeOrdering_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/auth/login.go"}},
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/api/**"}},
		{Issue: "task-4", Title: "Task 4", Priority: "high", Scope: []string{"src/auth/logout.go"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
		"task-4": {Title: "Task 4", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

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

func TestComputeWaves_AncestorDescendantExclusion_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "story-1", Title: "Story 1", Priority: "high", Scope: []string{"src/**"}},
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/api/**"}},
	}

	index := materialize.Index{
		"story-1": {Title: "Story 1", Type: "story", Children: []string{"task-1"}},
		"task-1":  {Title: "Task 1", Type: "task", Parent: "story-1"},
		"task-2":  {Title: "Task 2", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

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
		assert.False(t, hasStory && hasTask, "Ancestor and descendant should not be in the same wave")
	}
}

func TestComputeWaves_GreedyFirstFitPlacement_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

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

	waves := PartitionWaves(entries, index)

	assert.GreaterOrEqual(t, len(waves), 2, "Expected at least 2 waves")

	for i, wave := range waves {
		for entryIdx := range wave {
			for otherIdx := entryIdx + 1; otherIdx < len(wave); otherIdx++ {
				e1 := wave[entryIdx]
				e2 := wave[otherIdx]
				for _, s1 := range e1.Scope {
					for _, s2 := range e2.Scope {
						assert.NotEqual(t, s1, s2, "Wave %d has duplicate exact scopes: %s", i, s1)
					}
				}
			}
		}
	}
}

func TestComputeWaves_EmptyInput_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{}
	index := materialize.Index{}
	waves := PartitionWaves(entries, index)

	assert.Empty(t, waves, "Empty input should produce empty output")
}

func TestComputeWaves_SingleEntry_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

	require.Len(t, waves, 1, "Single entry should produce one wave")
	require.Len(t, waves[0], 1, "Single entry wave should have one entry")
	assert.Equal(t, "task-1", waves[0][0].Issue)
}

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

	waves := PartitionWaves(entries, index)

	require.Len(t, waves, 1, "All disjoint scopes should fit in one wave")
	require.Len(t, waves[0], 3, "All three entries should be in the same wave")
}

func TestComputeWaves_AllConflictingScopes_REQ_LNGHZN_S2_T1(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/**"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/auth/login.go"}},
		{Issue: "task-3", Title: "Task 3", Priority: "high", Scope: []string{"src/auth/login.go"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
		"task-3": {Title: "Task 3", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

	require.Len(t, waves, 3, "All conflicting scopes should create separate waves")
	for i, wave := range waves {
		require.Len(t, wave, 1, "Wave %d should have exactly one entry", i)
	}
}

func TestComputeWaves_SiblingFilesInSameDirectoryShareAWave_REQ_LNGHZN_S10_T6(t *testing.T) {
	t.Parallel()

	entries := []ReadyEntry{
		{Issue: "task-1", Title: "Task 1", Priority: "high", Scope: []string{"src/auth/login.go"}},
		{Issue: "task-2", Title: "Task 2", Priority: "high", Scope: []string{"src/auth/logout.go"}},
	}

	index := materialize.Index{
		"task-1": {Title: "Task 1", Type: "task"},
		"task-2": {Title: "Task 2", Type: "task"},
	}

	waves := PartitionWaves(entries, index)

	require.Len(t, waves, 1, "distinct sibling files in the same directory must not conflict, so both entries share one wave")
	require.Len(t, waves[0], 2, "both entries should be in the single wave")
}
