package decompose

import (
	"encoding/json"
	"fmt"
	"os"
)

// PlanIssue represents a single issue in a plan file.
type PlanIssue struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Scope     string   `json:"scope"`
	Priority  string   `json:"priority"`
	DoD       string   `json:"dod"`
	Parent    string   `json:"parent"`
	BlockedBy []string `json:"blocked_by"`
	Notes     []string `json:"notes"`
}

// Plan represents a parsed plan file.
type Plan struct {
	Version int         `json:"version"`
	Title   string      `json:"title"`
	Issues  []PlanIssue `json:"issues"`
}

// ParsePlan parses a plan JSON file from the given path.
func ParsePlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan file %s: %w", path, err)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan file %s: %w", path, err)
	}

	if plan.Version != 1 {
		return nil, fmt.Errorf("unsupported plan version: %d", plan.Version)
	}

	return &plan, nil
}
