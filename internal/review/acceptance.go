package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParseAcceptanceCriteria normalizes acceptance criteria to a list of plain strings.
// Canonical type-based acceptance forms are documented in docs/design/architecture.md,
// e.g. {"type":"test_passes","pattern":"*.go"} -> "test_passes: *.go".
func ParseAcceptanceCriteria(input json.RawMessage) ([]string, error) {
	if len(input) == 0 {
		return []string{}, nil
	}

	var plainStrings []string
	if err := json.Unmarshal(input, &plainStrings); err == nil {
		return plainStrings, nil
	}

	var objects []map[string]interface{}
	if err := json.Unmarshal(input, &objects); err != nil {
		return nil, fmt.Errorf("acceptance criteria must be a JSON array: %w", err)
	}

	var criteria []string
	for i, obj := range objects {
		var text string

		if desc, ok := obj["description"]; ok {
			if descStr, ok := desc.(string); ok && descStr != "" {
				text = descStr
			}
		}

		if text == "" {
			if t, ok := obj["text"]; ok {
				if textStr, ok := t.(string); ok && textStr != "" {
					text = textStr
				}
			}
		}

		if text == "" {
			text = renderTypedCriterion(obj, i)
		}

		criteria = append(criteria, text)
	}

	return criteria, nil
}

// renderTypedCriterion renders the canonical type-based form (see docs/design/architecture.md),
// e.g. {"type":"test_passes","pattern":"tests/auth/callback.test.ts"}.
func renderTypedCriterion(obj map[string]interface{}, index int) string {
	typeStr := ""
	if t, ok := obj["type"].(string); ok {
		typeStr = t
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		if k == "type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, fmt.Sprintf("%v", obj[k]))
	}

	switch {
	case typeStr != "" && len(values) > 0:
		return typeStr + ": " + strings.Join(values, ", ")
	case typeStr != "":
		return typeStr
	default:
		if encoded, err := json.Marshal(obj); err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("acceptance criterion at index %d", index)
	}
}
