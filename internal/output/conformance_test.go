package output

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEveryAgentFacingCommandHasConformingFixture_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := sampleTree()
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)
	attachBind(root, BindWriter())

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
	root.Children = append(root.Children, &Command{Name: "mystery", HasRun: true})
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)
	attachBind(root, BindWriter())

	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mystery")
	require.Contains(t, err.Error(), "missing golden fixture")
}

func TestProtocolOutputExemptedByClassificationNotName_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{Name: "harness-hook", HasRun: true},
			{Name: "wire-protocol", HasRun: true, Annotations: MarkProtocolOutput(nil)},
		},
	}
	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`)
	attachBind(root, BindWriter())

	err := Lint(root, dir)
	require.Error(t, err, "a command named harness-hook is still agent-facing without MarkProtocolOutput")
	require.Contains(t, err.Error(), "harness-hook")
	require.NotContains(t, err.Error(), "wire-protocol", "exemption is the classification, not the Use string")

	root.Children[0].Annotations = MarkProtocolOutput(nil)
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
	require.Contains(t, agent, "")
}

func TestArtifactOutputModesUseForeignShapeAndOtherModesConform_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()

	root := &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{
				Name:   "prepare",
				HasRun: true,
				Annotations: MarkArtifactOutput(nil, ArtifactMode{
					Citation:          CitationReviewBundleSchema,
					WhenAllFlagsUnset: []string{"output"},
				}),
			},
			{
				Name:   "apply",
				HasRun: true,
				Annotations: MarkArtifactOutput(nil, ArtifactMode{
					Citation:       CitationPlanSchema,
					WhenAnyFlagSet: []string{"schema", "example"},
				}),
			},
			{
				Name:   "completion",
				HasRun: true,
				Annotations: MarkArtifactOutput(nil, ArtifactMode{
					Citation: CitationShellCompletionGrammar,
				}),
			},
		},
	}

	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["empty home"]}`)
	writeGolden(t, dir, "prepare.output.json", `{
		"count":1,
		"bundles":[{"path":"bundle.json"}],
		"help":["arm review record --issue X --assessment a.json --bundle bundle.json"]
	}`)
	writeGolden(t, dir, "prepare.artifact.json", reviewBundleGolden)
	writeGolden(t, dir, "apply.json", `{
		"count":1,
		"issues":[{"id":"T1","type":"task","status":"open","title":"one"}],
		"help":["arm show <id> for outcome, scope, and acceptance"]
	}`)
	writeGolden(t, dir, "apply.example.artifact.json", planInstanceGolden)
	writeGolden(t, dir, "apply.schema.artifact.json", `{
		"$schema":"http://json-schema.org/draft-07/schema#",
		"title":"Armature Plan Schema",
		"type":"object",
		"properties":{"version":{"type":"integer"}}
	}`)
	writeGolden(t, dir, "completion.artifact.sh", "# bash completion for arm\ncomplete -C arm arm\n")
	attachBind(root, BindWriter())

	require.NoError(t, Lint(root, dir))

	writeGolden(t, dir, "prepare.artifact.json", `{"count":1,"bundles":[{"path":"x"}],"help":["no"]}`)
	err := Lint(root, dir)
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

	root := &Command{
		Name:             "arm",
		HasRun:           true,
		HasAvailableSubs: true,
		Children: []*Command{
			{
				Name:             "review",
				HasRun:           true,
				HasAvailableSubs: true,
				Children: []*Command{
					{Name: "commits", HasRun: true},
				},
			},
			{Name: "show", HasRun: true},
			{Name: "help", HasRun: true},
		},
	}

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

	root := &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{Name: "list", HasRun: true},
		},
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "array", body: `[{"id":"X"}]`, want: "JSON object"},
		{name: "missing help", body: `{"count":1,"issues":[{"id":"X","type":"task","status":"open","title":"t"}]}`, want: "missing help"},
		{name: "count mismatch", body: `{"count":1,"issues":[],"help":["empty because filter"]}`, want: "does not equal payload"},
		{name: "literal payload", body: `{"count":0,"payload":[],"help":["empty"]}`, want: "payload"},
		{
			name: "outcome on list",
			body: `{"count":1,"issues":[{"id":"X","type":"task","status":"open","title":"t","outcome":"no"}],"help":["arm show <id>"]}`,
			want: "outcome",
		},
		{
			name: "non-string list fields",
			body: `{"count":1,"issues":[{"id":null,"type":[],"status":1,"title":{}}],"help":["arm show <id>"]}`,
			want: "must be a JSON string",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := t.TempDir()
			writeGolden(t, d, "_root.json", `{"count":0,"issues":[],"help":["empty"]}`)
			writeGolden(t, d, "list.json", tc.body)
			attachBind(root, BindWriter())
			err := Lint(root, d)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestUnboundHandlerWriterFailsLint_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	root := sampleTree()
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)
	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "handler writer is not bound")
}

func TestHandlerArrayDoesNotPassWithUnrelatedGolden_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	root := sampleTree()
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)
	attachBind(root, func(_ Mode, _ []byte, stdout, stderr io.Writer) error {
		_, err := stdout.Write([]byte("[]\n"))
		return err
	})
	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match captured handler stdout")
}

func TestStructuredStderrFailsLint_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	root := sampleTree()
	dir := t.TempDir()
	writeAllSampleGoldens(t, dir)
	attachBind(root, func(_ Mode, golden []byte, stdout, stderr io.Writer) error {
		if _, err := stdout.Write(golden); err != nil {
			return err
		}
		_, err := stderr.Write([]byte(`{"issues":[]}`))
		return err
	})
	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stderr")
}

func TestReviewBundleFixtureMustSatisfyCitedSchema_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	root := &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{
				Name:   "prepare",
				HasRun: true,
				Annotations: MarkArtifactOutput(nil, ArtifactMode{
					Citation: CitationReviewBundleSchema,
				}),
			},
		},
	}
	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["empty home"]}`)
	writeGolden(t, dir, "prepare.artifact.json", `{
		"schema_version":null,
		"bundle_id":null,
		"issue":null,
		"contract":null,
		"delivery":null,
		"fingerprints":null
	}`)
	attachBind(root, BindWriter())
	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "review bundle")
}

func TestInvalidShellCompletionFixtureFailsLint_REQ_AOC_S3_T3(t *testing.T) {
	t.Parallel()
	root := &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{
				Name:   "completion",
				HasRun: true,
				Annotations: MarkArtifactOutput(nil, ArtifactMode{
					Citation: CitationShellCompletionGrammar,
				}),
			},
		},
	}
	dir := t.TempDir()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["empty home"]}`)
	writeGolden(t, dir, "completion.artifact.sh", "not a completion script")
	attachBind(root, BindWriter())
	err := Lint(root, dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "completion-script grammar")
}

func sampleTree() *Command {
	return &Command{
		Name:   "arm",
		HasRun: true,
		Children: []*Command{
			{Name: "list", HasRun: true},
			{Name: "version", HasRun: true},
			{Name: "show", HasRun: true},
		},
	}
}

func writeAllSampleGoldens(t *testing.T, dir string) {
	t.Helper()
	writeGolden(t, dir, "_root.json", `{"count":0,"issues":[],"help":["not an Armature repository","run arm bootstrap in a git repository"]}`)
	writeGolden(t, dir, "list.json", `{
		"count":1,
		"issues":[{"id":"AOC-S3-T3","type":"task","status":"open","title":"Shape lint"}],
		"help":["arm show <id> for outcome, scope, and acceptance"]
	}`)
	writeGolden(t, dir, "version.json", `{"count":1,"versions":[{"version":"dev"}],"help":["arm version reports the build identity of this binary"]}`)
	writeGolden(t, dir, "show.json", `{
		"count":1,
		"issues":[{"id":"AOC-S3-T3","type":"task","status":"open","title":"Shape lint"}],
		"help":["arm show <id> --field <name> extracts a scalar value, never an envelope"]
	}`)
}

func writeGolden(t *testing.T, dir, name, body string) {
	t.Helper()
	if name != filepath.Base(name) {
		t.Fatalf("golden name %q must be a basename", name)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func attachBind(root *Command, bind func(Mode, []byte, io.Writer, io.Writer) error) {
	if root == nil {
		return
	}
	root.Bind = bind
	for _, child := range root.Children {
		attachBind(child, bind)
	}
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
