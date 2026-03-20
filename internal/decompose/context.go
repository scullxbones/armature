package decompose

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scullxbones/trellis/internal/sources"
)

type ContextParams struct {
	IssuesDir   string
	Plan        *Plan
	SourceIDs   []string
	Template    string
	ExistingDAG bool
}

type ContextOutput struct {
	PromptTemplate string                 `json:"prompt_template"`
	Sources        []SourceContent        `json:"sources"`
	ExistingDAG    []interface{}          `json:"existing_dag,omitempty"`
	Constraints    map[string]interface{} `json:"constraints"`
	PlanSchema     map[string]interface{} `json:"plan_schema"`
}

type SourceContent struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

func BuildContext(params ContextParams) (ContextOutput, error) {
	out := ContextOutput{
		Constraints: defaultConstraints(),
		PlanSchema:  defaultPlanSchema(),
	}

	if params.IssuesDir != "" && len(params.SourceIDs) > 0 {
		sourcesDir := filepath.Join(params.IssuesDir, "sources")
		manifest, err := sources.ReadManifest(sourcesDir)
		if err == nil {
			for _, id := range params.SourceIDs {
				if _, ok := manifest.Get(id); !ok {
					continue
				}
				data, err := sources.ReadCache(sourcesDir, id)
				if err != nil || data == nil {
					continue
				}
				out.Sources = append(out.Sources, SourceContent{ID: id, Content: string(data)})
			}
		}
	}

	tmpl := params.Template
	if tmpl == "" {
		tmpl = defaultTemplate()
	}
	planTitle := ""
	if params.Plan != nil {
		planTitle = fmt.Sprintf("# Plan: %s\n", params.Plan.Title)
	}
	tmpl = strings.ReplaceAll(tmpl, "{{SOURCES}}", buildSourcesBlock(out.Sources))
	tmpl = strings.ReplaceAll(tmpl, "{{EXISTING_DAG}}", buildDAGBlock(params))
	tmpl = strings.ReplaceAll(tmpl, "{{PLAN_SCHEMA}}", planSchemaBlock())
	tmpl = strings.ReplaceAll(tmpl, "{{CONSTRAINTS}}", constraintsBlock())
	out.PromptTemplate = planTitle + tmpl

	return out, nil
}

func buildSourcesBlock(srcs []SourceContent) string {
	if len(srcs) == 0 {
		return "(no sources registered)"
	}
	var sb strings.Builder
	for _, s := range srcs {
		sb.WriteString("--- Source: " + s.ID + " ---\n")
		sb.WriteString(s.Content + "\n")
	}
	return sb.String()
}

func buildDAGBlock(params ContextParams) string {
	if !params.ExistingDAG || params.Plan == nil {
		return ""
	}
	data, err := json.MarshalIndent(params.Plan.Issues, "", "  ")
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

// PlanContext returns a summary of the plan suitable for use as context.
func PlanContext(plan *Plan) string {
	return fmt.Sprintf("Plan: %s (%d issues)", plan.Title, len(plan.Issues))
}

func defaultPlanSchema() map[string]interface{} {
	return map[string]interface{}{
		"version": 1,
		"fields": map[string]string{
			"id":                 "string",
			"title":              "string",
			"type":               "epic|story|task",
			"parent":             "string (optional)",
			"scope":              "[]string",
			"acceptance":         "[]object",
			"definition_of_done": "string",
			"source_citation":    "object (optional)",
		},
	}
}
