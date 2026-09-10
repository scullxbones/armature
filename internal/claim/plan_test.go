package claim_test

import (
	"slices"
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/dag"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubGraph struct {
	descendants map[string][]string
}

func (g stubGraph) Descendants(id string) []string {
	if g.descendants == nil {
		return nil
	}
	return g.descendants[id]
}

func baseInput() claim.PlanInput {
	return claim.PlanInput{
		TargetID:    "task-target",
		TargetScope: []string{"src/auth/**"},
		WorkerID:    "worker-a",
		Graph:       stubGraph{},
		Issues:      map[string]claim.IssueFacts{},
	}
}

func overlapTask(title, status, claimedBy string, scope ...string) claim.IssueFacts {
	if len(scope) == 0 {
		scope = []string{"src/auth/login.go"}
	}
	return claim.IssueFacts{
		Type:      "task",
		Status:    status,
		ClaimedBy: claimedBy,
		Title:     title,
		Scope:     scope,
	}
}

func TestPlanClaim_InvalidInput_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	graph := stubGraph{}
	tests := []struct {
		name string
		in   claim.PlanInput
	}{
		{
			name: "empty target id",
			in: claim.PlanInput{
				TargetID: "", WorkerID: "worker-a", Graph: graph,
			},
		},
		{
			name: "empty worker id",
			in: claim.PlanInput{
				TargetID: "task-target", WorkerID: "", Graph: graph,
			},
		},
		{
			name: "nil graph",
			in: claim.PlanInput{
				TargetID: "task-target", WorkerID: "worker-a", Graph: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan, err := claim.PlanClaim(tt.in)
			require.Error(t, err)
			assert.Empty(t, plan.BlockReasons)
			assert.Empty(t, plan.Warnings)
			assert.Empty(t, plan.Notes)
		})
	}
}

func TestPlanClaim_NilCollectionsEmptyPlan_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := claim.PlanInput{
		TargetID:    "task-target",
		TargetScope: []string{"src/auth/**"},
		WorkerID:    "worker-a",
		Graph:       stubGraph{},
		Issues:      nil,
		PriorOps:    nil,
	}
	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Empty(t, plan.Notes)
}

func TestPlanClaim_IgnoresNonCompetitors_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-target": overlapTask("Self", ops.StatusClaimed, "worker-b"),
		"story-01": {
			Type: "story", Status: ops.StatusInProgress, ClaimedBy: "worker-b",
			Title: "Parent Story", Scope: []string{"src/auth/**"},
		},
		"task-open":   overlapTask("Open Task", ops.StatusOpen, "worker-b"),
		"task-done":   overlapTask("Done Task", ops.StatusDone, "worker-b"),
		"task-merged": overlapTask("Merged Task", ops.StatusMerged, "worker-b"),
		"bug-claimed": {
			Type: "bug", Status: ops.StatusClaimed, ClaimedBy: "worker-b",
			Title: "A Bug", Scope: []string{"src/auth/login.go"},
		},
		"task-disjoint": overlapTask("Other Dir", ops.StatusClaimed, "worker-b", "src/billing/**"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Empty(t, plan.Notes)
}

func TestPlanClaim_ActiveTasksCompete_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-claimed": overlapTask("Claimed Work", ops.StatusClaimed, "worker-b"),
		"task-wip":     overlapTask("In Progress Work", ops.StatusInProgress, "worker-c"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scope overlap with task-claimed (Claimed Work), a task claimed held by worker-b",
		"scope overlap with task-wip (In Progress Work), a task in-progress held by worker-c",
	}, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Empty(t, plan.Notes)
}

func TestPlanClaim_ExcludesAncestorDescendant_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	nodes := map[string]*dag.Node{
		"story-01": {
			ID: "story-01", Title: "Parent Story", Type: "story",
			Children: []string{"task-target", "task-sibling"},
		},
		"task-target": {
			ID: "task-target", Title: "Child Task", Type: "task",
			Parent: "story-01", Children: []string{},
		},
		"task-child": {
			ID: "task-child", Title: "Grandchild", Type: "task",
			Parent: "task-target", Children: []string{},
		},
		"task-sibling": {
			ID: "task-sibling", Title: "Sibling", Type: "task",
			Parent: "story-01", Children: []string{},
		},
	}
	nodes["task-target"].Children = []string{"task-child"}
	graph := dag.FromIndex(nodes)

	in := claim.PlanInput{
		TargetID:    "task-target",
		TargetScope: []string{"src/**"},
		WorkerID:    "worker-a",
		Graph:       graph,
		Issues: map[string]claim.IssueFacts{
			"task-child":   overlapTask("Grandchild", ops.StatusClaimed, "worker-b", "src/auth/**"),
			"task-sibling": overlapTask("Sibling", ops.StatusClaimed, "worker-c", "src/auth/**"),
		},
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scope overlap with task-sibling (Sibling), a task claimed held by worker-c",
	}, plan.BlockReasons)
	assert.Empty(t, plan.Notes)
}

func TestPlanClaim_SameWorkerDismissalAndDedup_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-mine": overlapTask("Mine", ops.StatusClaimed, "worker-a"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Equal(t, []claim.NoteIntent{{
		IssueID: "task-target",
		Message: "Serial claim: scope overlap with task-mine (same worker, dismissed)",
	}}, plan.Notes)

	in.PriorOps = []ops.Op{{
		Type: ops.OpNote, TargetID: "task-target", WorkerID: "worker-a",
		Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-mine (same worker, dismissed)"},
	}}
	plan, err = claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.Notes)

	in.PriorOps = []ops.Op{{
		Type: ops.OpNote, TargetID: "task-target", WorkerID: "worker-other",
		Payload: ops.Payload{Msg: "Serial claim: scope overlap with task-mine (same worker, dismissed)"},
	}}
	plan, err = claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []claim.NoteIntent{{
		IssueID: "task-target",
		Message: "Serial claim: scope overlap with task-mine (same worker, dismissed)",
	}}, plan.Notes)
}

func TestPlanClaim_UnknownClaimedBy_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-orphan": overlapTask("Orphan", ops.StatusClaimed, ""),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scope overlap with task-orphan (Orphan), a task claimed held by unknown",
	}, plan.BlockReasons)
}

func TestPlanClaim_CollectsSortedForeignBlocks_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-z": overlapTask("Zed", ops.StatusClaimed, "worker-z"),
		"task-m": overlapTask("Mid", ops.StatusInProgress, "worker-m"),
		"task-a": overlapTask("Aye", ops.StatusClaimed, "worker-x"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scope overlap with task-a (Aye), a task claimed held by worker-x",
		"scope overlap with task-m (Mid), a task in-progress held by worker-m",
		"scope overlap with task-z (Zed), a task claimed held by worker-z",
	}, plan.BlockReasons)
}

func TestPlanClaim_BlockedMixedProducesNoNotes_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Issues = map[string]claim.IssueFacts{
		"task-mine":    overlapTask("Mine", ops.StatusClaimed, "worker-a"),
		"task-foreign": overlapTask("Theirs", ops.StatusClaimed, "worker-b"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"scope overlap with task-foreign (Theirs), a task claimed held by worker-b",
	}, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Empty(t, plan.Notes)
}

func TestPlanClaim_ForceWarningsAndReciprocalNotes_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Force = true
	in.Issues = map[string]claim.IssueFacts{
		"task-z": overlapTask("Zed", ops.StatusClaimed, "worker-z"),
		"task-a": overlapTask("Aye", ops.StatusInProgress, "worker-x"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.BlockReasons)
	assert.Equal(t, []string{
		"scope overlap with task-a (Aye), a task in-progress held by worker-x",
		"scope overlap with task-z (Zed), a task claimed held by worker-z",
	}, plan.Warnings)
	assert.Equal(t, []claim.NoteIntent{
		{IssueID: "task-target", Message: "Scope overlap with task-a detected at claim time"},
		{IssueID: "task-a", Message: "Scope overlap with task-target detected at claim time"},
		{IssueID: "task-target", Message: "Scope overlap with task-z detected at claim time"},
		{IssueID: "task-z", Message: "Scope overlap with task-target detected at claim time"},
	}, plan.Notes)
}

func TestPlanClaim_SameWorkerDismissalUnderForce_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Force = true
	in.Issues = map[string]claim.IssueFacts{
		"task-mine": overlapTask("Mine", ops.StatusClaimed, "worker-a"),
	}

	plan, err := claim.PlanClaim(in)
	require.NoError(t, err)
	assert.Empty(t, plan.BlockReasons)
	assert.Empty(t, plan.Warnings)
	assert.Equal(t, []claim.NoteIntent{{
		IssueID: "task-target",
		Message: "Serial claim: scope overlap with task-mine (same worker, dismissed)",
	}}, plan.Notes)
}

func TestPlanClaim_MapOrderInvariant_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	mk := func(order []string) claim.PlanInput {
		in := baseInput()
		in.Force = true
		in.Issues = make(map[string]claim.IssueFacts, len(order))
		for _, id := range order {
			in.Issues[id] = overlapTask(id, ops.StatusClaimed, "worker-b")
		}
		return in
	}

	planA, err := claim.PlanClaim(mk([]string{"task-c", "task-a", "task-b"}))
	require.NoError(t, err)
	planB, err := claim.PlanClaim(mk([]string{"task-a", "task-b", "task-c"}))
	require.NoError(t, err)
	planC, err := claim.PlanClaim(mk([]string{"task-b", "task-c", "task-a"}))
	require.NoError(t, err)

	assert.Equal(t, planA, planB)
	assert.Equal(t, planA, planC)
	assert.Equal(t, []string{"task-a", "task-b", "task-c"}, warningIDs(planA.Warnings))
}

func TestPlanClaim_InputsUnchanged_REQ_ARCHIMP_S20_T1(t *testing.T) {
	t.Parallel()

	in := baseInput()
	in.Force = true
	in.TargetScope = []string{"src/auth/**", "src/billing/**"}
	in.Issues = map[string]claim.IssueFacts{
		"task-mine":    overlapTask("Mine", ops.StatusClaimed, "worker-a"),
		"task-foreign": overlapTask("Theirs", ops.StatusClaimed, "worker-b"),
	}
	in.PriorOps = []ops.Op{{
		Type: ops.OpNote, TargetID: "task-target", WorkerID: "worker-a",
		Payload: ops.Payload{Msg: "unrelated"},
	}}

	scopeBefore := slices.Clone(in.TargetScope)
	mineScopeBefore := slices.Clone(in.Issues["task-mine"].Scope)
	foreignScopeBefore := slices.Clone(in.Issues["task-foreign"].Scope)
	opsBefore := slices.Clone(in.PriorOps)
	issuesLenBefore := len(in.Issues)
	targetID, workerID, force := in.TargetID, in.WorkerID, in.Force
	graph := in.Graph

	_, err := claim.PlanClaim(in)
	require.NoError(t, err)

	assert.Equal(t, targetID, in.TargetID)
	assert.Equal(t, workerID, in.WorkerID)
	assert.Equal(t, force, in.Force)
	assert.Equal(t, graph, in.Graph)
	assert.Equal(t, scopeBefore, in.TargetScope)
	assert.Equal(t, issuesLenBefore, len(in.Issues))
	assert.Equal(t, mineScopeBefore, in.Issues["task-mine"].Scope)
	assert.Equal(t, foreignScopeBefore, in.Issues["task-foreign"].Scope)
	assert.Equal(t, opsBefore, in.PriorOps)
}

func warningIDs(warnings []string) []string {
	ids := make([]string, 0, len(warnings))
	for _, w := range warnings {
		// "scope overlap with {id} (...)"
		rest := w[len("scope overlap with "):]
		for i, r := range rest {
			if r == ' ' {
				ids = append(ids, rest[:i])
				break
			}
		}
	}
	return ids
}
