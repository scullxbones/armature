package decompose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlan_Valid(t *testing.T) {
	plan := Plan{
		Version: 1,
		Title:   "Test Plan",
		Issues: []PlanIssue{
			{ID: "PLAN-001", Title: "First issue", Type: "task"},
			{ID: "PLAN-002", Title: "Second issue", Type: "task"},
		},
	}
	data, err := json.Marshal(plan)
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	parsed, err := ParsePlan(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Len(t, parsed.Issues, 2)
	assert.Equal(t, "PLAN-001", parsed.Issues[0].ID)
	assert.Equal(t, "PLAN-002", parsed.Issues[1].ID)
}

func TestParsePlan_InvalidVersion(t *testing.T) {
	plan := Plan{
		Version: 2,
		Title:   "Bad Plan",
		Issues:  []PlanIssue{},
	}
	data, err := json.Marshal(plan)
	require.NoError(t, err)

	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err = ParsePlan(tmpFile)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unsupported plan version"), "expected error to contain 'unsupported plan version', got: %s", err.Error())
}

func TestParsePlan_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := ParsePlan(filepath.Join(tmpDir, "nonexistent.json"))
	require.Error(t, err)
}

func TestParsePlan_InvalidJSON(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte("{invalid json"), 0644))

	_, err := ParsePlan(tmpFile)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "parse plan file"), "expected error to contain 'parse plan file', got: %s", err.Error())
}
