package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderContext_IncludesIssueIDAndCoreLayer(t *testing.T) {
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

	rendered, err := runTrls(t, repo, "render-context", "--issue", "TASK-CTX", "--format", "agent", "--budget", "1")
	require.NoError(t, err)

	assert.Contains(t, rendered, `"issue_id": "TASK-CTX"`)
	assert.Contains(t, rendered, `"name": "core_spec"`)
	assert.Contains(t, rendered, "Core DOD marker")
	assert.Contains(t, rendered, "cmd/armature/*")
}
