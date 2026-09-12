package output

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
)

var (
	schemaOnce sync.Once
	schemaRoot string
	schemaErr  error
)

func validateAgainstCitedSchema(name string, body []byte) error {
	schema, err := loadCitedSchema(name)
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(body, &instance); err != nil {
		return fmt.Errorf("must be JSON: %w", err)
	}
	return applyJSONSchema(schema, instance, "$")
}

func loadCitedSchema(name string) (map[string]any, error) {
	root, err := docsSchemaDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // schema names are fixed citations
	if err != nil {
		return nil, fmt.Errorf("read cited schema %s: %w", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse cited schema %s: %w", name, err)
	}
	return schema, nil
}

func docsSchemaDir() (string, error) {
	schemaOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			schemaErr = fmt.Errorf("cannot locate cited JSON Schema files")
			return
		}
		dir := filepath.Dir(file)
		for range 8 {
			candidate := filepath.Join(dir, "docs", "schemas")
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				schemaRoot = candidate
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		schemaErr = fmt.Errorf("docs/schemas not found from %s", file)
	})
	return schemaRoot, schemaErr
}

func applyJSONSchema(schema map[string]any, instance any, path string) error {
	if types, ok := schemaTypes(schema); ok && !matchesJSONType(instance, types) {
		return fmt.Errorf("%s: want JSON type %v", path, types)
	}
	if instance == nil {
		return nil
	}
	if rawEnum, ok := schema["enum"]; ok {
		enum, ok := rawEnum.([]any)
		if !ok {
			return fmt.Errorf("%s: schema enum must be an array", path)
		}
		if !enumContains(enum, instance) {
			return fmt.Errorf("%s: value not in enum", path)
		}
	}
	switch inst := instance.(type) {
	case map[string]any:
		if rawReq, ok := schema["required"]; ok {
			req, ok := rawReq.([]any)
			if !ok {
				return fmt.Errorf("%s: schema required must be an array", path)
			}
			for _, item := range req {
				key, ok := item.(string)
				if !ok {
					return fmt.Errorf("%s: schema required entries must be strings", path)
				}
				if _, exists := inst[key]; !exists {
					return fmt.Errorf("%s: missing %s", path, key)
				}
			}
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			props = map[string]any{}
		}
		for key, value := range inst {
			sub, ok := props[key].(map[string]any)
			if !ok {
				continue
			}
			if err := applyJSONSchema(sub, value, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		items, ok := schema["items"].(map[string]any)
		if !ok {
			break
		}
		for i, item := range inst {
			if err := applyJSONSchema(items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case string:
		if pat, ok := schema["pattern"].(string); ok {
			re, err := regexp.Compile(pat)
			if err != nil {
				return fmt.Errorf("%s: invalid schema pattern: %w", path, err)
			}
			if !re.MatchString(inst) {
				return fmt.Errorf("%s: does not match pattern %s", path, pat)
			}
		}
		if min, ok := schemaInt(schema["minLength"]); ok && len(inst) < min {
			return fmt.Errorf("%s: shorter than minLength %d", path, min)
		}
	case float64:
		if min, ok := schemaFloat(schema["minimum"]); ok && inst < min {
			return fmt.Errorf("%s: below minimum %v", path, min)
		}
	}
	return nil
}

func schemaTypes(schema map[string]any) ([]string, bool) {
	raw, ok := schema["type"]
	if !ok {
		return nil, false
	}
	switch t := raw.(type) {
	case string:
		return []string{t}, true
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func matchesJSONType(instance any, types []string) bool {
	for _, t := range types {
		switch t {
		case "null":
			if instance == nil {
				return true
			}
		case "object":
			if _, ok := instance.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := instance.([]any); ok {
				return true
			}
		case "string":
			if _, ok := instance.(string); ok {
				return true
			}
		case "boolean":
			if _, ok := instance.(bool); ok {
				return true
			}
		case "number":
			if _, ok := instance.(float64); ok {
				return true
			}
		case "integer":
			n, ok := instance.(float64)
			if ok && !math.IsInf(n, 0) && !math.IsNaN(n) && n == math.Trunc(n) {
				return true
			}
		}
	}
	return false
}

func enumContains(enum []any, instance any) bool {
	enc, err := json.Marshal(instance)
	if err != nil {
		return false
	}
	for _, item := range enum {
		itemEnc, err := json.Marshal(item)
		if err != nil {
			continue
		}
		if string(enc) == string(itemEnc) {
			return true
		}
	}
	return false
}

func schemaInt(v any) (int, bool) {
	n, ok := schemaFloat(v)
	if !ok || n != math.Trunc(n) {
		return 0, false
	}
	return int(n), true
}

func schemaFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
