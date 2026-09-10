package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorListRecipeCountsIssuesNotLines_REQ_AOC_S2_T2(t *testing.T) {
	t.Parallel()
	root := projectRootDir(t)

	commands := readEmbeddedSkill(t, root, "armature-coordinator", "references", "commands.md")
	require.NotContains(t, commands, `grep -c '"status":"done"'`,
		"list envelope is one JSON line; grep -c counts lines, not .issues entries")
	require.NotRegexp(t, `(?m)^\s*arm list.*\|\s*grep -c\b`, commands,
		"do not pipe compact list output to grep -c")
	require.Regexp(t, `jq.*\.issues.*length`, commands,
		"shipped coordinator count recipe must count .issues entries")

	skill := readEmbeddedSkill(t, root, "armature-coordinator", "SKILL.md")
	require.NotRegexp(t, `(?m)^\s*arm list.*\|\s*grep\b`, skill,
		"do not pipe compact list output to grep; the envelope is one line")
	require.Regexp(t, `jq.*\.issues\[\]\s*\|\s*select`, skill,
		"shipped coordinator recovery filter must select .issues entries")
}

func readEmbeddedSkill(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root, "internal", "skillsembed", "skills"}, parts...)...)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
