// Package docvalidate validates typed JSON examples in canonical Markdown sources.
package docvalidate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var artifactSchemas = map[string]string{
	"plan":                   "plan.schema.json",
	"review_bundle":          "review-bundle.schema.json",
	"review-bundle":          "review-bundle.schema.json",
	"conformance_assessment": "conformance-assessment.schema.json",
	"conformance-assessment": "conformance-assessment.schema.json",
	"activity_index":         "activity-index.schema.json",
	"activity-index":         "activity-index.schema.json",
}

var artifactFence = regexp.MustCompile("(?s)```json[ \\t]+artifact_type=([A-Za-z0-9_-]+)[ \\t]*\\n(.*?)```")

type example struct {
	path, artifactType, json string
	line                     int
}

// Validate checks typed JSON examples in docs and embedded canonical skills.
func Validate(repo string) error {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}
	examples, err := findExamples(repo)
	if err != nil {
		return err
	}
	validators, err := loadSchemas(filepath.Join(repo, "docs", "schemas"))
	if err != nil {
		return err
	}
	var failures []error
	for _, ex := range examples {
		schemaName, ok := artifactSchemas[ex.artifactType]
		if !ok {
			failures = append(failures, ex.errorf("Unknown artifact type: %s", ex.artifactType))
			continue
		}
		var document any
		if err := json.Unmarshal([]byte(strings.TrimSpace(ex.json)), &document); err != nil {
			failures = append(failures, ex.errorf("invalid JSON: %v", err))
			continue
		}
		if err := validators[schemaName].Validate(document); err != nil {
			failures = append(failures, ex.errorf("%v", err))
		}
	}
	return errors.Join(failures...)
}

func (e example) errorf(format string, args ...any) error {
	return fmt.Errorf("%s:%d: "+format, append([]any{e.path, e.line}, args...)...)
}

func findExamples(repo string) ([]example, error) {
	var examples []example
	for _, dir := range []string{filepath.Join(repo, "docs"), filepath.Join(repo, "internal", "skillsembed", "skills")} {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			content, err := os.ReadFile(path) //nolint:gosec // path is restricted to Markdown discovered under canonical directories
			if err != nil {
				return err
			}
			for _, match := range artifactFence.FindAllSubmatchIndex(content, -1) {
				examples = append(examples, example{
					path:         relativePath(repo, path),
					line:         bytes.Count(content[:match[0]], []byte("\n")) + 1,
					artifactType: string(content[match[2]:match[3]]),
					json:         string(content[match[4]:match[5]]),
				})
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("scan canonical Markdown: %w", err)
		}
	}
	return examples, nil
}

func relativePath(repo, path string) string {
	rel, err := filepath.Rel(repo, path)
	if err == nil {
		return rel
	}
	return path
}

func loadSchemas(dir string) (map[string]*jsonschema.Schema, error) {
	validators := make(map[string]*jsonschema.Schema, len(artifactSchemas))
	for _, name := range artifactSchemas {
		if _, loaded := validators[name]; loaded {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // schema names are fixed by artifactSchemas
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", name, err)
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(name, bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("load schema %s: %w", name, err)
		}
		schema, err := compiler.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("compile schema %s: %w", name, err)
		}
		validators[name] = schema
	}
	return validators, nil
}
