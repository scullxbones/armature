package contextreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func artifactBySuffix(t *testing.T, report Report, suffix string) Artifact {
	t.Helper()
	for _, a := range report.Artifacts {
		if strings.HasSuffix(a.Path, suffix) || a.Path == suffix {
			return a
		}
	}
	t.Fatalf("artifact ending in %q not found in %+v", suffix, report.Artifacts)
	return Artifact{}
}

func TestContextReportPricesSkills_REQ_NXTTN_S3_T1(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	report, err := Collect(root)
	require.NoError(t, err)

	wantSkills := []string{
		"armature",
		"armature-activity-indexer",
		"armature-auditor",
		"armature-coordinator",
		"armature-planner",
		"armature-reviewer",
		"armature-worker",
		"test-skill",
	}
	gotSkills := map[string]Artifact{}
	for _, a := range report.Artifacts {
		if a.Class != ClassSkill {
			continue
		}
		gotSkills[filepath.Base(a.Path)] = a
	}
	for _, name := range wantSkills {
		art, ok := gotSkills[name]
		require.True(t, ok, "embedded skill %q must be priced", name)
		assert.Greater(t, art.Bytes, 0, "skill %q should have content", name)
		assert.Equal(t, art.Bytes/BytesPerToken, art.EstimatedTokens, "bytes/4 for skill %q", name)
	}
	assert.Len(t, gotSkills, len(wantSkills), "exactly the embedded skills, no extra skill rows")

	coordMD, err := os.ReadFile(filepath.Join(root, "internal", "skillsembed", "skills", "armature-coordinator", "SKILL.md"))
	require.NoError(t, err)
	coord := gotSkills["armature-coordinator"]
	assert.Greater(t, coord.Bytes, len(coordMD), "skill weight includes nested reference files, not only SKILL.md")

	for _, spec := range []struct {
		class, suffix string
	}{
		{ClassGlossary, "CONTEXT.md"},
		{ClassCommands, "docs/commands.md"},
		{ClassConcepts, "docs/concepts.md"},
		{ClassUseCases, "docs/use-cases.md"},
	} {
		art := artifactBySuffix(t, report, spec.suffix)
		assert.Equal(t, spec.class, art.Class)
		assert.Greater(t, art.Bytes, 0)
		assert.Equal(t, art.Bytes/BytesPerToken, art.EstimatedTokens)
	}

	assert.Equal(t, EstimationMethod, report.EstimationMethod)
	assert.Contains(t, report.EstimationMethod, "bytes/4")
	assert.Contains(t, report.EstimationMethod, "token_budget")
	assert.Contains(t, report.EstimationMethod, "AOC-S3-T2")

	var summedBytes, summedTokens int
	for _, a := range report.Artifacts {
		summedBytes += a.Bytes
		summedTokens += a.EstimatedTokens
		assert.NotEqual(t, "payload", a.Class)
		assert.NotContains(t, a.Path, "render-context")
	}
	assert.Equal(t, summedBytes, report.TotalBytes)
	assert.Equal(t, summedTokens, report.TotalEstimatedTokens)

	human := RenderHuman(report)
	assert.Contains(t, human, "bytes/4")
	assert.Contains(t, human, "token_budget")
	assert.Contains(t, human, "AOC-S3-T2")
	assert.Contains(t, human, "armature-coordinator")
	assert.Contains(t, human, "CONTEXT.md")

	raw, err := RenderJSON(report)
	require.NoError(t, err)
	var decoded Report
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, report.EstimationMethod, decoded.EstimationMethod)
	assert.Contains(t, string(raw), "bytes/4")
	assert.NotContains(t, strings.ToLower(string(raw)), `"class": "payload"`)
}

func TestCollectSumsNestedSkillFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "internal/skillsembed/skills/demo/SKILL.md", "abcd")
	writeFile(t, root, "internal/skillsembed/skills/demo/references/extra.md", "efghijkl")
	writeFile(t, root, "CONTEXT.md", "glossary-body")
	writeFile(t, root, "docs/commands.md", "commands-body")
	writeFile(t, root, "docs/concepts.md", "concepts-body")
	writeFile(t, root, "docs/use-cases.md", "use-cases-body")

	report, err := Collect(root)
	require.NoError(t, err)
	skill := artifactBySuffix(t, report, "internal/skillsembed/skills/demo")
	assert.Equal(t, 12, skill.Bytes)
	assert.Equal(t, 3, skill.EstimatedTokens)
}

func TestCollectErrorsWhenRequiredDocsMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "internal/skillsembed/skills/demo/SKILL.md", "abcd")

	_, err := Collect(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONTEXT.md")
}

func TestCollectErrorsWhenSkillsDirMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "CONTEXT.md", "x")
	writeFile(t, root, "docs/commands.md", "x")
	writeFile(t, root, "docs/concepts.md", "x")
	writeFile(t, root, "docs/use-cases.md", "x")

	_, err := Collect(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills")
}

func TestEstimateTokensIntegerDivision(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, EstimateTokens(3))
	assert.Equal(t, 1, EstimateTokens(4))
	assert.Equal(t, 1, EstimateTokens(5))
}
