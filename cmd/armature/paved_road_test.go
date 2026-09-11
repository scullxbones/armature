package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		require.Contains(t, got, "`arm sources accept-citation`")
		require.Contains(t, got, "`arm help`")
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

	t.Run("helpCommandIsInstalledAndClassified", func(t *testing.T) {
		help := findPavedRoadCommand(root, "help")
		require.NotNil(t, help, "cobra help must be installed before paved-road metadata")
		require.Equal(t, pavedRoadKindPaved, pavedRoadKindOf(help))
	})

	t.Run("defaultsAuditCoversForceAndSkipFlags", func(t *testing.T) {
		listed := map[string]bool{}
		for _, d := range pavedRoadDefaultsAudit {
			if d.Flag != "--force" && !strings.HasPrefix(d.Flag, "--skip-") {
				continue
			}
			for _, path := range d.Commands {
				listed[path+"\t"+d.Flag] = true
			}
		}
		var missing []string
		walkPavedRoadCommands(root, func(cmd *cobra.Command) {
			path := pavedRoadPath(cmd)
			if path == "" {
				return
			}
			cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
				if f.Name != "force" && !strings.HasPrefix(f.Name, "skip-") {
					return
				}
				key := path + "\t--" + f.Name
				if !listed[key] {
					missing = append(missing, "arm "+path+" --"+f.Name)
				}
			})
		})
		require.Empty(t, missing, "defaults audit omitted force/skip flags: %s", strings.Join(missing, ", "))
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

func isPavedRoadLeaf(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() {
			return false
		}
	}
	return cmd.Parent() != nil || cmd.Name() == "arm"
}

func pavedRoadKindOf(cmd *cobra.Command) string {
	if cmd == nil || cmd.Annotations == nil {
		return ""
	}
	return cmd.Annotations[pavedRoadAnnotationKey]
}

func generatePavedRoadDoc(root *cobra.Command) string {
	var b bytes.Buffer
	b.WriteString("# The Paved Road\n\n")
	b.WriteString("Generated from cobra command metadata in `cmd/armature/main.go`. Do not edit by hand.\n")
	b.WriteString("Regenerate: `UPDATE_PAVED_ROAD=1 go test ./cmd/armature -run TestPavedRoadHelpClassification_REQ_NXTTN_S4_T1`.\n\n")
	b.WriteString("## What this is\n\n")
	b.WriteString("The paved road is the one blessed end-to-end pipeline: bootstrap, plan/decompose, wave dispatch, work, review, sync.\n")
	b.WriteString("Agents should follow that pipeline. Everything else is an escape hatch.\n")
	b.WriteString("Escape-hatch commands stay in their existing `--help` groups. They are marked `[escape hatch]` inline.\n")
	b.WriteString("There is no separate escape-hatch group.\n\n")

	b.WriteString("## Pipeline\n\n")
	for i, step := range pavedRoadPipeline {
		fmt.Fprintf(&b, "### %d. %s\n\n%s\n\n", i+1, step.Title, step.Description)
		for _, path := range step.Commands {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
		}
		b.WriteString("\n")
	}

	var paved []string
	var escape []string
	var unclassified []string
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		path := pavedRoadPath(cmd)
		if path == "" {
			return
		}
		kind := pavedRoadKindOf(cmd)
		switch kind {
		case pavedRoadKindPaved:
			paved = append(paved, path)
		case pavedRoadKindEscape:
			escape = append(escape, path)
		default:
			unclassified = append(unclassified, path)
		}
	})
	sort.Strings(paved)
	sort.Strings(escape)
	sort.Strings(unclassified)

	b.WriteString("## Command classification\n\n")
	b.WriteString("Every cobra command, including leaf subcommands, is paved or escape-hatch.\n")
	b.WriteString("Zero unclassified leaf commands is a gate.\n\n")
	b.WriteString("### Paved\n\n")
	for _, path := range paved {
		step := pavedRoadStepFor(path)
		if step == "" {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
			continue
		}
		fmt.Fprintf(&b, "- `arm %s` — %s\n", path, step)
	}
	b.WriteString("\n### Escape hatch\n\n")
	for _, path := range escape {
		note := pavedRoadCommands[path].Note
		if note == "" {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
			continue
		}
		fmt.Fprintf(&b, "- `arm %s` — %s\n", path, note)
	}
	b.WriteString("\n### Unclassified\n\n")
	if len(unclassified) == 0 {
		b.WriteString("None. Every registered command is classified.\n\n")
	} else {
		for _, path := range unclassified {
			fmt.Fprintf(&b, "- `arm %s`\n", path)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Defaults audit\n\n")
	b.WriteString("These flags exist because the road was not always the default.\n")
	b.WriteString("Skip and force flags stay listed here. They are not the paved-road invocation.\n\n")
	b.WriteString("| Flag | Command | Why it exists |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, d := range pavedRoadDefaultsAudit {
		cmds := make([]string, 0, len(d.Commands))
		for _, c := range d.Commands {
			cmds = append(cmds, "`arm "+c+"`")
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", d.Flag, strings.Join(cmds, ", "), d.Reason)
	}
	b.WriteString("\n")
	return b.String()
}

func pavedRoadStepFor(path string) string {
	for _, step := range pavedRoadPipeline {
		for _, c := range step.Commands {
			if c == path {
				return step.Title
			}
		}
	}
	return ""
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
