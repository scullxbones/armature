package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const objectIDFix = "Use github.com/scullxbones/armature/internal/gitoid.Valid " +
	"(equals gitoid.PatternLower: lowercase 40|64 hex). Do not hard-code a 40-hex-only object-id pattern."

// oidOnly40Pattern matches hex-class + {40} OID regexes that do not also allow {64}.
var oidOnly40Pattern = regexp.MustCompile(`(?i)(\[[0-9a-fA-F\-]+\]|\\x[0-9a-fA-F]{2})\{40\}`)

func checkObjectID(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	var hits []string
	files, fset, err := parseGoTree(root, false)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	for _, pf := range files {
		if underDir(pf.rel, "testdata") || underDir(pf.rel, "scripts/repoguard") {
			continue
		}
		if pf.rel == "internal/gitoid/oid.go" {
			continue
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if isOIDOnly40(s) {
				hits = append(hits, formatHit(fset, lit.Pos(), pf.rel, `40-hex-only object-id pattern; allow 40|64 via gitoid`))
			}
			return true
		})
	}
	schemaHits, err := checkSchemaObjectIDPatterns(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	hits = append(hits, schemaHits...)
	if len(hits) > 0 {
		for _, h := range hits {
			fmt.Fprintln(stderr, "FAIL: "+h)
		}
		fmt.Fprintln(stderr, objectIDFix)
		return fmt.Errorf("object-id 40-hex-only patterns")
	}
	fmt.Fprintln(stdout, "OK: object-id patterns accept SHA-1 (40) and SHA-256 (64) via gitoid")
	return nil
}

func isOIDOnly40(s string) bool {
	if !oidOnly40Pattern.MatchString(s) {
		return false
	}
	if strings.Contains(s, "{64}") {
		return false
	}
	return true
}

func checkSchemaObjectIDPatterns(root string) ([]string, error) {
	dir := filepath.Join(root, "docs", "schemas")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("docs", "schemas", e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		walkSchemaPatterns(doc, rel, "", &hits)
	}
	return hits, nil
}

func walkSchemaPatterns(node any, rel, path string, hits *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if pat, ok := v["pattern"].(string); ok && isOIDOnly40(pat) {
			*hits = append(*hits, fmt.Sprintf("%s: pattern %s is 40-hex-only; use gitoid.PatternLower (40|64)", rel, path))
		}
		for k, child := range v {
			childPath := path
			if childPath == "" {
				childPath = k
			} else {
				childPath = path + "." + k
			}
			walkSchemaPatterns(child, rel, childPath, hits)
		}
	case []any:
		for i, child := range v {
			walkSchemaPatterns(child, rel, fmt.Sprintf("%s[%d]", path, i), hits)
		}
	}
}
