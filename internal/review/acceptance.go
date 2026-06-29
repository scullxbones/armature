package review

import (
	"encoding/json"
	"fmt"
)

// ParseAcceptanceCriteria normalizes acceptance criteria from either plain strings
// or structured objects to a list of plain strings. It supports two formats:
//
// 1. Plain strings: ["test 1", "test 2"]
// 2. Structured objects: [{"description":"test 1"}, {"text":"test 2"}]
//
// For structured objects, it extracts the "description" field first, then falls back
// to "text" field if "description" is not present. At least one of these fields
// must be present and non-empty.
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

		// Error if neither field exists or both are empty
		if text == "" {
			return nil, fmt.Errorf(
				"acceptance criteria object at index %d must have non-empty 'description' or 'text' field",
				i,
			)
		}

		criteria = append(criteria, text)
	}

	return criteria, nil
}
