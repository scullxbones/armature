package contextreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func runtimeBudgetFixture(overPath string, measured, max, target int) (Report, budgetFile) {
	mk := func(path, class string, bytes, maxBytes, targetBytes int) (artifact, budget) {
		art := artifact{
			Path:            path,
			Class:           class,
			Bytes:           bytes,
			EstimatedTokens: estimateTokens(bytes),
		}
		row := budget{
			Path:        path,
			Class:       class,
			MaxBytes:    maxBytes,
			TargetBytes: targetBytes,
		}
		return art, row
	}

	paths := []struct {
		path, class        string
		bytes, max, target int
	}{
		{"list", classInvocation, 8, 16, 16},
		{"ready", classInvocation, 8, 16, 16},
		{"show", classInvocation, 8, 16, 16},
		{"render-context", classInvocation, 8, 16, 16},
		{"review", classInvocation, 8, 16, 16},
		{"render-context.bundle", classBundle, 8, 16, 16},
	}

	var report Report
	var budgets budgetFile
	for _, p := range paths {
		bytes, maxBytes, targetBytes := p.bytes, p.max, p.target
		if p.path == overPath {
			bytes, maxBytes, targetBytes = measured, max, target
		}
		art, row := mk(p.path, p.class, bytes, maxBytes, targetBytes)
		report.Artifacts = append(report.Artifacts, art)
		budgets.Artifacts = append(budgets.Artifacts, row)
	}
	return finalize(report.Artifacts), budgets
}

func TestBudgetGateFailsOverBudget_REQ_NXTTN_S3_T2(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("list", 8, 4, 4)
	err := enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invocation")
	assert.Contains(t, err.Error(), "list")
	assert.Contains(t, err.Error(), "8 bytes > budget 4")

	var gate *gateError
	require.ErrorAs(t, err, &gate)
	require.Len(t, gate.OverBudget, 1)
	assert.Equal(t, classInvocation, gate.OverBudget[0].Class)
	assert.Equal(t, "list", gate.OverBudget[0].Path)
	assert.Equal(t, 8, gate.OverBudget[0].Bytes)
	assert.Equal(t, 4, gate.OverBudget[0].MaxBytes)
	assert.Equal(t, []string{classInvocation}, gate.overBudgetClasses())
}

func TestRuntimeBudgetCapsAreExplicitTargets_REQ_NXTTN_S3_T2(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect()
	require.NoError(t, err)

	budgets, err := loadBudgets(filepath.Join(root, filepath.FromSlash(budgetsRelPath)))
	require.NoError(t, err)

	require.NoError(t, explicitTargets(report, budgets),
		"checked-in runtime budgets must name a promise, not seed silently at measured size")

	docs, err := os.ReadFile(filepath.Join(root, "docs", "context-budgets.md"))
	require.NoError(t, err)
	body := string(docs)

	wantPaths := []string{"list", "ready", "show", "render-context", "review", "render-context.bundle"}
	got := map[string]budget{}
	for _, row := range budgets.Artifacts {
		got[row.Path] = row
	}
	require.Len(t, got, len(wantPaths))
	for _, path := range wantPaths {
		row, ok := got[path]
		require.True(t, ok, "budget row %q", path)
		assert.Greater(t, row.TargetBytes, 0, "target_bytes is the named promise for %q", path)
		assert.Contains(t, body, path)
		assert.Contains(t, body, strconv.Itoa(row.TargetBytes),
			"docs must state the target_bytes promise for %q", path)

		art := artifactByPath(t, report, path)
		assert.NotEqual(t, art.Bytes, row.TargetBytes,
			"%q target_bytes must not be today's measured size pretending to be a promise", path)
		if row.MaxBytes == art.Bytes && row.TargetBytes == art.Bytes {
			t.Errorf("%q is a measured-size-only seed (PR 162 failure mode)", path)
		}
		if art.Bytes > row.TargetBytes {
			assert.True(t, hasDatedTrimPlan(body, path),
				"dated trim plan heading for %q required when measured bytes exceed target", path)
		}
	}
	assert.NotContains(t, body, "internal/skillsembed/skills")
}

func TestHasDatedTrimPlanRequiresPathSpecificHeading(t *testing.T) {
	t.Parallel()

	weak := `# Context budgets

trim plan

Measured fixture sizes on 2026-09-10.

| path | class | target_bytes | max_bytes |
|---|---|---:|---:|
| list | invocation | 2048 | 4096 |
| ready | invocation | 1024 | 1024 |
| show | invocation | 2048 | 2048 |
| render-context | invocation | 16000 | 16000 |
| review | invocation | 4096 | 4096 |
| render-context.bundle | bundle | 16000 | 20000 |

## Dated trim plan

` + "```\n### 2026-10-01 render-context.bundle (measured M > target 16000)\n```\n"

	assert.False(t, hasDatedTrimPlan(weak, "list"),
		"table row, phrase, and dates must not count as a list trim plan")
	assert.False(t, hasDatedTrimPlan(weak, "render-context.bundle"),
		"fenced example heading must not count as a plan")
	assert.False(t, hasDatedTrimPlan(weak, "render-context"))

	planned := weak + "\n### 2026-10-01 list (measured 3000 > target 2048)\n"
	assert.True(t, hasDatedTrimPlan(planned, "list"))
	assert.False(t, hasDatedTrimPlan(planned, "listed"))
	assert.False(t, hasDatedTrimPlan(planned, "render-context.bundle"))

	bundled := planned + "\n### 2026-10-02 render-context.bundle\n"
	assert.True(t, hasDatedTrimPlan(bundled, "render-context.bundle"))
	assert.False(t, hasDatedTrimPlan(bundled, "render-context"),
		"render-context must not inherit a bundle heading")
}

func TestCheckedInBudgetsDocHasNoFencedFalsePositive(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	docs, err := os.ReadFile(filepath.Join(root, "docs", "context-budgets.md"))
	require.NoError(t, err)
	body := string(docs)
	for _, path := range []string{
		"list", "ready", "show", "render-context", "review", "render-context.bundle",
	} {
		assert.False(t, hasDatedTrimPlan(body, path),
			"checked-in example must not count as a trim plan for %q", path)
	}
}

func TestExplicitTargetsRejectsMeasuredSizeOnlySeed(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("list", 8, 8, 8)
	err := explicitTargets(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measured-size-only")
	assert.Contains(t, err.Error(), "list")
}

func TestBudgetGatePassesAtOrUnderBudget(t *testing.T) {
	t.Parallel()

	report, atBudget := runtimeBudgetFixture("list", 8, 8, 8)
	require.NoError(t, enforce(report, atBudget))

	_, above := runtimeBudgetFixture("list", 8, 9, 8)
	require.NoError(t, enforce(report, above))
}

func TestBudgetGateFailsMissingAndUnknownPaths(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("", 0, 0, 0)
	budgets.Artifacts = budgets.Artifacts[:len(budgets.Artifacts)-1]
	budgets.Artifacts = append(budgets.Artifacts, budget{
		Path:        "docs/not-a-runtime-artifact.md",
		Class:       classBundle,
		MaxBytes:    10,
		TargetBytes: 10,
	})

	err := enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing budget")
	assert.Contains(t, err.Error(), "unknown budget path")
}

func TestBudgetGateFailsClassMismatch(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("", 0, 0, 0)
	for i := range budgets.Artifacts {
		if budgets.Artifacts[i].Path == "list" {
			budgets.Artifacts[i].Class = classBundle
		}
	}
	err := enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "class mismatch")
	assert.Contains(t, err.Error(), "list")
}

func TestLoadBudgetsRejectsDuplicatesAndNegative(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dup := filepath.Join(dir, "dup.json")
	require.NoError(t, os.WriteFile(dup, []byte(`{
  "artifacts": [
    {"path": "list", "class": "invocation", "max_bytes": 1, "target_bytes": 1},
    {"path": "list", "class": "invocation", "max_bytes": 2, "target_bytes": 1}
  ]
}`), 0o644))
	_, err := loadBudgets(dup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	neg := filepath.Join(dir, "neg.json")
	require.NoError(t, os.WriteFile(neg, []byte(`{
  "artifacts": [
    {"path": "list", "class": "invocation", "max_bytes": -1, "target_bytes": 1}
  ]
}`), 0o644))
	_, err = loadBudgets(neg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_bytes")

	noTarget := filepath.Join(dir, "no-target.json")
	require.NoError(t, os.WriteFile(noTarget, []byte(`{
  "artifacts": [
    {"path": "list", "class": "invocation", "max_bytes": 8}
  ]
}`), 0o644))
	_, err = loadBudgets(noTarget)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_bytes")

	emptyPath := filepath.Join(dir, "empty-path.json")
	require.NoError(t, os.WriteFile(emptyPath, []byte(`{
  "artifacts": [{"path": "", "class": "invocation", "max_bytes": 1, "target_bytes": 1}]
}`), 0o644))
	_, err = loadBudgets(emptyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty path")

	emptyClass := filepath.Join(dir, "empty-class.json")
	require.NoError(t, os.WriteFile(emptyClass, []byte(`{
  "artifacts": [{"path": "list", "class": "", "max_bytes": 1, "target_bytes": 1}]
}`), 0o644))
	_, err = loadBudgets(emptyClass)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty class")

	_, err = loadBudgets(filepath.Join(dir, "missing.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read context budgets")

	_, err = parseBudgets([]byte(`{`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse context budgets")
}

func TestRaisedBudgetsListsIncreasesOnly(t *testing.T) {
	t.Parallel()

	prev := budgetFile{Artifacts: []budget{
		{Path: "list", Class: classInvocation, MaxBytes: 10, TargetBytes: 8},
		{Path: "ready", Class: classInvocation, MaxBytes: 20, TargetBytes: 20},
		{Path: "show", Class: classInvocation, MaxBytes: 30, TargetBytes: 30},
	}}
	next := budgetFile{Artifacts: []budget{
		{Path: "list", Class: classInvocation, MaxBytes: 9, TargetBytes: 8},
		{Path: "ready", Class: classInvocation, MaxBytes: 20, TargetBytes: 20},
		{Path: "show", Class: classInvocation, MaxBytes: 40, TargetBytes: 30},
		{Path: "review", Class: classInvocation, MaxBytes: 1, TargetBytes: 1},
	}}
	raised := raisedBudgets(prev, next)
	require.Equal(t, []string{"show"}, raised)
}

func TestOverBudgetClassesOrdersKnownThenExtra(t *testing.T) {
	t.Parallel()
	gate := &gateError{OverBudget: []violation{
		{Path: "z", Class: "payload", Bytes: 2, MaxBytes: 1},
		{Path: "a", Class: classBundle, Bytes: 2, MaxBytes: 1},
		{Path: "b", Class: classInvocation, Bytes: 2, MaxBytes: 1},
	}}
	assert.Equal(t, []string{classInvocation, classBundle, "payload"}, gate.overBudgetClasses())
	assert.Contains(t, gate.Error(), "payload")
	var none *gateError
	assert.Empty(t, none.Error())
	assert.Nil(t, none.overBudgetClasses())
}

func TestCheckedInContextBudgetsHold(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect()
	require.NoError(t, err)

	budgets, err := loadBudgets(filepath.Join(root, filepath.FromSlash(budgetsRelPath)))
	require.NoError(t, err)
	require.NoError(t, enforce(report, budgets), "runtime budgets must hold against fixture-measured sizes")

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(budgetsRelPath)))
	require.NoError(t, err)
	var decoded budgetFile
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, budgets.Artifacts, decoded.Artifacts)
}
