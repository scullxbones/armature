package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestEveryAgentFacingCommandHasConformingFixture_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := sampleTree()
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)

	require.NoError(t, Lint(root, dir), "every enumerated agent-facing mode must have a conforming golden")

	modes := EnumerateModes(root)
	var paths []string
	for _, m := range modes {
		paths = append(paths, m.ID()+"/"+string(m.Channel))
	}
	require.Contains(t, paths, "list/"+string(ChannelAgentFacing))
	require.Contains(t, paths, "version/"+string(ChannelAgentFacing))
	require.NotContains(t, strings.Join(paths, ","), "field", "--field is a projection, not a mode")
}

func TestNewCommandWithoutFixtureFailsLint_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := sampleTree()
	root.AddCommand(&cobra.Command{
		Use: "mystery",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	})
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)

	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mystery")
	require.Contains(t, err.Error(), "missing golden fixture")
}

func TestProtocolOutputExemptedByClassificationNotName_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "arm", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	namedHook := &cobra.Command{
		Use:  "harness-hook",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	classified := &cobra.Command{
		Use:         "wire-protocol",
		Annotations: MarkProtocolOutput(nil),
		RunE:        func(cmd *cobra.Command, args []string) error { return nil },
	}
	root.AddCommand(namedHook, classified)

	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`)

	err := Lint(root, dir)
	require.Error(t, err, "a command named harness-hook is still agent-facing without MarkProtocolOutput")
	require.Contains(t, err.Error(), "harness-hook")
	require.NotContains(t, err.Error(), "wire-protocol", "exemption is the classification, not the Use string")

	namedHook.Annotations = MarkProtocolOutput(nil)
	require.NoError(t, Lint(root, dir), "classified Protocol Output needs no envelope fixture")

	modes := EnumerateModes(root)
	var protocol, agent []string
	for _, m := range modes {
		if m.Channel == ChannelProtocolOutput {
			protocol = append(protocol, m.Path)
		} else {
			agent = append(agent, m.Path)
		}
	}
	require.ElementsMatch(t, []string{"harness-hook", "wire-protocol"}, protocol)
	require.NotContains(t, protocol, "arm")
	require.Contains(t, agent, "")
}

func TestArtifactOutputModesUseForeignShapeAndOtherModesConform_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "arm", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	prepare := &cobra.Command{
		Use: "prepare",
		Annotations: MarkArtifactOutput(nil, ArtifactMode{
			Citation:          CitationReviewBundleSchema,
			WhenAllFlagsUnset: []string{"output"},
		}),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	apply := &cobra.Command{
		Use: "apply",
		Annotations: MarkArtifactOutput(nil, ArtifactMode{
			Citation:       CitationPlanSchema,
			WhenAnyFlagSet: []string{"schema", "example"},
		}),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	completion := &cobra.Command{
		Use: "completion",
		Annotations: MarkArtifactOutput(nil, ArtifactMode{
			Citation: CitationShellCompletionGrammar,
		}),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	root.AddCommand(prepare, apply, completion)

	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["empty home"]}`)
	writeGolden(t, dir, "prepare.output.json", `{"count":1,"bundles":[{"path":"bundle.json"}],"help":["arm review record --issue X --assessment a.json --bundle bundle.json"]}`)
	writeGolden(t, dir, "prepare.artifact.json", reviewBundleGolden)
	writeGolden(t, dir, "apply.json", `{"count":1,"issues":[{"id":"T1","type":"task","status":"open","title":"one"}],"help":["arm show <id> for outcome, scope, and acceptance"]}`)
	writeGolden(t, dir, "apply.example.artifact.json", planInstanceGolden)
	schemaPath := filepath.Join(findRepoRoot(t), "docs", "schemas", "plan.schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	require.NoError(t, err)
	writeGolden(t, dir, "apply.schema.artifact.json", string(schemaBytes))
	writeGolden(t, dir, "completion.artifact.sh", "# bash completion for arm\ncomplete -C arm arm\n")

	require.NoError(t, Lint(root, dir))

	writeGolden(t, dir, "prepare.artifact.json", `{"count":1,"bundles":[{"path":"x"}],"help":["no"]}`)
	err = Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prepare")
	require.Contains(t, err.Error(), "must not use the Agent Output Contract envelope")

	writeGolden(t, dir, "prepare.artifact.json", reviewBundleGolden)
	require.NoError(t, os.Remove(filepath.Join(dir, "prepare.output.json")))
	err = Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prepare --output")
}

func TestEnumerateModesSkipsGroupHelpAndFieldFlags(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "arm", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	group := &cobra.Command{
		Use:  "review",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	leaf := &cobra.Command{
		Use:  "show",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	leaf.Flags().String("field", "", "scalar projection")
	group.AddCommand(&cobra.Command{
		Use:  "commits",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	root.AddCommand(group, leaf)
	root.InitDefaultHelpCmd()

	modes := EnumerateModes(root)
	var ids []string
	for _, m := range modes {
		ids = append(ids, m.Path+"|"+m.Selector+"|"+string(m.Channel))
	}
	require.Contains(t, ids, "||agent-facing")
	require.Contains(t, ids, "show||agent-facing")
	require.Contains(t, ids, "review commits||agent-facing")
	require.NotContains(t, ids, "review||agent-facing")
	require.NotContains(t, strings.Join(ids, "\n"), "help")
	require.NotContains(t, strings.Join(ids, "\n"), "field")
}

func TestEnvelopeFixtureRejectsNonConformingShapes(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "arm", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	list := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	root.AddCommand(list)
	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["empty"]}`)

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "array", body: `[{"id":"X"}]`, want: "JSON object"},
		{name: "missing help", body: `{"count":1,"issues":[{"id":"X","type":"task","status":"open","title":"t"}]}`, want: "missing help"},
		{name: "count mismatch", body: `{"count":1,"issues":[],"help":["empty because filter"]}`, want: "does not equal payload"},
		{name: "literal payload", body: `{"count":0,"payload":[],"help":["empty"]}`, want: "payload"},
		{name: "outcome on list", body: `{"count":1,"issues":[{"id":"X","type":"task","status":"open","title":"t","outcome":"no"}],"help":["arm show <id>"]}`, want: "outcome"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := t.TempDir()
			writeGolden(t, d, "_root.json", `{"count":0,"issues":[],"help":["empty"]}`)
			writeGolden(t, d, "list.json", tc.body)
			err := Lint(root, d)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func sampleTree() *cobra.Command {
	root := &cobra.Command{Use: "arm", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	list := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	version := &cobra.Command{Use: "version", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	show := &cobra.Command{Use: "show", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	show.Flags().String("field", "", "projection")
	root.AddCommand(list, version, show)
	return root
}

func writeAllSampleGoldens(t *testing.T, dir string) {
	t.Helper()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`)
	writeGolden(t, dir, "list.json", `{"count":1,"issues":[{"id":"AOC-S3-T3","type":"task","status":"open","title":"Shape lint"}],"help":["arm show <id> for outcome, scope, and acceptance"]}`)
	writeGolden(t, dir, "version.json", `{"count":1,"versions":[{"version":"dev"}],"help":["arm version reports the build identity of this binary"]}`)
	writeGolden(t, dir, "show.json", `{"count":1,"issues":[{"id":"AOC-S3-T3","type":"task","status":"open","title":"Shape lint"}],"help":["arm show <id> --field <name> extracts a scalar value, never an envelope"]}`)
}

func writeGolden(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	require.NoError(t, err)
	return root
}

const reviewBundleGolden = `{
  "schema_version": 1,
  "bundle_id": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "issue": {
    "id": "AOC-S3-T3",
    "type": "task",
    "title": "Shape lint enumerated from the cobra command tree",
    "outcome": "Lint walks cobra modes and requires goldens"
  },
  "contract": {
    "definition_of_done": "A lint enumerates every registered cobra command mode",
    "scope": ["internal/output/conformance.go"],
    "acceptance": ["TestEveryAgentFacingCommandHasConformingFixture_REQ_AOC_S3_T3"]
  },
  "delivery": {
    "base_sha": "1234567890abcdef1234567890abcdef12345678",
    "head_sha": "abcdef1234567890abcdef1234567890abcdef12",
    "changed_files": ["internal/output/conformance.go"],
    "diff": "--- a/internal/output/conformance.go\n+++ b/internal/output/conformance.go\n"
  },
  "fingerprints": {
    "contract": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "delivery": "5d41402abc4b2a76b9719d911017c592250c71ed373cade4e832627b4f6c043a"
  }
}`

const planInstanceGolden = `{
  "version": 1,
  "title": "AOC shape lint",
  "issues": [
    {
      "id": "AOC-S3-T3",
      "title": "Shape lint enumerated from the cobra command tree",
      "type": "task"
    }
  ]
}`
