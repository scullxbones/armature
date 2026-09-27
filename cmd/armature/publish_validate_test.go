package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakefileValidateGraphIsArmValidateCI_REQ_PUBLISH_VALIDATE(t *testing.T) {
	root := projectRootDir(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "validate-graph:", "CI job make validate-graph must exist")
	assert.Contains(t, text, "./bin/arm validate --ci",
		"make validate-graph must invoke the single fail-closed command")
}

func TestPushOpsAndValidateCIAgreeOnW1_REQ_PUBLISH_VALIDATE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	plantOverlappingFooPair(t, repo)
	run(t, worktree, "git", "add", "-A")
	run(t, worktree, "git", "commit", "-m", "ops: overlapping pair")

	valOut, valErr := runTrls(t, repo, "validate", "--ci", "--format", "human")
	require.Error(t, valErr, "arm validate --ci must fail closed on W1")
	assert.Contains(t, valOut+"\n"+valErr.Error(), "scope overlap")

	pushOut, pushErr := runTrls(t, repo, "push-ops", "--format", "human")
	require.Error(t, pushErr, "push-ops must use the same fail-closed contract")
	combined := pushOut + "\n" + pushErr.Error()
	assert.Contains(t, combined, "scope overlap")
	assert.Contains(t, combined, "push refused")
	assert.Contains(t, combined, "arm validate --ci")
	assert.False(t, originArmatureContains(t, bareDir, "task-02"),
		"origin must not receive the overlapping unpublished graph")
}

func TestPushOpsRefusesRemoteW1Shape_REQ_PUBLISH_VALIDATE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	claimordLog := filepath.Join(worktree, "ops", "claimord-worker.log")
	require.NoError(t, ops.AppendOp(claimordLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "CLAIMORD-W21",
		Timestamp: nowEpoch(),
		WorkerID:  "claimord-worker",
		Payload: ops.Payload{
			Title:            "in-flight other story",
			NodeType:         "task",
			Scope:            []string{"docs/commands.md"},
			DefinitionOfDone: "Task CLAIMORD-W21 is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
		},
	}))
	run(t, worktree, "git", "add", "ops/claimord-worker.log")
	run(t, worktree, "git", "commit", "-m", "ops: create CLAIMORD-W21")
	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	require.True(t, originArmatureContains(t, bareDir, "CLAIMORD-W21"))

	run(t, worktree, "git", "reset", "--hard", "HEAD~1")
	require.NoError(t, os.Remove(claimordLog))
	require.NoError(t, os.RemoveAll(filepath.Join(worktree, "state")))
	require.NoFileExists(t, claimordLog)

	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--id", "LNGHZN-S11-T1",
		"--title", "Record a delivery at done",
		"--scope", "docs/commands.md",
		"--dod", "Task LNGHZN-S11-T1 is complete and tested")
	require.NoError(t, err)

	valOut, valErr := runTrls(t, repo, "validate", "--ci", "--format", "human")
	require.NoError(t, valErr, "stale local graph must not see the remote CLAIMORD task yet; got %s / %v", valOut, valErr)

	pushOut, pushErr := runTrls(t, repo, "push-ops", "--format", "human")
	require.Error(t, pushErr)
	combined := pushOut + "\n" + pushErr.Error()
	assert.Contains(t, combined, "scope overlap")
	assert.Contains(t, combined, "CLAIMORD-W21")
	assert.Contains(t, combined, "LNGHZN-S11-T1")
	assert.False(t, originArmatureContains(t, bareDir, "LNGHZN-S11-T1"),
		"push must refuse so origin stays CLAIMORD-only")
	assert.True(t, originArmatureContains(t, bareDir, "CLAIMORD-W21"))
}

func TestPushOpsCleanGraphStillPushes_REQ_PUBLISH_VALIDATE(t *testing.T) {
	bareDir, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create", "--type", "task", "--id", "CLEAN-TASK",
		"--title", "clean", "--scope", "cmd/armature/clean_only.go",
		"--dod", "Task CLEAN-TASK is complete and tested")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	assert.True(t, originArmatureContains(t, bareDir, "CLEAN-TASK"))
}

func TestPushOpsOverrideRequiresReasonAndTTY_REQ_PUBLISH_VALIDATE(t *testing.T) {
	_, repo, _ := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops", "--override-validate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--reason")

	_, err = runTrls(t, repo, "push-ops", "--non-interactive", "--override-validate", "--reason", "ship anyway")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controlling terminal")
}

func TestPlannerSkillTreatsPublishValidateFailureAsStop_REQ_PUBLISH_VALIDATE(t *testing.T) {
	root := projectRootDir(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/skillsembed/skills/armature-planner/SKILL.md"))
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "arm validate --ci")
	assert.Contains(t, text, "make validate-graph")
	assert.Contains(t, strings.ToLower(text), "stop")
	assert.NotContains(t, text, "--override-validate")
}
