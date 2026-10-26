package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"reflect"
	"strconv"
	"strings"
)

const sourceManifestFix = "Decode source-manifest bytes with sources.Manifest.Unmarshal or " +
	"sources.ReadManifest, then pass validate.KnownSources / validate.UnreadableSources into validate and decompose."

func checkSourceManifest(root string, stdout, stderr io.Writer) error {
	if err := requireGoMod(root); err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	files, fset, err := parseGoTree(root, false)
	if err != nil {
		fmt.Fprintf(stderr, "FAIL: %v\n", err)
		return err
	}
	var hits []string
	for _, pf := range files {
		if underDir(pf.rel, "testdata") {
			continue
		}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.TypeSpec:
				if x.Name != nil && x.Name.Name == "sourceEntry" {
					hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `duplicate source-manifest decoder names are banned; use sources.Manifest.Unmarshal`))
				}
			case *ast.FuncDecl:
				if x.Name != nil && (x.Name.Name == "parseManifestData" || x.Name.Name == "sourceIDsFromManifest") {
					hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `duplicate source-manifest decoder names are banned; use sources.Manifest.Unmarshal`))
				}
			case *ast.Field:
				name := jsonFieldName(x.Tag)
				rawMap := isRawMessageMap(x.Type, pf.imports)
				inSources := underDir(pf.rel, "internal/sources")
				switch name {
				case "entries":
					if rawMap {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `map[string]json.RawMessage json:"entries" reintroduces the PR #333/#362 decode hole`))
					} else if !inSources {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `json:"entries" is only allowed on sources.Manifest in internal/sources`))
					}
				case "last_synced":
					if !inSources {
						hits = append(hits, formatHit(fset, x.Pos(), pf.rel, `json:"last_synced" is only allowed on sources.SourceEntry in internal/sources`))
					}
				}
			}
			return true
		})
	}
	if len(hits) > 0 {
		for _, h := range hits {
			fmt.Fprintln(stderr, "FAIL: "+h)
		}
		fmt.Fprintln(stderr, sourceManifestFix)
		return fmt.Errorf("source-manifest decode ownership")
	}
	fmt.Fprintln(stdout, "OK: source-manifest JSON is decoded only by sources.Manifest.Unmarshal")
	return nil
}

func jsonFieldName(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	s, err := strconv.Unquote(tag.Value)
	if err != nil {
		return ""
	}
	j := reflect.StructTag(s).Get("json")
	if j == "" {
		return ""
	}
	name, _, _ := strings.Cut(j, ",")
	if name == "-" {
		return ""
	}
	return name
}

func isRawMessageMap(expr ast.Expr, imports map[string]string) bool {
	m, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := m.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return false
	}
	switch v := m.Value.(type) {
	case *ast.Ident:
		return v.Name == "RawMessage"
	case *ast.SelectorExpr:
		return v.Sel != nil && v.Sel.Name == "RawMessage" && isPkg(v.X, imports, "encoding/json")
	default:
		return false
	}
}

func formatHit(fset *token.FileSet, pos token.Pos, rel, msg string) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d: %s", rel, p.Line, msg)
}
