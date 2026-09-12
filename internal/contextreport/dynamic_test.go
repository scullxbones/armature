package contextreport

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scullxbones/armature/internal/output"
	"github.com/scullxbones/armature/internal/ready"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextReportPricesDynamicPayloads_REQ_AOC_S3_T2(t *testing.T) {
	t.Parallel()

	report, err := Collect()
	require.NoError(t, err)

	got := map[string]Artifact{}
	for _, a := range report.Artifacts {
		got[a.Path] = a
	}
	for _, path := range DynamicInvocationPaths {
		art, ok := got[path]
		require.True(t, ok, "dynamic command %q must be priced", path)
		assert.Equal(t, ClassInvocation, art.Class, path)
		assert.Greater(t, art.Bytes, 0, "command %q should have structured stdout", path)
		assert.Equal(t, art.Bytes/BytesPerToken, art.EstimatedTokens, "bytes/4 for %q", path)
	}
	for _, path := range AOCEnvelopePaths {
		require.Contains(t, got, path)
	}

	state, index, err := replayFixtureState()
	require.NoError(t, err)

	list, err := measureList(index)
	require.NoError(t, err)
	assert.Equal(t, len(list), got["list"].Bytes)
	assertAOCEnvelope(t, list, "issues")

	now := time.Unix(fixtureReadyNow, 0)
	readyPayload, err := measureReady(index, state, now)
	require.NoError(t, err)
	assert.Equal(t, len(readyPayload), got["ready"].Bytes)
	assertAOCEnvelope(t, readyPayload, "issues")
	assert.Contains(t, string(readyPayload), "expired_claims")

	show, err := measureShow(state)
	require.NoError(t, err)
	assert.Equal(t, len(show), got["show"].Bytes)
	assertAOCEnvelope(t, show, "issues")
	assert.NotContains(t, string(show), "Spend-to-date:")

	render, bundle, err := measureRenderContext(state, embedFileReader{})
	require.NoError(t, err)
	assert.Equal(t, len(render), got["render-context"].Bytes)
	raw := bytes.TrimSpace(render)
	require.True(t, json.Valid(raw), "render-context json/agent is structured JSON")
	assert.Contains(t, string(raw), FixtureShowIssue)
	assert.Greater(t, len(bundle), 0)

	assert.Contains(t, report.EstimationMethod, "bytes/4")
	assert.Contains(t, report.EstimationMethod, "AOC")
	assert.Contains(t, report.EstimationMethod, "WriteShowEnvelope")
}

func TestDynamicPricingIsFixtureDeterministic_REQ_AOC_S3_T2(t *testing.T) {
	t.Parallel()

	first, err := Collect()
	require.NoError(t, err)
	second, err := Collect()
	require.NoError(t, err)
	require.Equal(t, first, second, "fixture graph must produce identical reports")

	root := moduleRoot(t)
	goldenDir := filepath.Join(root, "internal", "contextreport", "testdata", "dynamic")
	state, index, err := replayFixtureState()
	require.NoError(t, err)

	payloads := map[string][]byte{}
	list, err := measureList(index)
	require.NoError(t, err)
	payloads["list"] = list
	readyPayload, err := measureReady(index, state, time.Unix(fixtureReadyNow, 0))
	require.NoError(t, err)
	payloads["ready"] = readyPayload
	show, err := measureShow(state)
	require.NoError(t, err)
	payloads["show"] = show
	render, _, err := measureRenderContext(state, embedFileReader{})
	require.NoError(t, err)
	payloads["render-context"] = render

	for _, path := range DynamicInvocationPaths {
		out := filepath.Join(goldenDir, path+".json")
		if os.Getenv("WRITE_DYNAMIC_GOLDENS") == "1" {
			require.NoError(t, os.MkdirAll(goldenDir, 0o755))
			require.NoError(t, os.WriteFile(out, payloads[path], 0o644))
			continue
		}
		golden, err := os.ReadFile(out) //nolint:gosec // checked-in fixture
		require.NoError(t, err, "golden for %s", path)
		assert.Equal(t, golden, payloads[path],
			"priced %s payload must match testdata/dynamic/%s.json", path, path)
	}
}

func TestDynamicBudgetsEnforcePromisedCaps_REQ_AOC_S3_T2(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect()
	require.NoError(t, err)

	budgets, err := LoadBudgets(filepath.Join(root, filepath.FromSlash(BudgetsRelPath)))
	require.NoError(t, err)
	require.NoError(t, Enforce(report, budgets))
	require.NoError(t, ExplicitTargets(report, budgets),
		"dynamic rows must name a promise, not seed at measured size")

	byPath := map[string]Budget{}
	for _, row := range budgets.Artifacts {
		byPath[row.Path] = row
	}
	for _, path := range DynamicInvocationPaths {
		row, ok := byPath[path]
		require.True(t, ok, "budget row for dynamic path %q", path)
		art := artifactByPath(t, report, path)
		assert.Equal(t, row.TargetBytes, row.MaxBytes,
			"%q max_bytes must be the named promise, not a holding cap above target", path)
		assert.Less(t, art.Bytes, row.TargetBytes,
			"%q must be under the promised cap (%d), not seeded at measured size", path, row.TargetBytes)
		assert.NotEqual(t, art.Bytes, row.MaxBytes,
			"%q must not greenwash by setting max_bytes to today's measured size", path)
	}
}

func TestMeasureShowMatchesWriteShowEnvelope_REQ_AOC_S3_T2(t *testing.T) {
	t.Parallel()

	state, _, err := replayFixtureState()
	require.NoError(t, err)
	got, err := measureShow(state)
	require.NoError(t, err)

	issue := state.Issues[FixtureShowIssue]
	require.NotNil(t, issue)
	row := output.MarshalIssue(issue)
	trunc := output.TruncateShowIssue(&row)
	var want bytes.Buffer
	require.NoError(t, output.WriteShowEnvelope(&want, []string{issue.ID}, []output.IssueJSON{row}, trunc))
	assert.Equal(t, want.Bytes(), got)

	assert.False(t, bytes.Contains(got, []byte("\n  ")), "show envelope is compact")
}

func TestMeasureReadyPassesFrozenClockToComputeReady(t *testing.T) {
	t.Parallel()

	state, index, err := replayFixtureState()
	require.NoError(t, err)
	now := time.Unix(fixtureReadyNow, 0)
	got, err := measureReady(index, state, now)
	require.NoError(t, err)

	entries := ready.ComputeReady(index, state.Issues, "", now.Unix())
	expired := ready.ExpiredClaims(state.Issues, now)
	var want bytes.Buffer
	require.NoError(t, output.WriteReadyEnvelope(&want, entries, nil, false, expired, "", ""))
	assert.Equal(t, want.Bytes(), got)
}

func assertAOCEnvelope(t *testing.T, payload []byte, payloadKey string) {
	t.Helper()
	raw := bytes.TrimSpace(payload)
	require.True(t, json.Valid(raw))
	require.True(t, bytes.HasPrefix(raw, []byte("{")), "AOC envelope is a JSON object")
	assert.False(t, bytes.Contains(payload, []byte("\n  ")), "AOC envelope is compact, not pretty")

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "count")
	require.Contains(t, decoded, payloadKey)
	require.Contains(t, decoded, "help")
	require.NotContains(t, decoded, "payload")
}
