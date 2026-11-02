package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const objectIDFix = "Use github.com/scullxbones/armature/internal/gitoid.Valid " +
	"(equals gitoid.PatternLower: lowercase 40|64 hex). Do not hard-code a 40-hex-only object-id pattern. " +
	"Per-site waivers go in allow.go (allowObjectID)."

// oidHex40Pattern matches hex-class + {40} or {40,40} OID regexes.
var oidHex40Pattern = regexp.MustCompile(`(?i)(\[[0-9a-fA-F\-]+\]|\\x[0-9a-fA-F]{2})\{40(?:,40)?\}`)

// oidHex64Pattern matches hex-class + {64} (dual-width companion).
var oidHex64Pattern = regexp.MustCompile(`(?i)(\[[0-9a-fA-F\-]+\]|\\x[0-9a-fA-F]{2})\{64(?:,64)?\}`)

// oidWord40Pattern matches \w{40} / \w{40,40} — never a hex-only OID predicate.
var oidWord40Pattern = regexp.MustCompile(`\\w\{40(?:,40)?\}`)

func isOIDTextExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".sh", ".bash", ".py", ".yml", ".yaml", ".json", ".md", ".txt", ".toml":
		return true
	default:
		return false
	}
}

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
		if allowed(pf.rel, allowObjectID) {
			continue
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(x.Value)
				if err != nil {
					return true
				}
				if isOIDOnly40(s) {
					hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `40-hex-only object-id pattern; allow 40|64 via gitoid`))
				}
			case *ast.BinaryExpr:
				if reason := oidLenEq40Reason(x); reason != "" {
					hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason))
				}
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
	textHits, err := checkTextObjectIDPatterns(root)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	hits = append(hits, textHits...)
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

// isOIDOnly40 reports a hard-coded 40-only (or \w{40}) OID pattern.
// A string that also carries a hex-class {64} quantifier is treated as dual-width
// and allowed (gitoid.PatternLower). A bare "{64}" substring elsewhere does NOT
// suppress a 40-only hit — that was the #419 gap.
func isOIDOnly40(s string) bool {
	if oidWord40Pattern.MatchString(s) {
		return true
	}
	if !oidHex40Pattern.MatchString(s) {
		return false
	}
	if oidHex64Pattern.MatchString(s) {
		return false
	}
	return true
}

// oidLenEq40Reason flags len(x) == 40 / len(x) != 40 with a numeric literal.
// Callers must use gitoid.Valid (or compare against both LenSHA1 and LenSHA256).
func oidLenEq40Reason(bin *ast.BinaryExpr) string {
	if bin.Op != token.EQL && bin.Op != token.NEQ {
		return ""
	}
	if litIs40(bin.Y) && isLenCall(bin.X) {
		return "len(x)==40 object-id check; use gitoid.Valid (40|64)"
	}
	if litIs40(bin.X) && isLenCall(bin.Y) {
		return "len(x)==40 object-id check; use gitoid.Valid (40|64)"
	}
	return ""
}

func litIs40(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == "40"
}

func isLenCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
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
		if allowed(rel, allowObjectID) {
			continue
		}
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

func checkTextObjectIDPatterns(root string) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			switch name {
			case "testdata", "vendor", ".git", "node_modules", "ops":
				return filepath.SkipDir
			case ".github":
				return nil // scan workflow YAML for raw 40-only OID regexes
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if underDir(rel, "scripts/repoguard") {
			return nil
		}
		// docs/schemas are covered by checkSchemaObjectIDPatterns; other docs
		// (ADRs, design notes) describe banned forms and are not executable.
		if underDir(rel, "docs") {
			return nil
		}
		if allowed(rel, allowObjectID) {
			return nil
		}
		ext := filepath.Ext(name)
		if !isOIDTextExt(ext) {
			return nil
		}
		// Go sources are covered by the AST pass (string lits + len==40).
		if strings.EqualFold(ext, ".go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if isOIDOnly40(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: 40-hex-only object-id pattern; allow 40|64 via gitoid", rel, i+1))
			}
		}
		return nil
	})
	return hits, err
}
