package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMixedD14Tree builds (legal hierarchy: story→feature→task):
//
//	E (merged epic)
//	├── LF (done leaf; legacy — no claim/delivery provenance)
//	├── R (done story)
//	│   ├── RC1 (done; legacy)
//	│   ├── RC2 (open — inconsistent under merged E)
//	│   ├── RC3 (merged)
//	│   └── RS (done feature)
//	│       └── RSC (done; legacy)
//	├── AM (rollup-merged when AMC merged)
//	│   └── AMC (merged)
//	└── AC (done story; all-cancelled child — will not roll up)
//	    └── ACC (cancelled)
//
// Plus G2/P2/OS all-cancelled shape (Codex P2):
//
//	G2 (merged epic) → P2 (done) → OS (cancelled)
func setupMixedD14Tree(t *testing.T) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	type createSpec struct {
		id, title, typ, parent string
	}
	creates := []createSpec{
		{"E", "Epic E", "epic", ""},
		{"LF", "Leaf LF", "task", "E"},
		{"R", "Story R", "story", "E"},
		{"RC1", "Child RC1", "task", "R"},
		{"RC2", "Child RC2", "task", "R"},
		{"RC3", "Child RC3", "task", "R"},
		{"RS", "Feature RS", "feature", "R"},
		{"RSC", "Child RSC", "task", "RS"},
		{"AM", "Story AM", "story", "E"},
		{"AMC", "Child AMC", "task", "AM"},
		{"AC", "Story AC", "story", "E"},
		{"ACC", "Child ACC", "task", "AC"},
		{"G2", "Epic G2", "epic", ""},
		{"P2", "Story P2", "story", "G2"},
		{"OS", "Child OS", "task", "P2"},
	}
	for _, c := range creates {
		payload := ops.Payload{
			Title: c.title, NodeType: c.typ, Parent: c.parent,
			DefinitionOfDone: c.title + " is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
			Scope:            []string{"cmd/armature/" + strings.ToLower(c.id) + ".go"},
		}
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpCreate, TargetID: c.id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: payload,
		}))
	}

	outcome := "fixture done for mixed-tree remediation class"
	for _, id := range []string{"LF", "RC1", "RSC", "RS", "R", "AM", "AC", "P2"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusDone, Outcome: outcome},
		}))
	}
	for _, id := range []string{"RC3", "AMC", "ACC", "OS"} {
		to := ops.StatusMerged
		if id == "ACC" || id == "OS" {
			to = ops.StatusCancelled
		}
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: to},
		}))
	}
	for _, id := range []string{"E", "G2"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusMerged},
		}))
	}
	// RC2 stays open.

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	return repo
}

func TestMixedTreeRemediationRepairsD14_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupMixedD14Tree(t)

	amStatus, err := runTrls(t, repo, "show", "--field", "status", "AM")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", amStatus, "RunRollup must auto-merge AM when AMC is merged")

	// Ancestors intentionally reopened by tree repair (E/G2) are not protected.
	// Merged siblings / roll-ups must stay merged.
	protectedMerged := map[string]string{
		"RC3": "merged",
		"AM":  "merged",
		"AMC": "merged",
	}

	d14 := doctorD14Explain(t, repo)
	require.Equal(t, "warning", d14.severity)
	assert.Contains(t, d14.explanation, promotion.TreeRepairGuidance)
	require.NotEmpty(t, d14.items)

	// Full doctor suggested sequence on a fresh fixture must clear every D14 item
	// it claims to address — exit 0 alone is not enough.
	for _, item := range d14.items {
		id := d14ItemID(item)
		t.Run("repair_"+id, func(t *testing.T) {
			t.Parallel()
			r := setupMixedD14Tree(t)
			acts := promotion.RemediationsForIssue(id, loadPromotionGraph(t, r))
			require.NotEmpty(t, acts, "D14 item %s must have remediations", item)
			assert.NotContains(t, acts, "arm doctor")
			err := applyRepairSequence(t, r, acts)
			require.NoError(t, err, "sequence for %s: %v", item, acts)
			after := doctorD14Explain(t, r)
			assert.NotContains(t, after.items, item,
				"full suggested sequence must clear D14 item %s; remaining=%v cmds=%v", item, after.items, acts)
			for mid, want := range protectedMerged {
				st, showErr := runTrls(t, r, "show", "--field", "status", mid)
				require.NoError(t, showErr)
				assert.Equal(t, want+"\n", st,
					"repair must not disturb protected merged node %s", mid)
			}
		})
	}

	// MERGED-1 on R: next_actions must be runnable repair set (no claim under merged).
	mergedOut := new(bytes.Buffer)
	code := executeThenHandleRootError(t, mergedOut, new(bytes.Buffer),
		"merged", "--repo", repo, "--issue", "R", "--force", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, mergedOut.String())
	assert.Equal(t, codeMerged1, cf.Code)
	require.NotEmpty(t, cf.NextActions)
	assert.Contains(t, cf.NextActions, "arm reopen --issue E")
	assert.NotContains(t, cf.NextActions, "arm claim --issue RC2")
	for _, action := range cf.NextActions {
		if promotion.IsRunnableArmCommand(action) {
			assert.NotContains(t, action, "; ")
		}
		assert.NotEqual(t, "arm merged --issue R", action)
	}
}

func TestRepairHarnessRejectsUnmergeAndCancelWithoutForce_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	t.Run("reopen_merged_child_fails_harness", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		err := applyRepairSequence(t, repo, []string{"arm reopen --issue RC3"})
		require.NoError(t, err, "reopen RC3 may exit 0")
		after := doctorD14Explain(t, repo)
		assert.Contains(t, after.items, "R -> E",
			"un-merging RC3 must not count as clearing D14 R→E")
		st, showErr := runTrls(t, repo, "show", "--field", "status", "RC3")
		require.NoError(t, showErr)
		assert.NotEqual(t, "merged\n", st, "mutation un-merged RC3 — harness must treat as bad")
		// Explicit harness check used by production test path:
		err = assertD14ClearedWithoutDamagingMerged(t, repo, "R -> E", map[string]string{"RC3": "merged"})
		require.Error(t, err, "harness must FAIL when suggested cmd un-merges RC3")
	})

	t.Run("cancel_without_force_fails_harness", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		err := applyRepairSequence(t, repo, []string{"arm transition --issue RC2 --to cancelled"})
		// May fail exit≠0 or succeed without clearing R→E; either way harness fails.
		after := doctorD14Explain(t, repo)
		if !containsStr(after.items, "R -> E") && err == nil {
			t.Fatalf("unexpected: cancel without --force cleared D14 and succeeded")
		}
		err = assertD14ClearedWithoutDamagingMerged(t, repo, "R -> E", map[string]string{"RC3": "merged", "AM": "merged"})
		require.Error(t, err, "harness must FAIL for cancel without --force / partial repair")
	})

	t.Run("cancel_RC2_alone_is_partial", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		require.NoError(t, applyRepairSequence(t, repo, []string{
			"arm transition --issue RC2 --to cancelled --force",
		}))
		err := assertD14ClearedWithoutDamagingMerged(t, repo, "R -> E", map[string]string{"RC3": "merged"})
		require.Error(t, err, "cancel RC2 alone leaves R→E; harness must FAIL partial repair")
	})
}

func TestFilledDeliverySequenceNamesReview_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// Harness must NOT inject unprinted attestation. Suggested sequence must
	// name review record when SHAs are knowable.
	repo := doneWithoutAssessment(t)
	base, tip := landDelivery(t, repo, "task-01")
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	g := promotion.Graph{
		"task-01": {Status: ops.StatusDone, Base: base, Tip: tip},
	}
	acts := promotion.RemediationsForIssue("task-01", g)
	require.Equal(t, []string{
		delivery.RecordArgvFilled("task-01", base, tip),
		"arm review record --issue task-01 --assessment <assessment.json>",
	}, acts)
	assert.False(t, promotion.IsDeliveryRecordTemplate(acts[0]))
	assert.True(t, promotion.IsLegacyTemplateRemediation(acts[1]))

	// Run only the printed filled delivery argv — no silent attest.
	require.NoError(t, applyRepairSequence(t, repo, []string{acts[0]}))
	status, err := runTrls(t, repo, "show", "--field", "status", "task-01")
	require.NoError(t, err)
	assert.Contains(t, status, "done", "delivery alone stays done; merge needs printed review record step")
}

func TestAllCancelledP2UnderG2_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupMixedD14Tree(t)
	d14 := doctorD14Explain(t, repo)
	assert.Contains(t, d14.items, "P2 -> G2")

	acts := promotion.RemediationsForIssue("P2", loadPromotionGraph(t, repo))
	assert.Contains(t, acts, "arm transition --issue P2 --to cancelled --force")
	assert.Contains(t, acts, "arm reopen --issue G2")

	r := setupMixedD14Tree(t)
	require.NoError(t, applyRepairSequence(t, r, acts))
	after := doctorD14Explain(t, r)
	assert.NotContains(t, after.items, "P2 -> G2")
	g2, err := runTrls(t, r, "show", "--field", "status", "G2")
	require.NoError(t, err)
	// Either reopen G2 or cancel P2 — both clear D14; G2 may stay merged if cancel P2 ran first alphabetically...
	// applyRepairSequence runs in list order (not sorted). treeRepair lists reopen then cancel.
	_ = g2
}

type d14Explain struct {
	severity    string
	explanation string
	items       []string
	suggested   []string
}

func doctorD14Explain(t *testing.T, repo string) d14Explain {
	t.Helper()
	stdout := new(bytes.Buffer)
	_ = executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"doctor", "--repo", repo, "--explain", "--format", "json")
	payload := assertSingleJSONObject(t, stdout.String())
	checks, ok := payload["checks"].([]any)
	require.True(t, ok)
	for _, raw := range checks {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		if row["check"] != "D14" {
			continue
		}
		out := d14Explain{severity: fmt.Sprint(row["severity"])}
		if expl, ok := row["explanation"].(string); ok {
			out.explanation = expl
		}
		if items, ok := row["items"].([]any); ok {
			for _, it := range items {
				out.items = append(out.items, fmt.Sprint(it))
			}
		}
		if sug, ok := row["suggested"].([]any); ok {
			for _, s := range sug {
				out.suggested = append(out.suggested, fmt.Sprint(s))
			}
		}
		return out
	}
	t.Fatalf("D14 check missing; out=%s", stdout.String())
	return d14Explain{}
}

func d14ItemID(item string) string {
	id, _, ok := strings.Cut(item, " -> ")
	if !ok || id == "" {
		return item
	}
	return id
}

func loadPromotionGraph(t *testing.T, repo string) promotion.Graph {
	t.Helper()
	ctx := getTestContext(t, repo)
	stateDir := getTestStateDir(t, repo)
	store := snapshot.NewStore(filepath.Join(ctx.IssuesDir, "ops"), stateDir)
	index, err := store.ReadIndex()
	require.NoError(t, err)
	require.NotEmpty(t, index, "expected materialized index at %s", store.IndexPath())
	return promotionGraphFromStore(store, index)
}

// applyRepairSequence runs suggested cmds exactly as printed (harness adds only
// --repo/--format). Guidance and legacy templates are not executed.
func applyRepairSequence(t *testing.T, repo string, cmds []string) error {
	t.Helper()
	var firstErr error
	for _, cmdLine := range cmds {
		if promotion.IsGuidanceRemediation(cmdLine) {
			continue
		}
		if promotion.IsLegacyTemplateRemediation(cmdLine) {
			// Labeled legacy/template — same honesty as kind "legacy"; do not substitute.
			continue
		}
		if !promotion.IsRunnableArmCommand(cmdLine) {
			return fmt.Errorf("non-arm remediation %q", cmdLine)
		}
		args := strings.Fields(cmdLine)
		if len(args) < 2 || args[0] != "arm" {
			return fmt.Errorf("bad argv %q", cmdLine)
		}
		cliArgs := append([]string{}, args[1:]...)
		cliArgs = append(cliArgs, "--repo", repo, "--format", "agent")
		out := new(bytes.Buffer)
		code := executeThenHandleRootError(t, out, new(bytes.Buffer), cliArgs...)
		if code != 0 && firstErr == nil {
			firstErr = fmt.Errorf("suggested %q exit %d; out=%s", cmdLine, code, out.String())
		}
	}
	return firstErr
}

func assertD14ClearedWithoutDamagingMerged(t *testing.T, repo, item string, protected map[string]string) error {
	t.Helper()
	after := doctorD14Explain(t, repo)
	if containsStr(after.items, item) {
		return fmt.Errorf("D14 item %s still present; items=%v", item, after.items)
	}
	for id, want := range protected {
		st, err := runTrls(t, repo, "show", "--field", "status", id)
		if err != nil {
			return err
		}
		if st != want+"\n" {
			return fmt.Errorf("protected %s status=%q want %s", id, strings.TrimSpace(st), want)
		}
	}
	return nil
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
