package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

// validProjectTypes is the closed set documented in docs/configuration.md.
var validProjectTypes = map[string]struct{}{
	"go":      {},
	"node":    {},
	"python":  {},
	"rust":    {},
	"make":    {},
	"unknown": {},
}

// retiredConfigKeys were removed from Config (ADR 0006 deleted mode) but still
// appear in pre-SB-ELIM config.json files. Strip them before DisallowUnknownFields
// so LoadConfig preserves custom TTL/budget/hooks instead of rejecting the file.
var retiredConfigKeys = []string{"mode"}

// StrictDecode decodes exactly one JSON object, rejecting unknown fields,
// a non-object document (including JSON null), and any trailing JSON value.
// The returned error names the offending key when the unknown field is in
// the object itself. Retired keys (see retiredConfigKeys) are ignored.
func StrictDecode(data []byte) (Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("strict config decode: %w", err)
	}
	if !isJSONObject(raw) {
		return Config{}, fmt.Errorf("strict config decode: document must be a JSON object")
	}

	var extra json.RawMessage
	switch err := dec.Decode(&extra); {
	case errors.Is(err, io.EOF):
		// exactly one value
	case err == nil:
		return Config{}, fmt.Errorf("strict config decode: unexpected trailing JSON value")
	default:
		return Config{}, fmt.Errorf("strict config decode: unexpected trailing JSON data: %w", err)
	}

	stripped, err := stripRetiredConfigKeys(raw)
	if err != nil {
		return Config{}, fmt.Errorf("strict config decode: %w", err)
	}

	var cfg Config
	strict := json.NewDecoder(bytes.NewReader(stripped))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("strict config decode: %w", err)
	}
	return cfg, nil
}

func stripRetiredConfigKeys(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	changed := false
	for _, key := range retiredConfigKeys {
		if _, ok := obj[key]; ok {
			delete(obj, key)
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// ValidatePresentFields strictly decodes data and reports a problem for every
// present field that is outside its valid range. Omitted fields are not checked,
// so an empty object is valid. Unknown fields fail via StrictDecode.
func ValidatePresentFields(data []byte) []string {
	cfg, err := StrictDecode(data)
	if err != nil {
		return []string{err.Error()}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return []string{fmt.Sprintf("strict config decode: %v", err)}
	}

	var problems []string
	if _, ok := raw["project_type"]; ok {
		if _, valid := validProjectTypes[cfg.ProjectType]; !valid {
			problems = append(problems, fmt.Sprintf("project_type %q is out of range", cfg.ProjectType))
		}
	}
	if _, ok := raw["default_ttl"]; ok {
		switch {
		case cfg.DefaultTTL <= 0:
			problems = append(problems, fmt.Sprintf("default_ttl %d is out of range (must be > 0 minutes)", cfg.DefaultTTL))
		case int64(cfg.DefaultTTL) > math.MaxInt64/60:
			problems = append(problems, fmt.Sprintf("default_ttl %d is out of range (must not overflow claim TTL seconds)", cfg.DefaultTTL))
		}
	}
	if _, ok := raw["token_budget"]; ok {
		switch {
		case cfg.TokenBudget <= 0:
			problems = append(problems, fmt.Sprintf("token_budget %d is out of range (must be > 0)", cfg.TokenBudget))
		case cfg.TokenBudget > math.MaxInt/4:
			problems = append(problems, fmt.Sprintf("token_budget %d is out of range (must not overflow character budget)", cfg.TokenBudget))
		}
	}
	if _, ok := raw["low_stakes_push_threshold"]; ok {
		if cfg.LowStakesPushThreshold <= 0 {
			problems = append(problems, fmt.Sprintf("low_stakes_push_threshold %d is out of range (must be > 0)", cfg.LowStakesPushThreshold))
		}
	}
	if _, ok := raw["hooks"]; ok {
		for i, hook := range cfg.Hooks {
			if hook.Name == "" {
				problems = append(problems, fmt.Sprintf("hooks[%d].name is out of range (must be non-empty)", i))
			}
			switch {
			case len(hook.Command) == 0:
				problems = append(problems, fmt.Sprintf("hooks[%d].command is out of range (must be non-empty)", i))
			case hook.Command[0] == "":
				problems = append(problems, fmt.Sprintf("hooks[%d].command[0] is out of range (must be non-empty)", i))
			}
		}
	}
	if _, ok := raw["gates"]; ok {
		problems = append(problems, "gates is unsupported (arm gate run reads "+GatesFileName+" at HEAD)")
	}
	sort.Strings(problems)
	return problems
}
