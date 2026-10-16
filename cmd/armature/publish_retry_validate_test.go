package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishRetryRevalidates_REQ_PUBLISH_VALIDATE(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("helpers.go")
	require.NoError(t, err)
	text := string(src)
	assert.Contains(t, text, "type afterIntegrateHook struct",
		"pushOpsBranchAfter must take a value-typed hook so nil cannot compile")
	assert.NotContains(t, text, "func pushOpsBranchAfter(ctx *config.Context, gc *adapters.Client, tracker ops.PendingPushTracker, afterIntegrate func() error",
		"a func() error parameter would still accept nil")
	assert.NotContains(t, text, "if afterIntegrate != nil",
		"retry must always run the after-integrate hook")

	stub := &stubOpsPublisher{
		firstPushErr: errors.New("git push origin _armature: rejected (non-fast-forward)"),
		secondPush:   errors.New("must not retry-push a dirty graph"),
	}
	called := 0
	err = publishArmatureSequence(stub, runAfterIntegrate(func() error {
		called++
		return skipCommandFailure(errors.New(
			"push refused: validation failed with 1 error(s) and 0 warning(s) " +
				"(same contract as arm validate --ci / make validate-graph)"))
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "push refused: validation")
	assert.Equal(t, 1, called, "CI validation must re-run after the rebase")
	assert.Equal(t, 1, stub.pushes, "retry push must not run after a validate finding")
	assert.Equal(t, 1, stub.rebases)

	bareDir, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	parent := t.TempDir()
	competing := filepath.Join(parent, "rival")
	run(t, parent, "git", "clone", "--branch", "_armature", "file://"+bareDir, competing)
	run(t, competing, "git", "config", "user.email", "test@test.com")
	run(t, competing, "git", "config", "user.name", "Test")
	run(t, competing, "git", "config", "commit.gpgsign", "false")

	rivalLog := filepath.Join(competing, "ops", "retry-rival-worker.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(rivalLog), 0o755))
	require.NoError(t, ops.AppendOp(rivalLog, ops.Op{
		Type:      ops.OpCreate,
		TargetID:  "RETRY-RIVAL-TASK",
		Timestamp: nowEpoch(),
		WorkerID:  "retry-rival-worker",
		Payload: ops.Payload{
			Title:            "in-flight rival after preflight",
			NodeType:         "task",
			Scope:            []string{"docs/commands.md"},
			DefinitionOfDone: "Task RETRY-RIVAL-TASK is complete and tested",
			Acceptance:       json.RawMessage(testAcceptance),
			Confidence:       "verified",
		},
	}))
	run(t, competing, "git", "add", "ops/retry-rival-worker.log")
	run(t, competing, "git", "commit", "-m", "ops: rival overlapping task")

	_, err = runTrls(t, repo, "create",
		"--type", "task",
		"--id", "RETRY-LOCAL-TASK",
		"--title", "local overlapping after preflight",
		"--scope", "docs/commands.md",
		"--dod", "Task RETRY-LOCAL-TASK is complete and tested")
	require.NoError(t, err)

	installOneShotPrePushRival(t, worktree, competing)

	pushOut, pushErr := runTrls(t, repo, "push-ops", "--format", "human")
	require.Error(t, pushErr, "rebased graph must be revalidated before the retry push")
	combined := pushOut + "\n" + pushErr.Error()
	assert.Contains(t, combined, "scope overlap")
	assert.Contains(t, combined, "push refused")
	assert.False(t, originArmatureContains(t, bareDir, "RETRY-LOCAL-TASK"),
		"retry push must not publish the dirty integrated graph")
	assert.True(t, originArmatureContains(t, bareDir, "RETRY-RIVAL-TASK"),
		"the first-push hook should have published the rival tip")
}

func TestOverlayIncludesRemoteSources_REQ_PUBLISH_VALIDATE(t *testing.T) {
	t.Parallel()
	_, repo, worktree := bootstrappedRepoWithFileOrigin(t)
	_, err := runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	srcFile := filepath.Join(t.TempDir(), "remote-only.md")
	require.NoError(t, os.WriteFile(srcFile, []byte("# remote only\n"), 0o600))
	out, err := runTrls(t, repo, "sources", "add", "--url", srcFile, "--type", "filesystem", "--title", "RemoteOnly")
	require.NoError(t, err)
	parts := strings.Fields(strings.TrimSpace(out))
	require.GreaterOrEqual(t, len(parts), 3, "expected 'added source <uuid> ...' in output: %s", out)
	sourceID := parts[2]

	_, err = runTrls(t, repo, "push-ops")
	require.NoError(t, err)

	staleID := "00000000-0000-0000-0000-000000000001"
	stale := sources.Manifest{}
	stale.Upsert(sources.SourceEntry{ID: staleID, Title: "stale-local"})
	require.NoError(t, sources.WriteManifest(filepath.Join(worktree, "sources"), stale))

	gc := adapters.New(worktree)
	valCtx, cleanup, err := contextWithPublishedOps(&config.Context{IssuesDir: worktree}, gc)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	got, err := sources.ReadManifest(filepath.Join(valCtx.IssuesDir, "sources"))
	require.NoError(t, err)
	_, ok := got.Get(sourceID)
	assert.True(t, ok, "overlay must union remote sources/manifest.json into the candidate graph")
	_, ok = got.Get(staleID)
	assert.True(t, ok, "local source entries must remain after remote overlay")
}
