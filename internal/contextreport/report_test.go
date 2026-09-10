package contextreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ctxpkg "github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

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

	root := moduleRoot(t)
	report, err := Collect(root)
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
	assert.NotContains(t, report.EstimationMethod, "AOC-S3-T2")

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

	root := moduleRoot(t)
	report, err := Collect(root)
	require.NoError(t, err)

	bundle := artifactByPath(t, report, "render-context.bundle")
	assert.Equal(t, ClassBundle, bundle.Class)
	assert.Greater(t, bundle.Bytes, 0)
	assert.Equal(t, bundle.Bytes/BytesPerToken, bundle.EstimatedTokens)

	opsPath := filepath.Join(root, "internal", "contextreport", "testdata", "graph", "ops.jsonl")
	allOps, err := loadFixtureOps(opsPath)
	require.NoError(t, err)
	state := materialize.NewState()
	for _, op := range allOps {
		require.NoError(t, state.ApplyOp(op))
	}
	workspace := filepath.Join(root, "internal", "contextreport", "testdata", "graph", "workspace")
	assembled, err := ctxpkg.Assemble(FixtureShowIssue, state, &ctxpkg.OSFileReader{Root: workspace})
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

func TestCollectErrorsWhenFixtureOpsMissing(t *testing.T) {
	t.Parallel()
	_, err := Collect(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ops.jsonl")
}

func TestCollectErrorsWhenFixtureOpsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "internal", "contextreport", "testdata", "graph", "ops.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("\n"), 0o644))
	_, err := Collect(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestCollectErrorsWhenFixtureOpsCorrupt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "internal", "contextreport", "testdata", "graph", "ops.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("not-json\n"), 0o644))
	_, err := Collect(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse fixture ops")
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
