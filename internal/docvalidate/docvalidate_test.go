package docvalidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func schemaRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "docs/schemas/plan.schema.json", `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	writeFile(t, root, "docs/schemas/review-bundle.schema.json", `{"type":"object"}`)
	writeFile(t, root, "docs/schemas/conformance-assessment.schema.json", `{"type":"object"}`)
	writeFile(t, root, "docs/schemas/activity-index.schema.json", `{"type":"object"}`)
	return root
}

func TestValidateScansOnlyCanonicalMarkdown(t *testing.T) {
	t.Parallel()
	root := schemaRepo(t)
	valid := "```json artifact_type=plan\n{\"name\": \"ok\"}\n```\n"
	writeFile(t, root, "docs/example.md", valid)
	writeFile(t, root, "internal/skillsembed/skills/example/SKILL.md", valid)
	writeFile(t, root, ".claude/skills/example/SKILL.md", "```json artifact_type=unknown\n{}\n```\n")

	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsHyphenatedArtifactTypes(t *testing.T) {
	t.Parallel()
	root := schemaRepo(t)
	writeFile(t, root, "docs/example.md", "```json artifact_type=review-bundle\n{}\n```\n")

	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateReportsMalformedJSONWithPathAndLine(t *testing.T) {
	t.Parallel()
	root := schemaRepo(t)
	writeFile(t, root, "docs/example.md", "before\n```json artifact_type=plan\n{bad}\n```\n")

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "docs/example.md:2") || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("Validate() error = %v, want path, line, and invalid JSON", err)
	}
}

func TestValidateReportsUnknownArtifactType(t *testing.T) {
	t.Parallel()
	root := schemaRepo(t)
	writeFile(t, root, "docs/example.md", "```json artifact_type=unknown\n{}\n```\n")

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "Unknown artifact type: unknown") {
		t.Fatalf("Validate() error = %v, want unknown artifact type", err)
	}
}

func TestValidateReportsSchemaViolation(t *testing.T) {
	t.Parallel()
	root := schemaRepo(t)
	writeFile(t, root, "docs/example.md", "```json artifact_type=plan\n{}\n```\n")

	err := Validate(root)
	if err == nil || !strings.Contains(err.Error(), "missing properties: 'name'") {
		t.Fatalf("Validate() error = %v, want schema violation", err)
	}
}

func TestValidateRepositoryExamples(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(root); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
