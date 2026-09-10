package worktree_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/scullxbones/armature/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provisionBase() worktree.ProvisionInput {
	return worktree.ProvisionInput{
		IssueID:        "ARCHIMP-S20-T5",
		Dest:           "/repo/.worktrees/ARCHIMP-S20-T5",
		ExpectedBranch: "task/ARCHIMP-S20-T5",
	}
}

func boundRow(path, branch string) worktree.InventoryRow {
	return worktree.InventoryRow{
		Path:    path,
		Branch:  branch,
		Binding: "ARCHIMP-S20-T5",
	}
}

func heads(branch string) string {
	return "refs/heads/" + branch
}

func snapshot(in worktree.ProvisionInput) (worktree.ProvisionInput, []worktree.InventoryRow) {
	cloned := slices.Clone(in.Inventory)
	return in, cloned
}

func assertInputUnchanged(t *testing.T, before worktree.ProvisionInput, inventory []worktree.InventoryRow, after worktree.ProvisionInput) {
	t.Helper()
	assert.Equal(t, before, after, "PlanProvision must not mutate ProvisionInput fields")
	assert.Equal(t, inventory, after.Inventory, "PlanProvision must not reorder or rewrite Inventory")
}

func TestPlanProvision_RefuseNested_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.NestedUnder = "/repo/.worktrees/ARCHIMP-S20-T4"
	in.InRepo = true
	in.UnderCanonical = false
	in.Inventory = []worktree.InventoryRow{
		boundRow("/z", heads(in.ExpectedBranch)),
		boundRow("/a", heads(in.ExpectedBranch)),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Equal(t, fmt.Sprintf(
		"custom worktree destination %s is nested inside registered worktree %s",
		in.Dest, in.NestedUnder), plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_RefuseInRepoOutsideCanonical_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.Dest = "/repo/custom-wt"
	in.InRepo = true
	in.UnderCanonical = false
	in.Inventory = []worktree.InventoryRow{
		boundRow("/z", heads(in.ExpectedBranch)),
		boundRow("/a", heads(in.ExpectedBranch)),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Equal(t, fmt.Sprintf(
		"custom worktree destination %s is inside the repository; explicit destinations must be outside the repository or under canonical .worktrees",
		in.Dest), plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_RefuseAmbiguousBinding_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.Inventory = []worktree.InventoryRow{
		{Path: "/unrelated", Branch: heads("other"), Binding: "OTHER"},
		boundRow("/wt/z", heads(in.ExpectedBranch)),
		boundRow("/wt/a", heads(in.ExpectedBranch)),
		boundRow("/wt/m", heads(in.ExpectedBranch)),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Equal(t, "issue ARCHIMP-S20-T5 is bound to 3 worktrees (/wt/a, /wt/m, /wt/z); "+
		"remove the armature-issue-id binding from the ones you do not want before claiming",
		plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_RefuseWrongBranch_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()

	t.Run("named other branch", func(t *testing.T) {
		t.Parallel()
		in := provisionBase()
		in.ProvenanceOK = true
		in.Inventory = []worktree.InventoryRow{
			boundRow("/legacy/ARCHIMP-S20-T5", "refs/heads/scratch"),
		}
		before, inv := snapshot(in)

		plan, err := worktree.PlanProvision(in)
		require.NoError(t, err)
		assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
		assert.Equal(t, "worktree at /legacy/ARCHIMP-S20-T5 is bound to ARCHIMP-S20-T5 "+
			"but is on refs/heads/scratch, not task/ARCHIMP-S20-T5; "+
			"finish or abandon the in-progress git operation there and check out task/ARCHIMP-S20-T5 before claiming",
			plan.RefuseReason)
		assertInputUnchanged(t, before, inv, in)
	})

	t.Run("empty branch is detached", func(t *testing.T) {
		t.Parallel()
		in := provisionBase()
		in.Inventory = []worktree.InventoryRow{
			boundRow("/legacy/ARCHIMP-S20-T5", ""),
		}

		plan, err := worktree.PlanProvision(in)
		require.NoError(t, err)
		assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
		assert.Equal(t, "worktree at /legacy/ARCHIMP-S20-T5 is bound to ARCHIMP-S20-T5 "+
			"but is on detached HEAD, not task/ARCHIMP-S20-T5; "+
			"finish or abandon the in-progress git operation there and check out task/ARCHIMP-S20-T5 before claiming",
			plan.RefuseReason)
	})
}

func TestPlanProvision_RefuseAdoptWithoutProvenance_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.ProvenanceOK = false
	in.Inventory = []worktree.InventoryRow{
		boundRow("/legacy/ARCHIMP-S20-T5", heads(in.ExpectedBranch)),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Equal(t, "adopted worktree has no recorded branch-point provenance; "+
		"re-claim it from a managed worktree or use --skip-delivery-gate only with an explicit override",
		plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_Adopt_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.ProvenanceOK = true
	in.DestExists = true
	in.Inventory = []worktree.InventoryRow{
		{Path: "/repo/.worktrees/other", Branch: heads("task/other"), Binding: "OTHER"},
		boundRow("/legacy/ARCHIMP-S20-T5", heads(in.ExpectedBranch)),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionAdopt, plan.Action)
	assert.Equal(t, "/legacy/ARCHIMP-S20-T5", plan.AdoptFrom)
	assert.Empty(t, plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_AlreadyAtDest_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.ProvenanceOK = false
	in.Inventory = []worktree.InventoryRow{
		boundRow(in.Dest, "refs/heads/scratch"),
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionAlreadyAtDest, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Empty(t, plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)
}

func TestPlanProvision_Fresh_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	in := provisionBase()
	in.DestExists = true
	in.ProvenanceOK = false
	in.Inventory = []worktree.InventoryRow{
		{Path: "/repo/.worktrees/other", Branch: heads("task/other"), Binding: "OTHER"},
		{Path: "/scratch", Branch: heads(in.ExpectedBranch), Binding: ""},
	}
	before, inv := snapshot(in)

	plan, err := worktree.PlanProvision(in)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionFresh, plan.Action)
	assert.Empty(t, plan.AdoptFrom)
	assert.Empty(t, plan.RefuseReason)
	assertInputUnchanged(t, before, inv, in)

	nilInventory := provisionBase()
	plan, err = worktree.PlanProvision(nilInventory)
	require.NoError(t, err)
	assert.Equal(t, worktree.ProvisionFresh, plan.Action)
}

func TestPlanProvision_InventoryOrderInvariant_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	branch := heads("task/ARCHIMP-S20-T5")
	orders := [][]worktree.InventoryRow{
		{
			boundRow("/wt/z", branch),
			{Path: "/other", Branch: heads("other"), Binding: "OTHER"},
			boundRow("/wt/a", branch),
		},
		{
			boundRow("/wt/a", branch),
			boundRow("/wt/z", branch),
			{Path: "/other", Branch: heads("other"), Binding: "OTHER"},
		},
		{
			{Path: "/other", Branch: heads("other"), Binding: "OTHER"},
			boundRow("/wt/z", branch),
			boundRow("/wt/a", branch),
		},
	}

	var reasons []string
	for _, inv := range orders {
		in := provisionBase()
		in.Inventory = inv
		plan, err := worktree.PlanProvision(in)
		require.NoError(t, err)
		assert.Equal(t, worktree.ProvisionRefuse, plan.Action)
		reasons = append(reasons, plan.RefuseReason)
	}
	for i := 1; i < len(reasons); i++ {
		assert.Equal(t, reasons[0], reasons[i])
	}
	assert.Equal(t, "issue ARCHIMP-S20-T5 is bound to 2 worktrees (/wt/a, /wt/z); "+
		"remove the armature-issue-id binding from the ones you do not want before claiming",
		reasons[0])

	adoptOrders := [][]worktree.InventoryRow{
		{
			{Path: "/other", Branch: heads("other"), Binding: "OTHER"},
			boundRow("/legacy", branch),
		},
		{
			boundRow("/legacy", branch),
			{Path: "/other", Branch: heads("other"), Binding: "OTHER"},
		},
	}
	for _, inv := range adoptOrders {
		in := provisionBase()
		in.ProvenanceOK = true
		in.Inventory = inv
		plan, err := worktree.PlanProvision(in)
		require.NoError(t, err)
		assert.Equal(t, worktree.ProvisionAdopt, plan.Action)
		assert.Equal(t, "/legacy", plan.AdoptFrom)
	}
}

func TestPlanProvision_InvalidInput_REQ_ARCHIMP_S20_T5(t *testing.T) {
	t.Parallel()
	base := provisionBase()
	cases := []struct {
		name string
		in   worktree.ProvisionInput
	}{
		{name: "empty IssueID", in: worktree.ProvisionInput{Dest: base.Dest, ExpectedBranch: base.ExpectedBranch}},
		{name: "empty Dest", in: worktree.ProvisionInput{IssueID: base.IssueID, ExpectedBranch: base.ExpectedBranch}},
		{name: "empty ExpectedBranch", in: worktree.ProvisionInput{IssueID: base.IssueID, Dest: base.Dest}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan, err := worktree.PlanProvision(tc.in)
			require.Error(t, err)
			assert.Equal(t, worktree.ProvisionPlan{}, plan)
		})
	}
}
