package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPavedRoadHelpClassification_REQ_NXTTN_S4_T1(t *testing.T) {
	root := newRootCmd()

	t.Run("everyCommandClassifiedIncludingLeaves", func(t *testing.T) {
		var unclassified []string
		var extra []string
		found := map[string]bool{}
		walkPavedRoadCommands(root, func(cmd *cobra.Command) {
			path := pavedRoadPath(cmd)
			if cmd.Name() == "help" {
				return
			}
			found[path] = true
			kind := pavedRoadKindOf(cmd)
			if kind != pavedRoadKindPaved && kind != pavedRoadKindEscape {
				unclassified = append(unclassified, pathLabel(path))
			}
			if isPavedRoadLeaf(cmd) && kind != pavedRoadKindPaved && kind != pavedRoadKindEscape {
				unclassified = append(unclassified, "leaf:"+pathLabel(path))
			}
		})
		for path := range pavedRoadCommands {
			if path == "" {
				continue
			}
			if !found[path] {
				extra = append(extra, path)
			}
		}
		require.Empty(t, unclassified, "unclassified commands (want zero leaves): %s", strings.Join(unclassified, ", "))
		require.Empty(t, extra, "metadata names commands that are not in the cobra tree: %s", strings.Join(extra, ", "))
	})

	t.Run("pipelineStepsArePavedAndPresent", func(t *testing.T) {
		for _, step := range pavedRoadPipeline {
			require.NotEmpty(t, step.Commands, "step %s must map to commands", step.ID)
			for _, path := range step.Commands {
				cmd := findPavedRoadCommand(root, path)
				require.NotNil(t, cmd, "pipeline %s missing command %q", step.ID, path)
				require.Equal(t, pavedRoadKindPaved, pavedRoadKindOf(cmd),
					"pipeline command %q must be paved", path)
			}
		}
	})

	t.Run("helpMarksEscapeHatchInExistingGroups", func(t *testing.T) {
		parents := []*cobra.Command{root}
		walkPavedRoadCommands(root, func(cmd *cobra.Command) {
			if cmd.HasAvailableSubCommands() {
				parents = append(parents, cmd)
			}
		})
		for _, parent := range parents {
			help := commandHelp(t, parent)
			require.NotContains(t, help, "Escape Hatch Commands",
				"%s help must keep escape hatches in existing groups", parent.CommandPath())
			for _, child := range parent.Commands() {
				if !child.IsAvailableCommand() {
					continue
				}
				if child.Name() == "help" {
					continue
				}
				kind := pavedRoadKindOf(child)
				line := helpLineFor(help, child.Name())
				require.NotEmpty(t, line, "expected %s in %s help", child.Name(), parent.CommandPath())
				switch kind {
				case pavedRoadKindEscape:
					require.Contains(t, line, escapeHatchHelpMarker,
						"%s help line for %s must include %s: %q", parent.CommandPath(), child.Name(), escapeHatchHelpMarker, line)
				case pavedRoadKindPaved:
					require.NotContains(t, line, escapeHatchHelpMarker,
						"%s help line for paved %s must not include the marker: %q", parent.CommandPath(), child.Name(), line)
				}
			}
		}
		walkPavedRoadCommands(root, func(cmd *cobra.Command) {
			if pavedRoadKindOf(cmd) != pavedRoadKindEscape {
				return
			}
			require.Contains(t, cmd.Short, escapeHatchHelpMarker, "%s Short", pavedRoadPath(cmd))
		})
	})

	t.Run("generatedDocMatchesMetadata", func(t *testing.T) {
		got := generatePavedRoadDoc(root)
		require.Contains(t, got, "## Pipeline")
		require.Contains(t, got, "### Unclassified\n\nNone.")
		require.Contains(t, got, "## Defaults audit")
		require.Contains(t, got, "--skip-delivery-gate")
		for _, step := range pavedRoadPipeline {
			require.Contains(t, got, step.Title)
			for _, path := range step.Commands {
				require.Contains(t, got, "`arm "+path+"`")
			}
		}

		_, thisFile, _, ok := runtime.Caller(0)
		require.True(t, ok)
		docPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "paved-road.md")
		if os.Getenv("UPDATE_PAVED_ROAD") == "1" {
			require.NoError(t, os.WriteFile(docPath, []byte(got), 0o644))
		}
		want, err := os.ReadFile(docPath)
		require.NoError(t, err, "docs/paved-road.md must exist; regenerate with UPDATE_PAVED_ROAD=1")
		require.Equal(t, string(want), got,
			"docs/paved-road.md drifted from metadata; regenerate with UPDATE_PAVED_ROAD=1")
	})
}

func TestPavedRoadUnclassifiedLeafFails(t *testing.T) {
	root := &cobra.Command{Use: "arm"}
	root.AddCommand(&cobra.Command{Use: "mystery", Short: "unclassified leaf"})
	applyPavedRoadMetadata(root)
	var unclassified []string
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		path := pavedRoadPath(cmd)
		if path == "" || cmd.Name() == "help" {
			return
		}
		if pavedRoadKindOf(cmd) == "" {
			unclassified = append(unclassified, path)
		}
	})
	require.Contains(t, unclassified, "mystery")
}

func pathLabel(path string) string {
	if path == "" {
		return "arm"
	}
	return path
}

func findPavedRoadCommand(root *cobra.Command, path string) *cobra.Command {
	var found *cobra.Command
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		if pavedRoadPath(cmd) == path {
			found = cmd
		}
	})
	return found
}

func commandHelp(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	require.NoError(t, cmd.Help())
	return buf.String()
}

func helpLineFor(help, name string) string {
	re := regexp.MustCompile(`(?m)^[ \t]+` + regexp.QuoteMeta(name) + `[ \t]+.+$`)
	return strings.TrimSpace(re.FindString(help))
}
