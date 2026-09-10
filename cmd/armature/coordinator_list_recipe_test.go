package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorListRecipeCountsIssuesNotLines_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	path := filepath.Join(projectRootDir(t), "internal", "skillsembed", "skills", "armature-coordinator", "references", "commands.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	body := string(data)

	require.NotContains(t, body, `grep -c '"status":"done"'`,
		"list envelope is one JSON line; grep -c counts lines, not .issues entries")
	require.NotRegexp(t, `(?m)^\s*arm list.*\|\s*grep -c\b`, body,
		"do not pipe compact list output to grep -c")
	require.Regexp(t, `jq.*\.issues.*length`, body,
		"shipped coordinator count recipe must count .issues entries")
}
