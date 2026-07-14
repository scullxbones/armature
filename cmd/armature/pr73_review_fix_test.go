package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoverageTargetsBuildAndPassArmBinary_REQ_TOPTIER_S1(t *testing.T) {
	root := projectRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	require.NoError(t, err)

	for _, target := range []string{"coverage", "coverage-check"} {
		t.Run(target, func(t *testing.T) {
			body := makeTargetBody(t, string(makefile), target)
			require.Contains(t, body, target+": build")
			require.Contains(t, body, "ARM_BIN=$(CURDIR)/bin/arm $(GO) test")
		})
	}
}

func TestCoordinatorRecoveryUsesTaskBranch_REQ_TOPTIER_S1(t *testing.T) {
	root := projectRoot(t)
	skill, err := os.ReadFile(filepath.Join(root, "internal", "skillsembed", "skills", "armature-coordinator", "SKILL.md"))
	require.NoError(t, err)

	require.Equal(t, 2, strings.Count(string(skill), "arm review commits TASK-ID --branch task/TASK-ID"))
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	require.NoError(t, err)
	for !fileExists(filepath.Join(root, "Makefile")) {
		parent := filepath.Dir(root)
		require.NotEqual(t, root, parent, "repository root not found")
		root = parent
	}
	return root
}

func makeTargetBody(t *testing.T, makefile, target string) string {
	t.Helper()
	start := strings.Index(makefile, target+":")
	require.NotEqual(t, -1, start, "target %q not found", target)
	rest := makefile[start:]
	if next := strings.Index(rest, "\n\n"); next >= 0 {
		return rest[:next]
	}
	return rest
}
