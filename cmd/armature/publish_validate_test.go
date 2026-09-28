package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
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
	require.NoError(t, os.RemoveAll(claimordLog))
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

func TestPublishValidateAfterIntegrateNilWhenOverride_REQ_PUBLISH_VALIDATE(t *testing.T) {
	assert.Nil(t, publishValidateAfterIntegrate(nil, nil, true),
		"recorded override must not revalidate after rebase")
	cb := publishValidateAfterIntegrate(nil, nil, false)
	require.NotNil(t, cb, "push-ops must revalidate after every integrate rebase")
	err := cb()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command context unavailable")
}

func TestMergeSourceManifestUnionsRemoteOnlyEntries_REQ_PUBLISH_VALIDATE(t *testing.T) {
	dir := t.TempDir()
	local := sources.Manifest{}
	local.Upsert(sources.SourceEntry{ID: "local-id", Title: "local"})
	require.NoError(t, sources.WriteManifest(dir, local))

	remote := sources.Manifest{}
	remote.Upsert(sources.SourceEntry{ID: "remote-id", Title: "remote"})
	blob, err := remote.Marshal()
	require.NoError(t, err)
	require.NoError(t, mergeSourceManifestFile(dir, blob))

	got, err := sources.ReadManifest(dir)
	require.NoError(t, err)
	_, ok := got.Get("local-id")
	assert.True(t, ok, "local source must remain")
	_, ok = got.Get("remote-id")
	assert.True(t, ok, "remote-only source must be merged into the candidate graph")
}

func TestPushOpsAcceptsRemoteOnlySourceCitation_REQ_PUBLISH_VALIDATE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	srcFile := filepath.Join(t.TempDir(), "remote-only.md")
	require.NoError(t, os.WriteFile(srcFile, []byte("# remote only\n"), 0o600))
	out, err := runTrls(t, repo, "sources", "add", "--url", srcFile, "--type", "filesystem", "--title", "RemoteOnly")
	require.NoError(t, err)
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3, "expected 'added source <uuid> ...' in output: %s", out)
	sourceID := parts[2]

	remoteLog := filepath.Join(worktree, "ops", "remote-src-worker.log")
	require.NoError(t, ops.AppendOp(remoteLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "REMOTE-SRC-TASK",
		Timestamp: nowEpoch(),
		WorkerID:  "remote-src-worker",
		Payload: ops.Payload{
			Title:            "remote cited task",
			NodeType:         "task",
			Scope:            []string{"cmd/armature/remote_src_only.go"},
			DefinitionOfDone: "Task REMOTE-SRC-TASK is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
		},
	}))
	require.NoError(t, ops.AppendOp(remoteLog, ops.Op{
		Type:      ops.OpSourceLink,
		TargetID:  "REMOTE-SRC-TASK",
		Timestamp: nowEpoch(),
		WorkerID:  "remote-src-worker",
		Payload:   ops.Payload{SourceID: sourceID, Title: "RemoteOnly"},
	}))
	run(t, worktree, "git", "add", "ops/remote-src-worker.log")
	run(t, worktree, "git", "commit", "-m", "ops: remote-only source citation")
	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)
	require.True(t, originArmatureContains(t, bareDir, "REMOTE-SRC-TASK"))

	// Stale worktree like a lagging clone: local manifest lacks the remote UUID
	// and the remote worker log is missing on disk. HEAD still matches origin.
	stale := sources.Manifest{}
	stale.Upsert(sources.SourceEntry{ID: "00000000-0000-0000-0000-000000000001", Title: "stale-local"})
	require.NoError(t, sources.WriteManifest(filepath.Join(worktree, "sources"), stale))
	require.NoError(t, os.RemoveAll(remoteLog))
	require.NoError(t, os.RemoveAll(filepath.Join(worktree, "state")))

	pushOut, pushErr := runTrls(t, repo, "push-ops", "--format", "human")
	require.NoError(t, pushErr, "remote-only source UUID must not refuse publish; got %s / %v", pushOut, pushErr)
	assert.NotContains(t, pushOut+"\n"+fmt.Sprint(pushErr), "unknown source")
	assert.True(t, originArmatureContains(t, bareDir, "REMOTE-SRC-TASK"))
}

func TestPushOpsRevalidatesAfterNonFFRebase_REQ_PUBLISH_VALIDATE(t *testing.T) {
	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	parent := t.TempDir()
	competing := filepath.Join(parent, "rival")
	run(t, parent, "git", "clone", "--branch", "_armature", "file://"+bareDir, competing)
	run(t, competing, "git", "config", "user.email", "test@test.com")
	run(t, competing, "git", "config", "user.name", "Test")
	run(t, competing, "git", "config", "commit.gpgsign", "false")

	rivalLog := filepath.Join(competing, "ops", "rival-worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(rivalLog), 0o755))
	require.NoError(t, ops.AppendOp(rivalLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "RIVAL-NFF-TASK",
		Timestamp: nowEpoch(),
		WorkerID:  "rival-worker",
		Payload: ops.Payload{
			Title:            "in-flight rival after preflight",
			NodeType:         "task",
			Scope:            []string{"docs/commands.md"},
			DefinitionOfDone: "Task RIVAL-NFF-TASK is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
		},
	}))
	run(t, competing, "git", "add", "ops/rival-worker.log")
	run(t, competing, "git", "commit", "-m", "ops: rival overlapping task")

	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--id", "LOCAL-NFF-TASK",
		"--title", "local overlapping after preflight",
		"--scope", "docs/commands.md",
		"--dod", "Task LOCAL-NFF-TASK is complete and tested")
	require.NoError(t, err)

	installOneShotPrePushRival(t, worktree, competing)

	pushOut, pushErr := runTrls(t, repo, "push-ops", "--format", "human")
	require.Error(t, pushErr, "rebased graph must be revalidated before the retry push")
	combined := pushOut + "\n" + pushErr.Error()
	assert.Contains(t, combined, "scope overlap")
	assert.Contains(t, combined, "push refused")
	assert.False(t, originArmatureContains(t, bareDir, "LOCAL-NFF-TASK"),
		"retry push must not publish the dirty integrated graph")
	assert.True(t, originArmatureContains(t, bareDir, "RIVAL-NFF-TASK"),
		"the first-push hook should have published the rival tip")
}

func installOneShotPrePushRival(t *testing.T, worktree, competing string) {
	t.Helper()
	common := strings.TrimSpace(runOutput(t, worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	hooks := filepath.Join(common, "hooks")
	require.NoError(t, os.MkdirAll(hooks, 0o755))
	marker := filepath.Join(hooks, "arm-nff-planted")
	script := fmt.Sprintf(`#!/bin/sh
if [ -f %q ]; then
  exit 0
fi
touch %q
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR
git -C %q push origin _armature
`, marker, marker, competing)
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "pre-push"), []byte(script), 0o755))
}
