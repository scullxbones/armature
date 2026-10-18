package decompose

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ContextParams struct {
	Plan        *Plan
	Sources     []SourceContent
	Template    string
	ExistingDAG bool
}

type ContextOutput struct {
	PromptTemplate string          `json:"prompt_template"`
	Sources        []SourceContent `json:"sources"`
	ExistingDAG    []any           `json:"existing_dag,omitempty"`
	Constraints    map[string]any  `json:"constraints"`
	PlanSchema     map[string]any  `json:"plan_schema"`
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

	if len(params.Sources) > 0 {
		out.Sources = append(out.Sources, params.Sources...)
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
		sb.WriteString("--- Source: ")
		sb.WriteString(s.ID)
		sb.WriteString(" ---\n")
		sb.WriteString(s.Content)
		sb.WriteString("\n")
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
	data, err := json.MarshalIndent(defaultPlanSchema(), "", "  ")
	if err != nil {
		return "(error serializing plan schema)"
	}
	return string(data)
}

func constraintsBlock() string {
	data, err := json.MarshalIndent(defaultConstraints(), "", "  ")
	if err != nil {
		return "(error serializing constraints)"
	}
	return string(data)
}

func defaultTemplate() string {
	return "{{SOURCES}}\n\n{{EXISTING_DAG}}\n\n{{CONSTRAINTS}}\n\n{{PLAN_SCHEMA}}"
}

func defaultConstraints() map[string]any {
	return map[string]any{
		"node_id_format":   "EPIC|STORY|TSK-N",
		"max_tasks_per_pr": 1,
		"scope_required":   true,
	}
}

func defaultPlanSchema() map[string]any {
	return map[string]any{
		"version": 1,
		"fields": map[string]string{
			"id":         "string",
			"title":      "string",
			"type":       "epic|story|feature|task|bug",
			"parent":     "string (optional)",
			"scope":      "string",
			"acceptance": "array of string-or-object (optional)",
			"dod":        "string",
		},
	}
}
