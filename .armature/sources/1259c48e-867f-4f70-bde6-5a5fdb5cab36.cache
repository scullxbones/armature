# E4 Decompose-Context Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the decompose-context stub with a full implementation: template interpolation, source fetch from cache, and full CLI flags.

**Spec:** `docs/superpowers/specs/2026-03-14-trellis-epic-decomposition-design.md` (E3-S5 section)

**Depends on:** `2026-03-18-e4-s3-full-validate.md`

**Execution order within E4:** S1 → S4 → S7 → S2 → S3 → S5 → S6 → S8 → S9

**Tech Stack:** Go 1.26, Cobra v1.8, testify

---

## File Structure

| File | Change |
|---|---|
| `internal/decompose/context.go` | Full decompose-context: template interpolation + source fetch (replaces stub) |
| `internal/decompose/context_stub.go` | **Delete** |
| `internal/decompose/decompose_test.go` | Extend with context tests |
| `cmd/trellis/decompose.go` | Full `decompose-context` flags: `--sources`, `--template`, `--format`, `--output`, `--existing-dag` |

---

## Tasks

### Task 1: decompose-context full implementation

**Files:**
- Create: `internal/decompose/context.go` (replaces `context_stub.go`)
- Delete: `internal/decompose/context_stub.go`
- Modify: `internal/decompose/decompose_test.go`
- Modify: `cmd/trellis/decompose.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/decompose/decompose_test.go (append)
func TestDecomposeContextBasic(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0755))

	// Register a local filesystem source in cache
	sourceContent := "# PRD\n\nThis is the product requirements document."
	require.NoError(t, sources.WriteCache(issuesDir, "prd", sourceContent))
	manifest := sources.Manifest{Sources: []sources.SourceEntry{
		{ID: "prd", Provider: "filesystem", Path: filepath.Join(dir, "prd.md"), SHA: "abc"},
	}}
	require.NoError(t, sources.WriteManifest(issuesDir, manifest))

	plan := &Plan{Title: "My Plan", Issues: []*Issue{{ID: "TSK-1", Title: "Task one"}}}
	template := "Sources: {{SOURCES}}\nDAG: {{EXISTING_DAG}}\nSchema: {{PLAN_SCHEMA}}"

	ctx, err := decompose.BuildContext(decompose.ContextParams{
		IssuesDir:   issuesDir,
		Plan:        plan,
		SourceIDs:   []string{"prd"},
		Template:    template,
		ExistingDAG: true,
	})
	require.NoError(t, err)
	assert.Contains(t, ctx.PromptTemplate, "PRD")
	assert.NotEmpty(t, ctx.Sources)
	assert.Equal(t, "prd", ctx.Sources[0].ID)
}

func TestDecomposeContextNoSources(t *testing.T) {
	plan := &Plan{Title: "Plan", Issues: []*Issue{}}
	ctx, err := decompose.BuildContext(decompose.ContextParams{Plan: plan})
	require.NoError(t, err)
	assert.Empty(t, ctx.Sources)
	assert.NotEmpty(t, ctx.PlanSchema)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/decompose/... -run TestDecomposeContext -v
```

Expected: FAIL.

- [ ] **Step 3: Implement context.go**

```go
// internal/decompose/context.go
package decompose

import (
	"encoding/json"
	"strings"

	"github.com/scullxbones/armature/internal/sources"
)

// ContextParams holds inputs for building decompose-context output.
type ContextParams struct {
	IssuesDir   string
	Plan        *Plan
	SourceIDs   []string // source IDs to include; empty = all registered sources
	Template    string   // prompt template with {{SOURCES}} etc.
	ExistingDAG bool     // whether to include the existing DAG from materialized state
}

// ContextOutput is the JSON output schema for decompose-context.
// Schema is stable — E3-S5 does not change field names, only fills them.
type ContextOutput struct {
	PromptTemplate string          `json:"prompt_template"`
	Sources        []SourceContent `json:"sources"`
	ExistingDAG    []interface{}   `json:"existing_dag,omitempty"`
	Constraints    map[string]interface{} `json:"constraints"`
	PlanSchema     map[string]interface{} `json:"plan_schema"`
}

// SourceContent holds a source ID and its cached content.
type SourceContent struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// BuildContext assembles the decompose-context output from the given params.
func BuildContext(params ContextParams) (ContextOutput, error) {
	out := ContextOutput{
		Constraints: defaultConstraints(),
		PlanSchema:  defaultPlanSchema(),
	}

	// Load sources
	if params.IssuesDir != "" && len(params.SourceIDs) > 0 {
		manifest, err := sources.ReadManifest(params.IssuesDir)
		if err == nil {
			for _, id := range params.SourceIDs {
				if _, ok := manifest.Get(id); !ok {
					continue
				}
				content, err := sources.ReadCache(params.IssuesDir, id)
				if err != nil {
					continue
				}
				out.Sources = append(out.Sources, SourceContent{ID: id, Content: content})
			}
		}
	}

	// Interpolate template
	tmpl := params.Template
	if tmpl == "" {
		tmpl = defaultTemplate()
	}
	sourcesBlock := buildSourcesBlock(out.Sources)
	dagBlock := ""
	if params.ExistingDAG && params.Plan != nil {
		dagBlock = buildDAGBlock(params.Plan)
	}
	tmpl = strings.ReplaceAll(tmpl, "{{SOURCES}}", sourcesBlock)
	tmpl = strings.ReplaceAll(tmpl, "{{EXISTING_DAG}}", dagBlock)
	tmpl = strings.ReplaceAll(tmpl, "{{PLAN_SCHEMA}}", planSchemaBlock())
	tmpl = strings.ReplaceAll(tmpl, "{{CONSTRAINTS}}", constraintsBlock())
	out.PromptTemplate = tmpl

	return out, nil
}

func buildSourcesBlock(srcs []SourceContent) string {
	if len(srcs) == 0 {
		return "(no sources registered)"
	}
	var sb strings.Builder
	for _, s := range srcs {
		sb.WriteString("--- Source: " + s.ID + " ---\n")
		sb.WriteString(s.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

func buildDAGBlock(plan *Plan) string {
	data, err := json.MarshalIndent(plan.Issues, "", "  ")
	if err != nil {
		return "(error serializing DAG)"
	}
	return string(data)
}

func planSchemaBlock() string {
	data, _ := json.MarshalIndent(defaultPlanSchema(), "", "  ")
	return string(data)
}

func constraintsBlock() string {
	data, _ := json.MarshalIndent(defaultConstraints(), "", "  ")
	return string(data)
}

func defaultTemplate() string {
	return "{{SOURCES}}\n\n{{EXISTING_DAG}}\n\n{{CONSTRAINTS}}\n\n{{PLAN_SCHEMA}}"
}

func defaultConstraints() map[string]interface{} {
	return map[string]interface{}{
		"node_id_format":   "EPIC|STORY|TSK-N",
		"max_tasks_per_pr": 1,
		"scope_required":   true,
	}
}

func defaultPlanSchema() map[string]interface{} {
	return map[string]interface{}{
		"version": 1,
		"fields": map[string]string{
			"id":                "string",
			"title":             "string",
			"type":              "epic|story|task",
			"parent":            "string (optional)",
			"scope":             "[]string",
			"acceptance":        "[]object",
			"definition_of_done": "string",
			"source_citation":   "object (optional)",
		},
	}
}
```

- [ ] **Step 4: Delete context_stub.go**

```bash
rm internal/decompose/context_stub.go
```

- [ ] **Step 5: Update decompose.go CLI to use full flags**

In `cmd/trellis/decompose.go`, replace `newDecomposeContextCmd()` with:

```go
func newDecomposeContextCmd() *cobra.Command {
	var (
		planPath    string
		sourceIDs   []string
		templatePath string
		outputPath  string
		formatFlag  string
		existingDAG bool
	)

	cmd := &cobra.Command{
		Use:               "decompose-context",
		Short:             "Build LLM prompt context for plan decomposition",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := decompose.ParsePlan(planPath)
			if err != nil {
				return err
			}

			tmpl := ""
			if templatePath != "" {
				data, err := os.ReadFile(templatePath)
				if err != nil {
					return fmt.Errorf("read template: %w", err)
				}
				tmpl = string(data)
			}

			ctxOut, err := decompose.BuildContext(decompose.ContextParams{
				IssuesDir:   appCtx.IssuesDir,
				Plan:        plan,
				SourceIDs:   sourceIDs,
				Template:    tmpl,
				ExistingDAG: existingDAG,
			})
			if err != nil {
				return fmt.Errorf("build context: %w", err)
			}

			var data []byte
			if formatFlag == "json" || formatFlag == "" {
				data, err = json.MarshalIndent(ctxOut, "", "  ")
				if err != nil {
					return err
				}
			} else {
				data = []byte(ctxOut.PromptTemplate)
			}

			if outputPath != "" {
				return os.WriteFile(outputPath, data, 0644)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&planPath, "plan", "", "Path to plan JSON file")
	cmd.Flags().StringArrayVar(&sourceIDs, "sources", nil, "Source IDs to include")
	cmd.Flags().StringVar(&templatePath, "template", "", "Path to prompt template file")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output file path (default: stdout)")
	cmd.Flags().StringVar(&formatFlag, "format", "json", "Output format: json|text")
	cmd.Flags().BoolVar(&existingDAG, "existing-dag", false, "Include existing DAG in context")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}
```

Add `"encoding/json"` and `"os"` to the decompose.go imports.

- [ ] **Step 6: Run all decompose tests**

```bash
go test ./internal/decompose/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/decompose/context.go cmd/trellis/decompose.go
git rm internal/decompose/context_stub.go
git commit -m "feat(decompose): full decompose-context implementation (replaces stub)"
```
