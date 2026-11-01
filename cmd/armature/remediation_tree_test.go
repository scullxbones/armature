package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/delivery"
	"github.com/scullxbones/armature/internal/gittest"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/promotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMixedD14Tree builds (legal hierarchy: story→feature→task):
//
//	E (merged epic)
//	├── LF (done leaf; legacy done — no claim/delivery provenance)
//	├── R (done story; non-leaf under test for arm merged)
//	│   ├── RC1 (done; legacy)
//	│   ├── RC2 (open — inconsistent under merged E)
//	│   ├── RC3 (merged)
//	│   └── RS (done feature / nested non-leaf)
//	│       └── RSC (done; legacy)
//	└── AM (story; rollup-merged after AMC merged)
//	    └── AMC (merged)
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
	for _, id := range []string{"LF", "RC1", "RSC", "RS", "R", "AM"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusDone, Outcome: outcome},
		}))
	}
	for _, id := range []string{"RC3", "AMC"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusMerged},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "E", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusMerged},
	}))
	// RC2 stays open.

	_, err = runTrls(t, repo, "materialize")
	require.NoError(t, err)
	return repo
}

func TestMixedTreeRemediationDoctorAndMerged_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	repo := setupMixedD14Tree(t)

	// AM rolled up when AMC merged — dead "children are merged; reopen" path unreachable.
	amStatus, err := runTrls(t, repo, "show", "--field", "status", "AM")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", amStatus, "RunRollup must auto-merge AM when AMC is merged")

	wantDoneLeaf := delivery.RecordArgv("LF") // template: legacy done, no Base/Tip
	wantRC1 := delivery.RecordArgv("RC1")
	wantReopen := "arm reopen --issue E"
	wantCancel := "arm transition --issue RC2 --to cancelled --force"
	wantRSC := delivery.RecordArgv("RSC")

	stdout := new(bytes.Buffer)
	_ = executeThenHandleRootError(t, stdout, new(bytes.Buffer),
		"doctor", "--repo", repo, "--explain", "--format", "json")
	payload := assertSingleJSONObject(t, stdout.String())
	checks, ok := payload["checks"].([]any)
	require.True(t, ok)
	var d14 map[string]any
	for _, raw := range checks {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		if row["check"] == "D14" {
			d14 = row
			break
		}
	}
	require.NotNil(t, d14, "D14 must be present; out=%s", stdout.String())
	assert.Equal(t, "warning", d14["severity"])
	expl, okExpl := d14["explanation"].(string)
	require.True(t, okExpl, "D14 explanation must be a string")
	assert.Contains(t, expl, promotion.TreeRepairGuidance)
	suggestedRaw, ok := d14["suggested"].([]any)
	require.True(t, ok, "D14 suggested must be an array")
	var suggested []string
	for _, s := range suggestedRaw {
		str, ok := s.(string)
		require.True(t, ok)
		suggested = append(suggested, str)
		assert.NotContains(t, str, "; ", "suggested entries must not be '; '-joined")
		assert.NotContains(t, str, "arm merged --issue R")
		assert.NotContains(t, str, "arm merged --issue RS")
		assert.NotContains(t, str, "arm merged --issue RC3")
		assert.NotContains(t, str, "arm claim --issue RC2",
			"open under merged ancestor must not suggest claim")
		assert.NotContains(t, str, "arm reopen --issue AM",
			"rollup-merged AM must not get dead children-merged reopen")
		assert.NotContains(t, str, "arm reopen --issue R")
	}
	assert.Contains(t, suggested, wantDoneLeaf)
	assert.Contains(t, suggested, wantRC1)
	assert.Contains(t, suggested, wantReopen)
	assert.Contains(t, suggested, wantCancel)
	assert.Contains(t, suggested, wantRSC)

	mergedOut := new(bytes.Buffer)
	code := executeThenHandleRootError(t, mergedOut, new(bytes.Buffer),
		"merged", "--repo", repo, "--issue", "R", "--force", "--format", "agent")
	assert.Equal(t, 1, code)
	cf := agentFailureFromStdout(t, mergedOut.String())
	assert.Equal(t, codeMerged1, cf.Code)
	assert.Contains(t, cf.Cause, "not a leaf")
	require.NotEmpty(t, cf.NextActions)
	for _, action := range cf.NextActions {
		assert.NotContains(t, action, "; ", "next_actions must be one command per entry")
		assert.NotEqual(t, "arm merged --issue R", action)
		assert.NotContains(t, action, "arm claim --issue RC2")
	}
	assert.Contains(t, cf.NextActions, wantRC1)
	assert.Contains(t, cf.NextActions, wantReopen)
	assert.Contains(t, cf.NextActions, wantCancel)
	assert.Contains(t, cf.NextActions, wantRSC)
	assert.NotContains(t, cf.NextActions, wantDoneLeaf, "LF is sibling of R, not under R")

	// Already-merged no-op retained.
	already := new(bytes.Buffer)
	alreadyCode := executeThenHandleRootError(t, already, new(bytes.Buffer),
		"merged", "--repo", repo, "--issue", "AM", "--force")
	assert.Equal(t, 0, alreadyCode)
	assert.Contains(t, already.String(), "already merged")

	// Run every suggested/next_actions command exactly as printed.
	allCmds := append([]string{}, suggested...)
	allCmds = append(allCmds, cf.NextActions...)
	seen := map[string]bool{}
	for _, cmdLine := range allCmds {
		if seen[cmdLine] {
			continue
		}
		seen[cmdLine] = true
		assertRunnableRemediation(t, repo, cmdLine)
	}
}

func TestFilledDeliveryRecordReachesMerge_REQ_TOPTIER_S18_T4(t *testing.T) {
	t.Parallel()
	// When Base/Tip are knowable (claim provenance), remediation emits filled
	// SHAs — not <sha> placeholders — and that argv is runnable through merge.
	repo := doneWithoutAssessment(t)
	base, tip := landDelivery(t, repo, "task-01")
	attestDeliveryForTest(t, repo, "task-01", base, tip)
	_, err := runTrls(t, repo, "materialize")
	require.NoError(t, err)
	require.NotEmpty(t, base)
	require.NotEmpty(t, tip)

	g := promotion.Graph{
		"task-01": {Status: ops.StatusDone, Base: base, Tip: tip},
	}
	acts := promotion.RemediationsForIssue("task-01", g)
	require.Len(t, acts, 1)
	filled := acts[0]
	assert.Equal(t, delivery.RecordArgvFilled("task-01", base, tip), filled)
	assert.False(t, promotion.IsDeliveryRecordTemplate(filled))
	assertRunnableRemediation(t, repo, filled)

	mergedOut := new(bytes.Buffer)
	mcode := executeThenHandleRootError(t, mergedOut, new(bytes.Buffer),
		"merged", "--repo", repo, "--issue", "task-01", "--force")
	require.Equal(t, 0, mcode, "filled delivery + attestation must reach merge; out=%s", mergedOut.String())
	status, err := runTrls(t, repo, "show", "--field", "status", "task-01")
	require.NoError(t, err)
	assert.Equal(t, "merged\n", status)
}

func assertRunnableRemediation(t *testing.T, repo, cmdLine string) {
	t.Helper()
	if !promotion.IsRunnableArmCommand(cmdLine) {
		// Prose guidance only — not an argv. Tree-repair prose is paired with
		// runnable reopen/cancel entries in the same suggested list.
		assert.True(t,
			cmdLine == promotion.FallbackGuidance || cmdLine == promotion.TreeRepairGuidance,
			"non-arm guidance must be a known constant, got %q", cmdLine)
		return
	}
	if promotion.IsDeliveryRecordTemplate(cmdLine) {
		// Documented next step: legacy done with no claim/delivery provenance.
		// Placeholders are not git objects; substituting invented SHAs fails DELIVERY-1.
		assert.Contains(t, cmdLine, "<sha>")
		assert.NotContains(t, cmdLine, "arm merged")
		return
	}
	if strings.Contains(cmdLine, "<assessment.json>") {
		assert.True(t, strings.HasPrefix(cmdLine, "arm review record"),
			"assessment placeholder recovery must be review record, got %q", cmdLine)
		return
	}
	// Execute exactly as printed (only harness adds --repo/--format).
	args := strings.Fields(cmdLine)
	require.GreaterOrEqual(t, len(args), 2)
	require.Equal(t, "arm", args[0])
	cliArgs := append([]string{}, args[1:]...)
	cliArgs = append(cliArgs, "--repo", repo, "--format", "agent")
	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer), cliArgs...)
	require.Equal(t, 0, code, "suggested %q must succeed; out=%s", cmdLine, out.String())
}
