package contextreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runtimeBudgetFixture(overPath string, measured, max, target int) (Report, BudgetFile) {
	mk := func(path, class string, bytes, maxBytes, targetBytes int) (Artifact, Budget) {
		art := Artifact{
			Path:            path,
			Class:           class,
			Bytes:           bytes,
			EstimatedTokens: EstimateTokens(bytes),
		}
		row := Budget{
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
		{"list", ClassInvocation, 8, 16, 16},
		{"ready", ClassInvocation, 8, 16, 16},
		{"show", ClassInvocation, 8, 16, 16},
		{"render-context", ClassInvocation, 8, 16, 16},
		{"review", ClassInvocation, 8, 16, 16},
		{"render-context.bundle", ClassBundle, 8, 16, 16},
	}

	var report Report
	var budgets BudgetFile
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
	err := Enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invocation")
	assert.Contains(t, err.Error(), "list")
	assert.Contains(t, err.Error(), "8 bytes > budget 4")

	var gate *GateError
	require.ErrorAs(t, err, &gate)
	require.Len(t, gate.OverBudget, 1)
	assert.Equal(t, ClassInvocation, gate.OverBudget[0].Class)
	assert.Equal(t, "list", gate.OverBudget[0].Path)
	assert.Equal(t, 8, gate.OverBudget[0].Bytes)
	assert.Equal(t, 4, gate.OverBudget[0].MaxBytes)
	assert.Equal(t, []string{ClassInvocation}, gate.OverBudgetClasses())
}

func TestRuntimeBudgetCapsAreExplicitTargets_REQ_NXTTN_S3_T2(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect(root)
	require.NoError(t, err)

	budgets, err := LoadBudgets(filepath.Join(root, filepath.FromSlash(BudgetsRelPath)))
	require.NoError(t, err)

	require.NoError(t, ExplicitTargets(report, budgets),
		"checked-in runtime budgets must name a promise, not seed silently at measured size")

	docs, err := os.ReadFile(filepath.Join(root, "docs", "context-budgets.md"))
	require.NoError(t, err)
	body := string(docs)

	wantPaths := []string{"list", "ready", "show", "render-context", "review", "render-context.bundle"}
	got := map[string]Budget{}
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
			assert.Contains(t, body, "trim plan")
			assert.Regexp(t, `\d{4}-\d{2}-\d{2}`, body,
				"dated trim plan required when measured bytes exceed target for %q", path)
			assert.Contains(t, body, path)
		}
	}
	assert.NotContains(t, body, "internal/skillsembed/skills")
}

func TestExplicitTargetsRejectsMeasuredSizeOnlySeed(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("list", 8, 8, 8)
	err := ExplicitTargets(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measured-size-only")
	assert.Contains(t, err.Error(), "list")
}

func TestBudgetGatePassesAtOrUnderBudget(t *testing.T) {
	t.Parallel()

	report, atBudget := runtimeBudgetFixture("list", 8, 8, 8)
	require.NoError(t, Enforce(report, atBudget))

	_, above := runtimeBudgetFixture("list", 8, 9, 8)
	require.NoError(t, Enforce(report, above))
}

func TestBudgetGateFailsMissingAndUnknownPaths(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("", 0, 0, 0)
	budgets.Artifacts = budgets.Artifacts[:len(budgets.Artifacts)-1]
	budgets.Artifacts = append(budgets.Artifacts, Budget{
		Path:        "docs/not-a-runtime-artifact.md",
		Class:       ClassBundle,
		MaxBytes:    10,
		TargetBytes: 10,
	})

	err := Enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing budget")
	assert.Contains(t, err.Error(), "unknown budget path")
}

func TestBudgetGateFailsClassMismatch(t *testing.T) {
	t.Parallel()

	report, budgets := runtimeBudgetFixture("", 0, 0, 0)
	for i := range budgets.Artifacts {
		if budgets.Artifacts[i].Path == "list" {
			budgets.Artifacts[i].Class = ClassBundle
		}
	}
	err := Enforce(report, budgets)
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
	_, err := LoadBudgets(dup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	neg := filepath.Join(dir, "neg.json")
	require.NoError(t, os.WriteFile(neg, []byte(`{
  "artifacts": [
    {"path": "list", "class": "invocation", "max_bytes": -1, "target_bytes": 1}
  ]
}`), 0o644))
	_, err = LoadBudgets(neg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_bytes")

	noTarget := filepath.Join(dir, "no-target.json")
	require.NoError(t, os.WriteFile(noTarget, []byte(`{
  "artifacts": [
    {"path": "list", "class": "invocation", "max_bytes": 8}
  ]
}`), 0o644))
	_, err = LoadBudgets(noTarget)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_bytes")

	emptyPath := filepath.Join(dir, "empty-path.json")
	require.NoError(t, os.WriteFile(emptyPath, []byte(`{
  "artifacts": [{"path": "", "class": "invocation", "max_bytes": 1, "target_bytes": 1}]
}`), 0o644))
	_, err = LoadBudgets(emptyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty path")

	emptyClass := filepath.Join(dir, "empty-class.json")
	require.NoError(t, os.WriteFile(emptyClass, []byte(`{
  "artifacts": [{"path": "list", "class": "", "max_bytes": 1, "target_bytes": 1}]
}`), 0o644))
	_, err = LoadBudgets(emptyClass)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty class")

	_, err = LoadBudgets(filepath.Join(dir, "missing.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read context budgets")

	_, err = ParseBudgets([]byte(`{`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse context budgets")
}

func TestRaisedBudgetsListsIncreasesOnly(t *testing.T) {
	t.Parallel()

	prev := BudgetFile{Artifacts: []Budget{
		{Path: "list", Class: ClassInvocation, MaxBytes: 10, TargetBytes: 8},
		{Path: "ready", Class: ClassInvocation, MaxBytes: 20, TargetBytes: 20},
		{Path: "show", Class: ClassInvocation, MaxBytes: 30, TargetBytes: 30},
	}}
	next := BudgetFile{Artifacts: []Budget{
		{Path: "list", Class: ClassInvocation, MaxBytes: 9, TargetBytes: 8},
		{Path: "ready", Class: ClassInvocation, MaxBytes: 20, TargetBytes: 20},
		{Path: "show", Class: ClassInvocation, MaxBytes: 40, TargetBytes: 30},
		{Path: "review", Class: ClassInvocation, MaxBytes: 1, TargetBytes: 1},
	}}
	raised := RaisedBudgets(prev, next)
	require.Equal(t, []string{"show"}, raised)
}

func TestOverBudgetClassesOrdersKnownThenExtra(t *testing.T) {
	t.Parallel()
	gate := &GateError{OverBudget: []Violation{
		{Path: "z", Class: "payload", Bytes: 2, MaxBytes: 1},
		{Path: "a", Class: ClassBundle, Bytes: 2, MaxBytes: 1},
		{Path: "b", Class: ClassInvocation, Bytes: 2, MaxBytes: 1},
	}}
	assert.Equal(t, []string{ClassInvocation, ClassBundle, "payload"}, gate.OverBudgetClasses())
	assert.Contains(t, gate.Error(), "payload")
	var none *GateError
	assert.Empty(t, none.Error())
	assert.Nil(t, none.OverBudgetClasses())
}

func TestCheckedInContextBudgetsHold(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect(root)
	require.NoError(t, err)

	budgets, err := LoadBudgets(filepath.Join(root, filepath.FromSlash(BudgetsRelPath)))
	require.NoError(t, err)
	require.NoError(t, Enforce(report, budgets), "runtime budgets must hold against fixture-measured sizes")

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(BudgetsRelPath)))
	require.NoError(t, err)
	var decoded BudgetFile
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, budgets.Artifacts, decoded.Artifacts)
}
