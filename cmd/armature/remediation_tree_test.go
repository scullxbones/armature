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
//	├── LF (done leaf)
//	└── R (done story; non-leaf under test for arm merged)
//	    ├── RC1 (done)
//	    ├── RC2 (open)
//	    ├── RC3 (merged)
//	    └── RS (done feature / nested non-leaf)
//	        └── RSC (done)
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
	for _, id := range []string{"LF", "RC1", "RSC", "RS", "R"} {
		require.NoError(t, appendOp(ctx, logPath, ops.Op{
			Type: ops.OpTransition, TargetID: id, Timestamp: nowEpoch(), WorkerID: workerID,
			Payload: ops.Payload{To: ops.StatusDone, Outcome: outcome},
		}))
	}
	require.NoError(t, appendOp(ctx, logPath, ops.Op{
		Type: ops.OpTransition, TargetID: "RC3", Timestamp: nowEpoch(), WorkerID: workerID,
		Payload: ops.Payload{To: ops.StatusMerged},
	}))
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

	wantDoneLeaf := delivery.RecordArgv("LF")
	wantRC1 := delivery.RecordArgv("RC1")
	wantRC2 := "arm claim --issue RC2"
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
	}
	assert.Contains(t, suggested, wantDoneLeaf)
	assert.Contains(t, suggested, wantRC1)
	assert.Contains(t, suggested, wantRC2)
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
		assert.NotContains(t, action, "arm merged --issue RS")
	}
	assert.Contains(t, cf.NextActions, wantRC1)
	assert.Contains(t, cf.NextActions, wantRC2)
	assert.Contains(t, cf.NextActions, wantRSC)
	assert.NotContains(t, cf.NextActions, wantDoneLeaf, "LF is sibling of R, not under R")

	// Run every suggested/next_actions command: succeed, or honest open-work / delivery placeholder.
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

func assertRunnableRemediation(t *testing.T, repo, cmdLine string) {
	t.Helper()
	if !promotion.IsRunnableArmCommand(cmdLine) {
		// Prose guidance (fallback / children-merged) — not an argv to execute.
		assert.True(t,
			cmdLine == promotion.FallbackGuidance || cmdLine == promotion.ChildrenMergedGuidance,
			"non-arm guidance must be a known constant, got %q", cmdLine)
		return
	}
	if strings.Contains(cmdLine, "<sha>") || strings.Contains(cmdLine, "<assessment.json>") {
		// Honest placeholder recovery (delivery record / review) — same truth as sync.
		assert.True(t, strings.HasPrefix(cmdLine, "arm delivery record") ||
			strings.HasPrefix(cmdLine, "arm review record"),
			"placeholder recovery must be delivery/review record, got %q", cmdLine)
		assert.NotContains(t, cmdLine, "arm merged")
		return
	}
	args := strings.Fields(cmdLine)
	require.GreaterOrEqual(t, len(args), 2)
	require.Equal(t, "arm", args[0])
	cliArgs := append([]string{}, args[1:]...)
	cliArgs = append(cliArgs, "--repo", repo, "--format", "agent")
	if args[1] == "merged" || args[1] == "claim" {
		cliArgs = append(cliArgs, "--force")
	}
	out := new(bytes.Buffer)
	code := executeThenHandleRootError(t, out, new(bytes.Buffer), cliArgs...)
	if code == 0 {
		return
	}
	// Open-work claim may refuse for graph reasons; must not be not-leaf / arm-merged self-loop.
	if !json.Valid(bytes.TrimSpace(out.Bytes())) {
		t.Fatalf("suggested %q failed without agent envelope: %s", cmdLine, out.String())
	}
	cf := agentFailureFromStdout(t, out.String())
	assert.NotContains(t, cf.Cause, "not a leaf",
		"suggested %q must not fail as not-leaf", cmdLine)
	assert.NotEqual(t, "arm merged --issue R", cmdLine)
}
