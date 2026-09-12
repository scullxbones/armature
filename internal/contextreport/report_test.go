package contextreport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	ctxpkg "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func artifactByPath(t *testing.T, report Report, path string) Artifact {
	t.Helper()
	for _, a := range report.Artifacts {
		if a.Path == path {
			return a
		}
	}
	t.Fatalf("artifact %q not found in %+v", path, report.Artifacts)
	return Artifact{}
}

func TestContextReportPricesMainPathCLI_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Parallel()

	report, err := Collect()
	require.NoError(t, err)

	want := []string{"list", "ready", "show", "render-context", "review"}
	got := map[string]Artifact{}
	for _, a := range report.Artifacts {
		if a.Class != ClassInvocation {
			continue
		}
		got[a.Path] = a
	}
	require.Len(t, got, len(want), "exactly the main-path CLI invocations")
	for _, name := range want {
		art, ok := got[name]
		require.True(t, ok, "main-path command %q must be priced", name)
		assert.Greater(t, art.Bytes, 0, "command %q should have structured stdout", name)
		assert.Equal(t, art.Bytes/BytesPerToken, art.EstimatedTokens, "bytes/4 for %q", name)
	}

	assert.Equal(t, EstimationMethod, report.EstimationMethod)
	assert.Contains(t, report.EstimationMethod, "bytes/4")
	assert.Contains(t, report.EstimationMethod, "token_budget")
	assert.Contains(t, report.EstimationMethod, "testdata")
	assert.Contains(t, report.EstimationMethod, "WriteShowEnvelope")

	var summedBytes, summedTokens int
	for _, a := range report.Artifacts {
		summedBytes += a.Bytes
		summedTokens += a.EstimatedTokens
		assert.NotEqual(t, "skill", a.Class)
		assert.NotEqual(t, "glossary", a.Class)
		assert.NotContains(t, a.Path, "skillsembed")
	}
	assert.Equal(t, summedBytes, report.TotalBytes)
	assert.Equal(t, summedTokens, report.TotalEstimatedTokens)

	human := RenderHuman(report)
	assert.Contains(t, human, "bytes/4")
	assert.Contains(t, human, "token_budget")
	assert.Contains(t, human, "list")
	assert.Contains(t, human, "ready")
	assert.Contains(t, human, "show")
	assert.Contains(t, human, "render-context")
	assert.Contains(t, human, "review")
	assert.NotContains(t, human, "armature-coordinator")

	raw, err := RenderJSON(report)
	require.NoError(t, err)
	var decoded Report
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, report.EstimationMethod, decoded.EstimationMethod)
	assert.Contains(t, string(raw), "bytes/4")
	assert.Contains(t, string(raw), `"class": "invocation"`)
}

func TestContextReportPricesRenderContextFixture_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Parallel()

	report, err := Collect()
	require.NoError(t, err)

	bundle := artifactByPath(t, report, "render-context.bundle")
	assert.Equal(t, ClassBundle, bundle.Class)
	assert.Greater(t, bundle.Bytes, 0)
	assert.Equal(t, bundle.Bytes/BytesPerToken, bundle.EstimatedTokens)

	state, _, err := replayFixtureState()
	require.NoError(t, err)
	assembled, err := ctxpkg.Assemble(FixtureShowIssue, state, embedFileReader{})
	require.NoError(t, err)
	raw, err := ctxpkg.RenderAgent(assembled)
	require.NoError(t, err)
	assert.Equal(t, len(raw), bundle.Bytes, "bundle row is the fixture render-context artifact")
	assert.Contains(t, raw, FixtureShowIssue)
	assert.Contains(t, raw, "hello.go")

	invocation := artifactByPath(t, report, "render-context")
	assert.Equal(t, ClassInvocation, invocation.Class)
	assert.Greater(t, invocation.Bytes, 0)
}

func TestCollectUsesEmbeddedFixturesIndependentOfRepo_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Parallel()

	report, err := Collect()
	require.NoError(t, err, "fixtures must be embedded; Collect must not read --repo testdata")
	assert.NotEmpty(t, report.Artifacts)
	assert.Greater(t, report.TotalBytes, 0)
}

func TestContextReportShowMeasuresAOCEnvelope_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Parallel()

	report, err := Collect()
	require.NoError(t, err)
	show := artifactByPath(t, report, "show")

	state, _, err := replayFixtureState()
	require.NoError(t, err)
	issue := state.Issues[FixtureShowIssue]
	require.NotNil(t, issue)

	var human bytes.Buffer
	require.NoError(t, output.RenderIssue(&human, issue))
	got, err := measureShow(state)
	require.NoError(t, err)

	row := output.MarshalIssue(issue)
	trunc := output.TruncateShowIssue(&row)
	var envelope bytes.Buffer
	require.NoError(t, output.WriteShowEnvelope(&envelope, []string{issue.ID}, []output.IssueJSON{row}, trunc))

	assert.Equal(t, len(got), show.Bytes)
	assert.Equal(t, envelope.Bytes(), got,
		"show row must price the json/agent writeShowEnvelope payload")
	assert.NotContains(t, string(got), "Spend-to-date:",
		"FormatSpend stays on human show; json/agent show does not emit it")
	assert.NotEqual(t, human.Len(), show.Bytes,
		"show row must not price the human RenderIssue path")
	assert.False(t, json.Valid(bytes.TrimSpace(human.Bytes())),
		"human show is prose, not a JSON object")
	assert.True(t, json.Valid(bytes.TrimSpace(got)))
	assert.Contains(t, human.String(), "ID:")
}

func TestParseFixtureOpsEmpty(t *testing.T) {
	t.Parallel()
	_, err := parseFixtureOps("ops.jsonl", []byte("\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseFixtureOpsCorrupt(t *testing.T) {
	t.Parallel()
	_, err := parseFixtureOps("ops.jsonl", []byte("not-json\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse fixture ops")
}

func TestEmbedFileReaderRejectsEscapingPath(t *testing.T) {
	t.Parallel()
	_, err := embedFileReader{}.ReadFile("../ops.jsonl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fixture path")
}

func TestEstimateTokensIntegerDivision(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, EstimateTokens(3))
	assert.Equal(t, 1, EstimateTokens(4))
	assert.Equal(t, 1, EstimateTokens(5))
}

func TestRenderHumanIncludesTotals(t *testing.T) {
	t.Parallel()
	report := finalize([]Artifact{price("list", ClassInvocation, []byte("abcd"))})
	human := RenderHuman(report)
	assert.Contains(t, human, "list")
	assert.Contains(t, human, "TOTAL")
	assert.True(t, strings.Contains(human, "4"))
}
