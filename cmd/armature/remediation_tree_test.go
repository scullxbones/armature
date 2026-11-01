package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/scullxbones/armature/internal/review"
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

// setupNestedMergedAncestorTree: M1 (merged epic) → M2 (merged story) → D (done task).
func setupNestedMergedAncestorTree(t *testing.T) string {
	t.Helper()
	repo := gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)

	ctx := getTestContext(t, repo)
	workerID, logPath, err := resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	for _, c := range []struct {
		id, title, typ, parent string
	}{
		{"M1", "Epic M1", "epic", ""},
		{"M2", "Story M2", "story", "M1"},
		{"D", "Task D", "task", "M2"},
	} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpCreate, TargetID: c.id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{
				Title: c.title, NodeType: c.typ, Parent: c.parent,
				DefinitionOfDone: c.title + " is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "verified",
				Scope:            []string{"cmd/armature/" + strings.ToLower(c.id) + ".go"},
			},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "D", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusDone, Outcome: "fixture done for nested merged-ancestor chain"},
	}))
	for _, id := range []string{"M2", "M1"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusMerged},
		}))
	}
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

	protectedMerged := map[string]string{
		"RC3": "merged",
		"AM":  "merged",
		"AMC": "merged",
	}

	d14 := doctorD14Explain(t, repo)
	require.Equal(t, "warning", d14.severity)
	assert.Contains(t, d14.explanation, promotion.TreeRepairGuidance)
	require.NotEmpty(t, d14.items)
	// Suggested: finding header → Option A:/B: on their own lines → paste-runnable argv.
	require.True(t, strings.HasPrefix(d14.suggested[0], promotion.FindingHeaderPrefix), d14.suggested[0])
	assert.Contains(t, d14.suggested, "Option A:")
	assert.Contains(t, d14.suggested, "arm reopen --issue E")
	assert.NotContains(t, d14.suggested, "Option A: arm reopen --issue E",
		"Option label must be its own line, not an argv prefix")

	for _, item := range d14.items {
		id := promotion.FindingID(item)
		t.Run("repair_"+id, func(t *testing.T) {
			t.Parallel()
			plan := promotion.RemediationsPlan(id, loadPromotionGraph(t, setupMixedD14Tree(t)))
			require.NotEmpty(t, plan.Alternatives, "D14 item %s must have alternatives", item)
			require.GreaterOrEqual(t, len(plan.Alternatives), 2,
				"D14 repair must expose ≥2 Options; dropping either must fail harness")

			for i, alt := range plan.Alternatives {
				t.Run(fmt.Sprintf("option_%c", 'A'+i), func(t *testing.T) {
					t.Parallel()
					r := setupMixedD14Tree(t)
					before := loadPromotionGraph(t, r)
					require.NoError(t, applyAlternative(t, r, alt))
					after := loadPromotionGraph(t, r)

					scope := subjectSubtree(id, after)
					for k := range alt.ExplicitlyTouchedIDs() {
						scope[k] = true
					}
					viol := filterViolations(promotion.TreeConsistencyViolations(after), scope)
					assert.Empty(t, viol,
						"option %c must satisfy tree-consistency for subject subtree; %v", 'A'+i, viol)
					destroyed := promotion.FinishedWorkDestroyed(before, after, alt.ExplicitlyTouchedIDs())
					assert.Empty(t, destroyed,
						"option %c must not destroy finished work another option would keep: %v", 'A'+i, destroyed)

					d14After := doctorD14Explain(t, r)
					assert.NotContains(t, d14After.items, item,
						"option %c must clear D14 item %s; remaining=%v cmds=%v",
						'A'+i, item, d14After.items, alt.RunnableCommands())
					for mid, want := range protectedMerged {
						st, showErr := runTrls(t, r, "show", "--field", "status", mid)
						require.NoError(t, showErr)
						assert.Equal(t, want+"\n", st,
							"option %c must not disturb protected merged node %s", 'A'+i, mid)
					}
				})
			}
		})
	}

	// MERGED-1 on R: next_actions from same Plan model (Options, no claim under merged).
	mergedOut := new(bytes.Buffer)
	code := executeThenHandleRootError(t, mergedOut, new(bytes.Buffer),
		"merged", "--repo", repo, "--issue", "R", "--force", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, mergedOut.String())
	assert.Equal(t, codeMerged1, cf.Code)
	require.NotEmpty(t, cf.NextActions)
	assert.Equal(t, "Option A:", cf.NextActions[0])
	assert.Equal(t, "arm reopen --issue E", cf.NextActions[1])
	assert.Equal(t, "Option B:", cf.NextActions[2])
	assert.Equal(t, "arm transition --issue RC2 --to cancelled --force", cf.NextActions[3])
	assert.Equal(t, "arm transition --issue R --to cancelled --force", cf.NextActions[4])
	// Repair Options must not claim under merged; more-steps may name claim after reopen.
	for i := 0; i <= 4; i++ {
		assert.NotEqual(t, "arm claim --issue RC2 --worktree", cf.NextActions[i])
	}
	for _, action := range cf.NextActions {
		if promotion.IsRunnableArmCommand(action) {
			assert.NotContains(t, action, "; ")
		}
		assert.NotEqual(t, "arm merged --issue R", action)
	}
}

func TestFlatSequenceDestroysFinishedWork_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// Codex/PJ class: running R's old flat next_actions (reopen E then force-cancel
	// R/RC1/RS/RSC) clears D14 but destroys finished work Option A would keep.
	repo := setupMixedD14Tree(t)
	before := loadPromotionGraph(t, repo)
	flat := []string{
		"arm reopen --issue E",
		"arm transition --issue R --to cancelled --force",
		"arm transition --issue RC1 --to cancelled --force",
		"arm transition --issue RS --to cancelled --force",
		"arm transition --issue RSC --to cancelled --force",
	}
	require.NoError(t, applyRepairSequence(t, repo, flat))
	after := loadPromotionGraph(t, repo)
	d14 := doctorD14Explain(t, repo)
	assert.NotContains(t, d14.items, "R -> E", "flat sequence can quiet D14")
	destroyed := promotion.FinishedWorkDestroyed(before, after, map[string]bool{"E": true})
	require.NotEmpty(t, destroyed, "harness must FAIL flat reopen+force-cancel cascade")
	assert.Contains(t, strings.Join(destroyed, ";"), "finished work destroyed")

	// reopen E alone clears R→E without destroying finished children.
	r2 := setupMixedD14Tree(t)
	before2 := loadPromotionGraph(t, r2)
	require.NoError(t, applyRepairSequence(t, r2, []string{"arm reopen --issue E"}))
	after2 := loadPromotionGraph(t, r2)
	assert.NotContains(t, doctorD14Explain(t, r2).items, "R -> E")
	assert.Empty(t, promotion.FinishedWorkDestroyed(before2, after2, map[string]bool{"E": true}))
	assert.Empty(t, promotion.TreeConsistencyViolations(after2))
}

func TestRepairHarnessRejectsMutations_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()

	t.Run("reopen_merged_child_fails_harness", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		before := loadPromotionGraph(t, repo)
		err := applyRepairSequence(t, repo, []string{"arm reopen --issue RC3"})
		require.NoError(t, err, "reopen RC3 may exit 0")
		after := loadPromotionGraph(t, repo)
		assert.Contains(t, doctorD14Explain(t, repo).items, "R -> E",
			"un-merging RC3 must not count as clearing D14 R→E")
		destroyed := promotion.FinishedWorkDestroyed(before, after, nil)
		require.NotEmpty(t, destroyed, "harness must FAIL when suggested cmd un-merges RC3")
		err = assertD14ClearedWithoutDamagingMerged(t, repo, "R -> E", map[string]string{"RC3": "merged"})
		require.Error(t, err)
	})

	t.Run("cancel_without_force_fails_harness", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		err := applyRepairSequence(t, repo, []string{"arm transition --issue RC2 --to cancelled"})
		after := doctorD14Explain(t, repo)
		if !containsStr(after.items, "R -> E") && err == nil {
			t.Fatalf("unexpected: cancel without --force cleared D14 and succeeded")
		}
		err = assertD14ClearedWithoutDamagingMerged(t, repo, "R -> E", map[string]string{"RC3": "merged", "AM": "merged"})
		require.Error(t, err, "harness must FAIL for cancel without --force / partial repair")
	})

	t.Run("suggest_claim_under_merged_fails_invariant", func(t *testing.T) {
		t.Parallel()
		repo := setupMixedD14Tree(t)
		// Product gate: claim under merged must refuse (CLAIM-1) before creating a violation.
		out := new(bytes.Buffer)
		code := executeThenHandleRootError(t, out, new(bytes.Buffer),
			"claim", "--repo", repo, "--issue", "R", "--worktree", "--format", "agent")
		assert.Equal(t, 1, code)
		cf := agentFailureFromStdout(t, out.String())
		assert.Equal(t, codeClaim1, cf.Code)
		assert.Contains(t, cf.Cause, "merged ancestor")
		require.NotEmpty(t, cf.NextActions)
		assert.Contains(t, cf.NextActions, "Option A:")
		assert.Contains(t, cf.NextActions, "arm reopen --issue E")

		after := loadPromotionGraph(t, repo)
		assert.Equal(t, ops.StatusDone, after["R"].Status, "refused claim must leave R done")
		assert.True(t, promotion.WouldViolateTreeConsistency("R", after))
		viol := promotion.TreeConsistencyViolations(after)
		require.NotEmpty(t, viol, "open RC2 under merged E remains a tree-consistency violation")
	})
}

func TestNestedMergedAncestorChainCLI_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupNestedMergedAncestorTree(t)
	d14 := doctorD14Explain(t, repo)
	assert.Contains(t, d14.items, "D -> M2")

	plan := promotion.RemediationsPlan("D", loadPromotionGraph(t, repo))
	require.Len(t, plan.Alternatives, 2)
	assert.Equal(t, []string{
		"arm reopen --issue M1",
		"arm reopen --issue M2",
	}, plan.Alternatives[0].RunnableCommands())

	// Option A (full chain) clears D14 without leaving M2 open under M1.
	r := setupNestedMergedAncestorTree(t)
	require.NoError(t, applyAlternative(t, r, plan.Alternatives[0]))
	assert.NotContains(t, doctorD14Explain(t, r).items, "D -> M2")
	assert.Empty(t, promotion.TreeConsistencyViolations(loadPromotionGraph(t, r)))

	// Nearest-only reopen leaves M2 open under M1 — harness must FAIL that shape.
	r2 := setupNestedMergedAncestorTree(t)
	require.NoError(t, applyRepairSequence(t, r2, []string{"arm reopen --issue M2"}))
	viol := promotion.TreeConsistencyViolations(loadPromotionGraph(t, r2))
	require.NotEmpty(t, viol, "nearest-only reopen must leave inconsistency")
	assert.Contains(t, strings.Join(viol, ";"), "M2 under merged M1")
}

func TestFilledDeliverySequenceReachesMerge_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := doneWithoutAssessment(t)
	base, tip := landDelivery(t, repo, "task-01")
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)

	plan := promotion.RemediationsPlan("task-01", promotion.Graph{
		"task-01": {Status: ops.StatusDone, Base: base, Tip: tip},
	})
	require.Len(t, plan.MoreSteps, 2)
	assert.Equal(t, promotion.StepCommand, plan.MoreSteps[0].Kind)
	assert.Equal(t, delivery.RecordArgvFilled("task-01", base, tip), plan.MoreSteps[0].Argv)
	assert.Equal(t, promotion.StepRequiredInput, plan.MoreSteps[1].Kind)
	assert.Contains(t, plan.MoreSteps[1].Help, "produce assessment.json:")
	assert.Contains(t, plan.MoreSteps[1].Help, "arm review prepare")
	assert.NotContains(t, strings.ToLower(plan.MoreSteps[1].Help), "legacy")

	rendered := plan.RenderNextActions()
	require.NoError(t, applyRepairSequence(t, repo, []string{rendered[0]}))

	// Produce assessment.json using only printed how-to + documented generation.
	assessmentPath, bundlePath := produceAssessmentFromHowTo(t, repo, "task-01", base, tip, plan.MoreSteps[1].Help)
	recordCmd := plan.MoreSteps[1].Argv
	recordCmd = strings.ReplaceAll(recordCmd, "<assessment.json>", assessmentPath)
	recordCmd = strings.ReplaceAll(recordCmd, "<bundle.json>", bundlePath)
	require.NoError(t, applyRepairSequence(t, repo, []string{recordCmd}))

	out, err := runTrls(t, repo, "merged", "--issue", "task-01", "--force")
	require.NoError(t, err, "harness must reach merge via printed steps + documented generation; out=%s", out)
	status, err := runTrls(t, repo, "show", "--field", "status", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", status)
}

func TestAllCancelledP2UnderG2_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupMixedD14Tree(t)
	d14 := doctorD14Explain(t, repo)
	assert.Contains(t, d14.items, "P2 -> G2")

	plan := promotion.RemediationsPlan("P2", loadPromotionGraph(t, repo))
	require.Len(t, plan.Alternatives, 2)
	acts := plan.RenderNextActions()
	assert.Equal(t, "Option A:", acts[0])
	assert.Equal(t, "arm reopen --issue G2", acts[1])
	assert.Equal(t, "Option B:", acts[2])
	assert.Equal(t, "arm transition --issue P2 --to cancelled --force", acts[3])

	for i, alt := range plan.Alternatives {
		r := setupMixedD14Tree(t)
		before := loadPromotionGraph(t, r)
		require.NoError(t, applyAlternative(t, r, alt))
		after := loadPromotionGraph(t, r)
		assert.NotContains(t, doctorD14Explain(t, r).items, "P2 -> G2")
		// Scoped invariant: P2/G2/OS subtree — fixture also has RC2 under E.
		assert.Empty(t, promotion.FinishedWorkDestroyed(before, after, alt.ExplicitlyTouchedIDs()),
			"option %c", 'A'+i)
		for _, id := range []string{"P2", "G2", "OS"} {
			for _, v := range promotion.TreeConsistencyViolations(after) {
				assert.NotContains(t, v, id+" ", "option %c left %s inconsistent: %s", 'A'+i, id, v)
			}
		}
	}
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
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)
	ctx := getTestContext(t, repo)
	stateDir := getTestStateDir(t, repo)
	store := snapshot.NewStore(filepath.Join(ctx.IssuesDir, "ops"), stateDir)
	index, err := store.ReadIndex()
	require.NoError(t, err)
	require.NotEmpty(t, index, "expected materialized index at %s", store.IndexPath())
	return promotionGraphFromStore(store, index)
}

// applyAlternative runs one Plan alternative on a fresh fixture (harness adds
// only --repo/--format). Does not run MoreSteps — each Option alone must clear D14.
func applyAlternative(t *testing.T, repo string, alt promotion.Alternative) error {
	t.Helper()
	return applyRepairSequence(t, repo, alt.RunnableCommands())
}

// applyRepairSequence runs suggested cmds exactly as printed (harness adds only
// --repo/--format). Guidance, Option labels, and how-to prose are not executed
// as shell; Option-prefixed lines strip to the arm argv.
func applyRepairSequence(t *testing.T, repo string, cmds []string) error {
	t.Helper()
	var firstErr error
	for _, cmdLine := range cmds {
		if promotion.IsGuidanceRemediation(cmdLine) {
			continue
		}
		if promotion.IsLegacyTemplateRemediation(cmdLine) {
			continue
		}
		argv := promotion.RunnableArgv(cmdLine)
		if argv == "" {
			if promotion.IsRequiredInputRemediation(cmdLine) {
				return fmt.Errorf("required-input %q needs produced assessment path before run", cmdLine)
			}
			return fmt.Errorf("non-arm remediation %q", cmdLine)
		}
		args := strings.Fields(argv)
		if len(args) < 2 || args[0] != "arm" {
			return fmt.Errorf("bad argv %q", argv)
		}
		cliArgs := append([]string{}, args[1:]...)
		cliArgs = append(cliArgs, "--repo", repo, "--format", "agent")
		out := new(bytes.Buffer)
		code := executeThenHandleRootError(t, out, new(bytes.Buffer), cliArgs...)
		if code != 0 && firstErr == nil {
			firstErr = fmt.Errorf("suggested %q exit %d; out=%s", argv, code, out.String())
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
	g := loadPromotionGraph(t, repo)
	if viol := promotion.TreeConsistencyViolations(g); len(viol) > 0 {
		return fmt.Errorf("tree-consistency violations: %v", viol)
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

// produceAssessmentFromHowTo follows the printed required-input Help: review
// prepare → author ConformanceAssessment from the bundle → validate → paths.
func produceAssessmentFromHowTo(t *testing.T, repo, issueID, base, tip, howTo string) (assessmentPath, bundlePath string) {
	t.Helper()
	require.Contains(t, howTo, "arm review prepare")
	require.Contains(t, howTo, "arm review schema --output conformance-assessment.schema.json")
	require.Contains(t, howTo, "--bundle bundle.json")

	bundlePath = filepath.Join(t.TempDir(), "bundle.json")
	_, err := runTrls(t, repo, "review", "prepare",
		"--issue", issueID, "--base", base, "--head", tip, "--output", bundlePath)
	require.NoError(t, err)

	schemaPath := filepath.Join(t.TempDir(), "conformance-assessment.schema.json")
	_, err = runTrls(t, repo, "review", "schema", "--output", schemaPath)
	require.NoError(t, err, "arm review schema must ship the assessment schema")
	schemaBytes, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	require.NotEmpty(t, schemaBytes)
	require.Contains(t, string(schemaBytes), "Conformance Assessment")

	bundleJSON, err := os.ReadFile(bundlePath)
	require.NoError(t, err)
	bundle, err := review.DecodeReviewBundle(bundleJSON)
	require.NoError(t, err)

	citePath := "README.md"
	if bundle.Delivery.Diff != "" {
		// Prefer a path that appears in the delivery diff so validate accepts citations.
		for _, line := range strings.Split(bundle.Delivery.Diff, "\n") {
			if strings.HasPrefix(line, "+++ b/") {
				citePath = strings.TrimPrefix(line, "+++ b/")
				break
			}
		}
	}
	results := make([]review.CriterionResult, 0, len(bundle.Contract.Acceptance)+1)
	results = append(results, review.CriterionResult{
		ID:        "definition_of_done",
		Status:    review.Satisfied,
		Rationale: "Harness produced assessment from printed how-to + schema.",
		Citations: []review.Citation{review.FileCitation(citePath, 1, 0)},
	})
	for i := range bundle.Contract.Acceptance {
		results = append(results, review.CriterionResult{
			ID:        fmt.Sprintf("acceptance[%d]", i),
			Status:    review.Satisfied,
			Rationale: "Declared acceptance met for merge-path proof.",
			Citations: []review.Citation{review.FileCitation(citePath, 1, 0)},
		})
	}
	assessment := review.ConformanceAssessment{
		SchemaVersion:       review.SchemaVersion,
		BundleID:            bundle.BundleID,
		ContractFingerprint: bundle.Fingerprints.Contract,
		DeliveryFingerprint: bundle.Fingerprints.Delivery,
		Results:             results,
	}
	assessmentJSON, err := json.Marshal(assessment)
	require.NoError(t, err)
	assessmentPath = filepath.Join(t.TempDir(), "assessment.json")
	require.NoError(t, os.WriteFile(assessmentPath, assessmentJSON, 0o600))

	_, err = runTrls(t, repo, "review", "validate",
		"--assessment", assessmentPath, "--bundle", bundlePath)
	require.NoError(t, err, "printed how-to requires validate before record")
	return assessmentPath, bundlePath
}

// TestDropEitherAlternativeFailsDoctorAndMerged_REQ_TOPTIER_S18_T4 injects a drop
// into the product planProducer (not a local Plan copy) and asserts doctor /
// arm merged observe the incomplete Options. Not parallel — mutates package state.
func TestDropEitherAlternativeFailsDoctorAndMerged_REQ_TOPTIER_S18_T4(t *testing.T) {
	type producer = func(string, promotion.Graph, promotion.PlanOpts) promotion.Plan
	restore := promotion.ReplacePlanProducerForTest(func(prev producer) producer {
		return func(id string, g promotion.Graph, opts promotion.PlanOpts) promotion.Plan {
			p := prev(id, g, opts)
			if len(p.Alternatives) >= 2 {
				p.Alternatives = p.Alternatives[:1]
			}
			return p
		}
	})
	t.Cleanup(restore)

	repo := setupMixedD14Tree(t)
	plan := promotion.RemediationsPlan("R", loadPromotionGraph(t, repo))
	assert.Less(t, len(plan.Alternatives), 2,
		"injected drop must shrink product Plan Alternatives")
	d14 := doctorD14Explain(t, repo)
	assert.NotContains(t, d14.suggested, "Option B:",
		"doctor --explain must reflect the mutated producer (not a local Plan copy)")
	assert.Contains(t, d14.items, "R -> E", "D14 finding remains; incomplete remediations do not clear it")

	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer),
		"merged", "--issue", "R", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code, "arm merged on incomplete-remediation tree must fail")
	assert.NotContains(t, out.String(), "Option B:")
}

// TestOwnedAndForeignLeaseUnderMergedCLI_REQ_TOPTIER_S18_T4 covers the PJ class:
// own lease + foreign live lease under a merged ancestor; every printed Option's
// steps clear the finding or the Option is correctly not offered (no invented takeover).
func TestOwnedAndForeignLeaseUnderMergedCLI_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo, workerID := setupOwnedForeignMergedFixture(t)
	other := "other-worker-foreign"

	g := loadPromotionGraph(t, repo)
	assert.Equal(t, ops.StatusClaimed, g["OS"].Status)
	assert.Equal(t, other, g["OS"].ClaimedBy)
	assert.Equal(t, ops.StatusClaimed, g["OWN"].Status)
	assert.Equal(t, workerID, g["OWN"].ClaimedBy)

	assertNoInventedTakeoverArgv := func(t *testing.T, lines []string) {
		t.Helper()
		for _, line := range lines {
			cmd := promotion.RunnableArgv(line)
			if cmd == "" {
				continue
			}
			assert.NotContains(t, cmd, "arm claim --issue OS --force", "no invented takeover argv: %s", cmd)
			assert.NotEqual(t, "arm transition --issue OS --to cancelled --force", cmd,
				"must not print gate-refused cancel-other as runnable step")
			assert.NotContains(t, cmd, "arm unassign ")
		}
	}

	// Claim under merged refuses; next_actions must not invent claim --force takeover.
	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer),
		"claim", "--issue", "OS", "--force", "--worktree", "--repo", repo, "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, out.String())
	assert.Equal(t, codeClaim1, cf.Code)
	assertNoInventedTakeoverArgv(t, cf.NextActions)
	assert.Contains(t, cf.NextActions, "arm reopen --issue G2")
	assert.True(t, containsSubstring(cf.NextActions, "live claimed lease on OS held by "+other))
	assert.True(t, containsSubstring(cf.NextActions, ".worktrees/OS"))

	// Doctor --explain: foreign OS → no Option B cancel-other; OWN subject → cancel owned OK.
	d14 := doctorD14Explain(t, repo)
	assertNoInventedTakeoverArgv(t, d14.suggested)
	assert.True(t, containsSubstring(d14.suggested, "live claimed lease on OS held by "+other))
	assert.True(t, containsSubstring(d14.suggested, ".worktrees/OS"))

	osPlan := promotion.RemediationsPlanFor("OS", g, workerID)
	require.Len(t, osPlan.Alternatives, 1, "foreign lease drops Option B")
	assert.Equal(t, []string{"arm reopen --issue G2"}, osPlan.Alternatives[0].RunnableCommands())

	ownPlan := promotion.RemediationsPlanFor("OWN", g, workerID)
	require.Len(t, ownPlan.Alternatives, 2, "owned lease keeps Option B cancel")
	assert.Equal(t, []string{"arm transition --issue OWN --to cancelled --force"},
		ownPlan.Alternatives[1].RunnableCommands())

	// Run every printed Option's steps on a fresh fixture whose viewer matches the plan.
	for i := range ownPlan.Alternatives {
		fresh, freshWorker := setupOwnedForeignMergedFixture(t)
		before := loadPromotionGraph(t, fresh)
		plan := promotion.RemediationsPlanFor("OWN", before, freshWorker)
		require.Len(t, plan.Alternatives, 2)
		alt := plan.Alternatives[i]
		require.NoError(t, applyAlternative(t, fresh, alt), "option %c", 'A'+i)
		after := loadPromotionGraph(t, fresh)
		assert.Empty(t, filterViolations(promotion.TreeConsistencyViolations(after), subjectSubtree("OWN", before)),
			"option %c must clear OWN tree inconsistency", 'A'+i)
		assert.Empty(t, promotion.FinishedWorkDestroyed(before, after, alt.ExplicitlyTouchedIDs()))
	}

	// Foreign Option A (reopen) clears OS inconsistency; Option B correctly not offered.
	freshOS, freshOSWorker := setupOwnedForeignMergedFixture(t)
	osFresh := promotion.RemediationsPlanFor("OS", loadPromotionGraph(t, freshOS), freshOSWorker)
	require.Len(t, osFresh.Alternatives, 1)
	require.NoError(t, applyAlternative(t, freshOS, osFresh.Alternatives[0]))
	afterOS := loadPromotionGraph(t, freshOS)
	assert.Empty(t, filterViolations(promotion.TreeConsistencyViolations(afterOS), subjectSubtree("OS", g)))
}

func containsSubstring(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// setupOwnedForeignMergedFixture builds G2(merged)→P2(done)→{OS foreign claimed, OWN self claimed}.
func setupOwnedForeignMergedFixture(t *testing.T) (repo, workerID string) {
	t.Helper()
	repo = gittest.InitWithOrigin(t).Dir
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")
	bootstrapRepoForTest(t, repo)
	ctx := getTestContext(t, repo)
	var logPath string
	var err error
	workerID, logPath, err = resolveWorkerAndLog(ctx)
	require.NoError(t, err)

	for _, c := range []struct {
		id, title, typ, parent string
	}{
		{"G2", "Epic G2", "epic", ""},
		{"P2", "Story P2", "story", "G2"},
		{"OS", "Foreign OS", "task", "P2"},
		{"OWN", "Owned leaf", "task", "P2"},
	} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpCreate, TargetID: c.id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{
				Title: c.title, NodeType: c.typ, Parent: c.parent,
				DefinitionOfDone: c.title + " is complete and tested",
				Acceptance:       json.RawMessage(testAcceptance),
				Confidence:       "verified",
				Scope:            []string{"cmd/armature/" + strings.ToLower(c.id) + ".go"},
			},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "P2", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusDone, Outcome: "fixture done parent under merged for owned/foreign lease class"},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)

	// Claim while G2 is still open so leases publish; then merge G2 (product gate
	// refuses claim under merged). Real arm claim — raw AppendOp is not published.
	_, err = runTrls(t, repo, "claim", "--issue", "OWN", "--worktree")
	require.NoError(t, err)
	other := "other-worker-foreign"
	_, err = runTrls(t, repo, "claim", "--issue", "OS", "--worktree", "--worker-id", other)
	require.NoError(t, err)

	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "G2", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusMerged},
	}))
	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	return repo, workerID
}

func subjectSubtree(id string, g promotion.Graph) map[string]bool {
	out := map[string]bool{id: true}
	var walk func(string)
	walk = func(cid string) {
		n, ok := g[cid]
		if !ok {
			return
		}
		for _, ch := range n.Children {
			if out[ch] {
				continue
			}
			out[ch] = true
			walk(ch)
		}
	}
	walk(id)
	return out
}

func filterViolations(viol []string, scope map[string]bool) []string {
	var out []string
	for _, v := range viol {
		id, _, _ := strings.Cut(v, " ")
		if scope[id] {
			out = append(out, v)
		}
	}
	return out
}
