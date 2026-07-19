// Package decompose turns a story or epic plan into concrete task issues, and supports applying, reverting, and re-planning a decomposition.
package decompose

import (
	"encoding/json"
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/strictjson"
)

// PlanIssue represents a single issue in a plan file.
type PlanIssue struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Type         string          `json:"type"`
	Scope        string          `json:"scope"`
	ContextFiles []string        `json:"context_files,omitempty"`
	Priority     string          `json:"priority"`
	DoD          string          `json:"dod"`
	Parent       string          `json:"parent"`
	BlockedBy    []string        `json:"blocked_by"`
	Notes        []string        `json:"notes"`
	Acceptance   json.RawMessage `json:"acceptance,omitempty"`
}

// Plan represents a parsed plan file.
type Plan struct {
	Version int         `json:"version"`
	Title   string      `json:"title"`
	Issues  []PlanIssue `json:"issues"`
}

// ParsePlan parses a plan JSON file from the given path.
// It uses DisallowUnknownFields to catch malformed or deprecated input.
func ParsePlan(path string) (*Plan, error) {
	data, err := adapters.ReadPlanFile(path)
	if err != nil {
		return nil, err
	}

	var plan Plan
	if err := strictjson.Decode(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan file %s: %w", path, err)
	}

	if plan.Version != 1 {
		return nil, fmt.Errorf("unsupported plan version: %d", plan.Version)
	}

	return &plan, nil
}
