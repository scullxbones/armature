package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParseAcceptanceCriteria normalizes acceptance criteria from either plain strings
// or structured objects to a list of plain strings. It supports two formats:
//
// 1. Plain strings: ["test 1", "test 2"]
// 2. Structured objects: [{"description":"test 1"}, {"text":"test 2"}]
//
// For structured objects, it extracts the "description" field first, then falls back
// to "text" field if "description" is not present. If neither is present, it renders
// the canonical type-based acceptance forms documented in docs/design/architecture.md,
// e.g. {"type":"test_passes","pattern":"*.go"} -> "test_passes: *.go".
func ParseAcceptanceCriteria(input json.RawMessage) ([]string, error) {
	// Handle nil or empty input
	if len(input) == 0 {
		return []string{}, nil
	}

	// Try to unmarshal as plain strings first
	var plainStrings []string
	if err := json.Unmarshal(input, &plainStrings); err == nil {
		return plainStrings, nil
	}

	// Try to unmarshal as structured objects
	var objects []map[string]interface{}
	if err := json.Unmarshal(input, &objects); err != nil {
		return nil, fmt.Errorf("acceptance criteria must be a JSON array: %w", err)
	}

	// Extract text from each object
	var criteria []string
	for i, obj := range objects {
		var text string

		// Try "description" field first
		if desc, ok := obj["description"]; ok {
			if descStr, ok := desc.(string); ok && descStr != "" {
				text = descStr
			}
		}

		// Fall back to "text" field if "description" is not present or empty
		if text == "" {
			if t, ok := obj["text"]; ok {
				if textStr, ok := t.(string); ok && textStr != "" {
					text = textStr
				}
			}
		}

		// Fall back to rendering a canonical type-based acceptance object,
		// e.g. {"type":"test_passes","pattern":"*.go"} -> "test_passes: *.go".
		if text == "" {
			text = renderTypedCriterion(obj, i)
		}

		criteria = append(criteria, text)
	}

	return criteria, nil
}

// renderTypedCriterion renders an acceptance object that uses the canonical
// type-based form (see docs/design/architecture.md), e.g.
// {"type":"test_passes","pattern":"tests/auth/callback.test.ts"}. It produces
// "<type>: <field values>" with non-type field values sorted by key, or just
// "<type>" when no other fields are present. If the object has no usable "type"
// field, it falls back to the compact JSON encoding of the object.
func renderTypedCriterion(obj map[string]interface{}, index int) string {
	typeStr := ""
	if t, ok := obj["type"].(string); ok {
		typeStr = t
	}

	// Collect non-type field values in deterministic (alphabetical) key order.
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
		// No recognizable fields: fall back to the JSON encoding so the
		// criterion is still surfaced rather than dropped.
		if encoded, err := json.Marshal(obj); err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("acceptance criterion at index %d", index)
	}
}
