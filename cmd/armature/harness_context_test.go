package main

import (
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHarnessStructuredContext_IncludesIssueIDAndCoreLayer(t *testing.T) {
	repo := initTempRepo(t)
	run(t, repo, "git", "commit", "--allow-empty", "-m", "init")

	_, err := runTrls(t, repo, "bootstrap")
	require.NoError(t, err)
	_, err = runTrls(t, repo, "create",
		"--id", "TASK-CTX",
		"--type", "task",
		"--title", "Context payload task",
		"--dod", "Core DOD marker",
		"--scope", "cmd/armature/*",
		"--acceptance", `[{"type":"test_passes","cmd":"go test ./cmd/armature"}]`,
	)
	require.NoError(t, err)

	appCtx, err := config.ResolveContext(repo)
	require.NoError(t, err)
	appCtx.Config.TokenBudget = 1 // keep heuristic; force aggressive truncation

	rendered, err := buildHarnessStructuredContext(appCtx, "TASK-CTX")
	require.NoError(t, err)

	assert.Contains(t, rendered, `"issue_id": "TASK-CTX"`)
	assert.Contains(t, rendered, `"name": "core_spec"`)
	assert.Contains(t, rendered, "Core DOD marker")
	assert.Contains(t, rendered, "cmd/armature/*")
}
