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
	"(equals gitoid.PatternLower: lowercase 40|64 hex). Do not hard-code a pattern that " +
	"accepts a 40-hex OID but rejects a 64-hex OID. Per-site waivers: allow.go (file+line+needle)."

// Lowercase hex probe OIDs (gitoid.PatternLower alphabet). Consts avoid globals.
const (
	oidProbe40Hex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"                         // 40
	oidProbe64Hex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 64
)

var (
	shellRegexLine = regexp.MustCompile(`(?i)(\bgrep\b.*\s-[EP]\b|\begrep\b|\bsed\b|\[\[.*=~|\bre\.compile\b)`)
	quotedString   = regexp.MustCompile(`(?:'([^']*)'|"([^"]*)"|` + "`" + `([^` + "`" + `]*)` + "`" + `)`)
)

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
				if oidPatternRejects64(s) {
					pos := fset.Position(x.Pos())
					if !allowedObjectIDSite(pf.rel, pos.Line, s) {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel,
							`pattern accepts 40-hex OID but rejects 64-hex; use gitoid.Valid`))
					}
				}
			case *ast.BinaryExpr:
				// Keep len(x)==40: not a regex, so the compile-probe cannot see it.
				if reason := oidLenEq40Reason(x); reason != "" {
					pos := fset.Position(x.Pos())
					if !allowedObjectIDSite(pf.rel, pos.Line, "len==40") {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, reason))
					}
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

// oidPatternRejects64 reports that s compiles as a Go regexp and matches a
// 40-char lowercase hex OID but not the 64-char equivalent. This is the class
// probe — POSIX classes, \w, {40,40}, etc. fall out of MatchString behavior.
func oidPatternRejects64(s string) bool {
	re, err := regexp.Compile(s)
	if err != nil {
		return false
	}
	return re.MatchString(oidProbe40Hex) && !re.MatchString(oidProbe64Hex)
}

// oidLenEq40Reason flags len(x) == 40 / len(x) != 40 with a numeric literal.
// Kept alongside the regex probe: length compares are not regexp literals.
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
		if pat, ok := v["pattern"].(string); ok && oidPatternRejects64(pat) {
			if !allowedObjectIDSite(rel, 0, pat) {
				*hits = append(*hits, fmt.Sprintf("%s: pattern %s accepts 40-hex OID but rejects 64-hex; use gitoid.PatternLower", rel, path))
			}
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
				return nil
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
		if underDir(rel, "scripts/repoguard") || underDir(rel, "docs") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".sh", ".bash", ".py":
		default:
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !shellRegexLine.MatchString(line) && !strings.Contains(line, "re.compile") {
				continue
			}
			for _, pat := range quotedStringsIn(line) {
				if !oidPatternRejects64(pat) {
					continue
				}
				lineNo := i + 1
				if allowedObjectIDSite(rel, lineNo, pat) {
					continue
				}
				hits = append(hits, fmt.Sprintf("%s:%d: pattern accepts 40-hex OID but rejects 64-hex; use gitoid", rel, lineNo))
			}
		}
		return nil
	})
	return hits, err
}

func quotedStringsIn(line string) []string {
	var out []string
	for _, m := range quotedString.FindAllStringSubmatch(line, -1) {
		for _, g := range m[1:] {
			if g != "" {
				out = append(out, g)
			}
		}
	}
	return out
}
