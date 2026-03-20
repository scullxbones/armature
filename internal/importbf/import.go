package importbf

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
)

type ImportedIssue struct {
	ID                   string
	Title                string
	Type                 string
	Parent               string
	Scope                []string
	Provenance           Provenance
	RequiresConfirmation bool
}

type Provenance struct {
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
}

var defaultProvenance = Provenance{Method: "imported", Confidence: "inferred"}

func ParseCSV(data []byte) ([]ImportedIssue, error) {
	r := csv.NewReader(bytes.NewReader(data))
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, nil
	}
	header := rows[0]
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, col string) string {
		if i, ok := idx[col]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var items []ImportedIssue
	for _, row := range rows[1:] {
		scopeStr := get(row, "scope")
		var scope []string
		if scopeStr != "" {
			for _, s := range strings.Split(scopeStr, ",") {
				if t := strings.TrimSpace(s); t != "" {
					scope = append(scope, t)
				}
			}
		}
		items = append(items, ImportedIssue{
			ID: get(row, "id"), Title: get(row, "title"),
			Type: get(row, "type"), Parent: get(row, "parent"),
			Scope: scope, Provenance: defaultProvenance, RequiresConfirmation: true,
		})
	}
	return items, nil
}

func ParseJSON(data []byte) ([]ImportedIssue, error) {
	var raw []struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Type   string   `json:"type"`
		Parent string   `json:"parent"`
		Scope  []string `json:"scope"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	items := make([]ImportedIssue, len(raw))
	for i, r := range raw {
		items[i] = ImportedIssue{
			ID: r.ID, Title: r.Title, Type: r.Type, Parent: r.Parent, Scope: r.Scope,
			Provenance: defaultProvenance, RequiresConfirmation: true,
		}
	}
	return items, nil
}
