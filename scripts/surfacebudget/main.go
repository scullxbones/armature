// Surface-budget ratchet for internal packages (quality-controls C10).
//
// Counts unique exported identifiers per package under internal/ and
// compares them to a committed baseline. Growth fails; shrinking always
// passes. `make surface-budget-ratchet` rewrites the baseline to the
// current counts.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const baselineRel = "docs/design/surface-budget-baseline.json"

type packageSurface struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

type baseline struct {
	Packages map[string]packageSurface `json:"packages"`
}

func main() {
	write := false
	root := "."
	args := os.Args[1:]
	for len(args) > 0 {
		switch args[0] {
		case "-write":
			write = true
			args = args[1:]
		case "-check":
			write = false
			args = args[1:]
		case "-h", "-help", "--help":
			fmt.Fprintf(os.Stderr, "usage: surfacebudget [-check|-write] [repo-root]\n")
			os.Exit(0)
		default:
			if strings.HasPrefix(args[0], "-") {
				fmt.Fprintf(os.Stderr, "FAIL: unknown flag %s\n", args[0])
				os.Exit(2)
			}
			root = args[0]
			args = args[1:]
		}
	}

	root, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	current, err := scanInternal(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}

	path := filepath.Join(root, baselineRel)
	if write {
		if err := writeBaseline(path, current); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: write baseline: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Wrote %s (%d packages)\n", baselineRel, len(current.Packages))
		return
	}

	prev, err := readBaseline(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	if err := checkGrowth(prev, current); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: internal exported surface within baseline (%d packages)\n", len(current.Packages))
}

func readBaseline(path string) (baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return baseline{}, fmt.Errorf("baseline not found: %s (run make surface-budget-ratchet)", path)
	}
	var b baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return baseline{}, fmt.Errorf("parse baseline %s: %w", path, err)
	}
	if b.Packages == nil {
		b.Packages = map[string]packageSurface{}
	}
	return b, nil
}

func writeBaseline(path string, b baseline) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func checkGrowth(prev, current baseline) error {
	var failures []string
	var shrinks []string

	keys := make([]string, 0, len(current.Packages))
	for k := range current.Packages {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, pkg := range keys {
		cur := current.Packages[pkg]
		old, ok := prev.Packages[pkg]
		if !ok {
			if cur.Count == 0 {
				continue
			}
			failures = append(failures, fmt.Sprintf(
				"FAIL: new package %s has %d exported identifiers; run make surface-budget-ratchet after review",
				pkg, cur.Count))
			continue
		}
		if cur.Count > old.Count {
			added := diffNames(old.Names, cur.Names)
			msg := fmt.Sprintf("FAIL: %s exported surface grew %d -> %d", pkg, old.Count, cur.Count)
			if len(added) > 0 {
				msg += "; new identifiers: " + strings.Join(added, ", ")
			}
			failures = append(failures, msg)
			continue
		}
		if cur.Count < old.Count {
			shrinks = append(shrinks, fmt.Sprintf("%s %d -> %d", pkg, old.Count, cur.Count))
		}
	}

	if len(failures) > 0 {
		failures = append(failures,
			"Shrinking is always allowed. After a shrink, run make surface-budget-ratchet to lock the new baseline.")
		return fmt.Errorf("%s", strings.Join(failures, "\n"))
	}
	if len(shrinks) > 0 {
		fmt.Printf("OK: packages below baseline (ratchet down with make surface-budget-ratchet): %s\n",
			strings.Join(shrinks, "; "))
	}
	return nil
}

func diffNames(oldNames, newNames []string) []string {
	seen := make(map[string]struct{}, len(oldNames))
	for _, n := range oldNames {
		seen[n] = struct{}{}
	}
	var added []string
	for _, n := range newNames {
		if _, ok := seen[n]; !ok {
			added = append(added, n)
		}
	}
	sort.Strings(added)
	return added
}

func scanInternal(root string) (baseline, error) {
	internal := filepath.Join(root, "internal")
	st, err := os.Stat(internal)
	if err != nil {
		return baseline{}, fmt.Errorf("internal/: %w", err)
	}
	if !st.IsDir() {
		return baseline{}, fmt.Errorf("internal/ is not a directory")
	}

	byDir := map[string]map[string]struct{}{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(internal, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isIgnoreTagged(src) {
			return nil
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if file.Name == nil || file.Name.Name == "main" {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		names, ok := byDir[rel]
		if !ok {
			names = map[string]struct{}{}
			byDir[rel] = names
		}
		collectExported(file, names)
		return nil
	})
	if err != nil {
		return baseline{}, err
	}

	out := baseline{Packages: map[string]packageSurface{}}
	for pkg, set := range byDir {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		out.Packages[pkg] = packageSurface{Count: len(names), Names: names}
	}
	return out, nil
}

func isIgnoreTagged(src []byte) bool {
	// Files whose build constraint is ignore (generator scripts sitting in
	// a library package) are not part of the imported API.
	for _, line := range strings.Split(string(src), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "//go:build") {
			return strings.Contains(trim, "ignore")
		}
		if strings.HasPrefix(trim, "// +build") {
			return strings.Contains(trim, "ignore")
		}
		if strings.HasPrefix(trim, "//") {
			continue
		}
		return false
	}
	return false
}

func collectExported(file *ast.File, names map[string]struct{}) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() {
				continue
			}
			if recv := recvTypeName(d.Recv); recv != "" {
				if token.IsExported(recv) {
					names[recv+"."+d.Name.Name] = struct{}{}
				}
				continue
			}
			names[d.Name.Name] = struct{}{}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name == nil || !s.Name.IsExported() {
						continue
					}
					names[s.Name.Name] = struct{}{}
					collectTypeMembers(s.Type, s.Name.Name, names)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n != nil && n.IsExported() {
							names[n.Name] = struct{}{}
						}
					}
				}
			}
		}
	}
}

func collectTypeMembers(expr ast.Expr, typeName string, names map[string]struct{}) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		collectTypeMembers(t.X, typeName, names)
	case *ast.ParenExpr:
		collectTypeMembers(t.X, typeName, names)
	case *ast.StructType:
		if t.Fields == nil {
			return
		}
		for _, f := range t.Fields.List {
			if len(f.Names) == 0 {
				// Embedded field: use the type's exported name.
				if name := embeddedName(f.Type); name != "" && token.IsExported(name) && typeName != "" {
					names[typeName+"."+name] = struct{}{}
				}
				continue
			}
			for _, n := range f.Names {
				if n != nil && n.IsExported() && typeName != "" {
					names[typeName+"."+n.Name] = struct{}{}
				}
			}
		}
	case *ast.InterfaceType:
		if t.Methods == nil {
			return
		}
		for _, f := range t.Methods.List {
			for _, n := range f.Names {
				if n != nil && n.IsExported() && typeName != "" {
					names[typeName+"."+n.Name] = struct{}{}
				}
			}
		}
	}
}

func embeddedName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embeddedName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(t.X)
	case *ast.IndexListExpr:
		return embeddedName(t.X)
	default:
		return ""
	}
}

func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return embeddedName(recv.List[0].Type)
}
