package contextreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureReportAndBudgets(t *testing.T, skillBody string, maxBytes int) (Report, BudgetFile) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "internal/skillsembed/skills/demo/SKILL.md", skillBody)
	writeFile(t, root, "CONTEXT.md", "glossary-body")
	writeFile(t, root, "docs/commands.md", "commands-body")
	writeFile(t, root, "docs/concepts.md", "concepts-body")
	writeFile(t, root, "docs/use-cases.md", "use-cases-body")

	report, err := Collect(root)
	require.NoError(t, err)

	budgets := BudgetFile{Artifacts: make([]Budget, 0, len(report.Artifacts))}
	for _, a := range report.Artifacts {
		max := a.Bytes
		if strings.HasSuffix(a.Path, "internal/skillsembed/skills/demo") {
			max = maxBytes
		}
		budgets.Artifacts = append(budgets.Artifacts, Budget{
			Path:     a.Path,
			Class:    a.Class,
			MaxBytes: max,
		})
	}
	return report, budgets
}

func TestBudgetGateFailsOverBudget_REQ_NXTTN_S3_T2(t *testing.T) {
	t.Parallel()

	report, budgets := fixtureReportAndBudgets(t, "abcdefgh", 4)
	err := Enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill")
	assert.Contains(t, err.Error(), "internal/skillsembed/skills/demo")
	assert.Contains(t, err.Error(), "8 bytes > budget 4")

	var gate *GateError
	require.ErrorAs(t, err, &gate)
	require.Len(t, gate.OverBudget, 1)
	assert.Equal(t, ClassSkill, gate.OverBudget[0].Class)
	assert.Equal(t, 8, gate.OverBudget[0].Bytes)
	assert.Equal(t, 4, gate.OverBudget[0].MaxBytes)
	assert.Equal(t, []string{ClassSkill}, gate.OverBudgetClasses())
}

func TestBudgetGatePassesAtOrUnderBudget(t *testing.T) {
	t.Parallel()

	report, atBudget := fixtureReportAndBudgets(t, "abcdefgh", 8)
	require.NoError(t, Enforce(report, atBudget))

	_, above := fixtureReportAndBudgets(t, "abcdefgh", 9)
	require.NoError(t, Enforce(report, above))
}

func TestBudgetGateFailsMissingAndUnknownPaths(t *testing.T) {
	t.Parallel()

	report, budgets := fixtureReportAndBudgets(t, "abcd", 4)
	budgets.Artifacts = budgets.Artifacts[:len(budgets.Artifacts)-1]
	budgets.Artifacts = append(budgets.Artifacts, Budget{
		Path:     "docs/not-a-measured-artifact.md",
		Class:    ClassConcepts,
		MaxBytes: 10,
	})

	err := Enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing budget")
	assert.Contains(t, err.Error(), "unknown budget path")
}

func TestBudgetGateFailsClassMismatch(t *testing.T) {
	t.Parallel()

	report, budgets := fixtureReportAndBudgets(t, "abcd", 4)
	for i := range budgets.Artifacts {
		if budgets.Artifacts[i].Class == ClassGlossary {
			budgets.Artifacts[i].Class = ClassCommands
		}
	}
	err := Enforce(report, budgets)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "class mismatch")
	assert.Contains(t, err.Error(), "CONTEXT.md")
}

func TestLoadBudgetsRejectsDuplicatesAndNegative(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dup := filepath.Join(dir, "dup.json")
	require.NoError(t, os.WriteFile(dup, []byte(`{
  "artifacts": [
    {"path": "CONTEXT.md", "class": "glossary", "max_bytes": 1},
    {"path": "CONTEXT.md", "class": "glossary", "max_bytes": 2}
  ]
}`), 0o644))
	_, err := LoadBudgets(dup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")

	neg := filepath.Join(dir, "neg.json")
	require.NoError(t, os.WriteFile(neg, []byte(`{
  "artifacts": [
    {"path": "CONTEXT.md", "class": "glossary", "max_bytes": -1}
  ]
}`), 0o644))
	_, err = LoadBudgets(neg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_bytes")

	emptyPath := filepath.Join(dir, "empty-path.json")
	require.NoError(t, os.WriteFile(emptyPath, []byte(`{
  "artifacts": [{"path": "", "class": "glossary", "max_bytes": 1}]
}`), 0o644))
	_, err = LoadBudgets(emptyPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty path")

	emptyClass := filepath.Join(dir, "empty-class.json")
	require.NoError(t, os.WriteFile(emptyClass, []byte(`{
  "artifacts": [{"path": "CONTEXT.md", "class": "", "max_bytes": 1}]
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
		{Path: "a", Class: ClassSkill, MaxBytes: 10},
		{Path: "b", Class: ClassGlossary, MaxBytes: 20},
		{Path: "c", Class: ClassCommands, MaxBytes: 30},
	}}
	next := BudgetFile{Artifacts: []Budget{
		{Path: "a", Class: ClassSkill, MaxBytes: 9},
		{Path: "b", Class: ClassGlossary, MaxBytes: 20},
		{Path: "c", Class: ClassCommands, MaxBytes: 40},
		{Path: "d", Class: ClassConcepts, MaxBytes: 1},
	}}
	raised := RaisedBudgets(prev, next)
	require.Equal(t, []string{"c"}, raised)
}

func TestOverBudgetClassesOrdersKnownThenExtra(t *testing.T) {
	t.Parallel()
	gate := &GateError{OverBudget: []Violation{
		{Path: "z", Class: "payload", Bytes: 2, MaxBytes: 1},
		{Path: "a", Class: ClassUseCases, Bytes: 2, MaxBytes: 1},
		{Path: "b", Class: ClassSkill, Bytes: 2, MaxBytes: 1},
	}}
	assert.Equal(t, []string{ClassSkill, ClassUseCases, "payload"}, gate.OverBudgetClasses())
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
	require.NoError(t, Enforce(report, budgets), "seeded budgets must be at or above measured size")

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(BudgetsRelPath))) //nolint:gosec // test reads the checked-in budgets file
	require.NoError(t, err)
	var decoded BudgetFile
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, budgets.Artifacts, decoded.Artifacts)
}
